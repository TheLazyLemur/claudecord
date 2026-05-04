package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/TheLazyLemur/claudecord/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/pkg/errors"
)

var _ core.Backend = (*Backend)(nil)

// Backend implements core.Backend using Anthropic API.
//
// Concurrency: at most one runConversationLoop runs at a time per Backend.
// While a loop is running, additional Converse calls enqueue their message
// into the mailbox and return immediately ("steering"). The running loop
// drains the mailbox after each tool round-trip and at natural exit, so
// queued messages are picked up before the next API call without aborting
// the in-flight one.
type Backend struct {
	client         anthropic.Client
	model          string
	sessionID      string
	history        []anthropic.MessageParam
	tools          []anthropic.ToolUnionParam
	systemPrompt   string
	workDir        string
	skillStore     skills.SkillStore
	minimaxAPIKey  string
	thinkingBudget int

	mu      sync.Mutex
	running bool
	mailbox []string
}

// NewBackend creates an API backend. workDir is checked for an AGENTS.md
// file on every API call; its contents are appended to the system prompt.
// thinkingBudget > 0 enables extended thinking with that token budget.
func NewBackend(client anthropic.Client, model, systemPrompt, workDir string, tools []anthropic.ToolUnionParam, skillStore skills.SkillStore, minimaxAPIKey string, thinkingBudget int) *Backend {
	if model == "" {
		model = config.DefaultModel
	}
	return &Backend{
		client:         client,
		model:          model,
		sessionID:      "api-" + time.Now().Format("20060102-150405"),
		history:        []anthropic.MessageParam{},
		tools:          tools,
		systemPrompt:   systemPrompt,
		workDir:        workDir,
		skillStore:     skillStore,
		minimaxAPIKey:  minimaxAPIKey,
		thinkingBudget: thinkingBudget,
	}
}

// effectiveSystemPrompt re-reads AGENTS.md from workDir on each call so live
// edits to the file land in the next turn without restarting the session.
func (b *Backend) effectiveSystemPrompt() string {
	return core.AppendAgentsContext(b.systemPrompt, core.LoadAgentsContext(b.workDir))
}

func (b *Backend) SessionID() string {
	return b.sessionID
}

func (b *Backend) Close() error {
	return nil
}

// Converse sends a message and handles the response loop. If a loop is
// already running for this Backend the message is queued and an empty
// response is returned; the running loop's responder will produce the
// combined reply.
func (b *Backend) Converse(ctx context.Context, msg string, responder core.Responder, perms core.PermissionChecker) (string, error) {
	if !b.claim(msg) {
		slog.Info("steering message queued", "session", b.sessionID)
		return "", nil
	}

	resp, err := b.runConversationLoop(ctx, responder, perms)
	if err != nil {
		b.release()
	}
	return resp, err
}

// claim atomically takes ownership of the loop. Returns true if this caller
// now owns the loop (msg has been appended to history). Returns false if
// another loop is already running, in which case msg has been added to the
// mailbox and the caller must return without posting a response.
func (b *Backend) claim(msg string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.running {
		b.mailbox = append(b.mailbox, msg)
		return false
	}

	b.running = true
	blocks := []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(msg)}
	for _, queued := range b.mailbox {
		blocks = append(blocks, anthropic.NewTextBlock(steeringText(queued)))
	}
	b.mailbox = nil
	b.history = append(b.history, anthropic.NewUserMessage(blocks...))
	return true
}

// drainMailbox returns and clears any pending steering messages. Used between
// tool round-trips, before the next API call. Does not change running.
func (b *Backend) drainMailbox() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.mailbox) == 0 {
		return nil
	}
	msgs := b.mailbox
	b.mailbox = nil
	return msgs
}

// finishOrContinue is called when the loop would otherwise return. If the
// mailbox is empty it atomically marks running=false and returns nil. If the
// mailbox has messages it returns them and the caller must continue the loop;
// running stays true. The atomicity is what prevents a steering message from
// being stranded between the loop's exit and the running flag flipping.
func (b *Backend) finishOrContinue() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.mailbox) == 0 {
		b.running = false
		return nil
	}
	msgs := b.mailbox
	b.mailbox = nil
	return msgs
}

// release clears the running flag on error paths so a future Converse call
// can start a new loop. Queued messages are preserved and will be picked up
// by the next claim().
func (b *Backend) release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.running = false
}

func steeringText(msg string) string {
	return "<user_steering>" + msg + "</user_steering>"
}

func (b *Backend) runConversationLoop(ctx context.Context, responder core.Responder, perms core.PermissionChecker) (string, error) {
	var finalResponse string

	for {
		resp, err := b.client.Messages.New(ctx, b.buildParams())
		if err != nil {
			return finalResponse, errors.Wrap(err, "API call failed")
		}

		text, toolUses := splitContent(resp)
		if text != "" {
			if finalResponse != "" {
				finalResponse += "\n"
			}
			finalResponse += text
		}

		b.history = append(b.history, resp.ToParam())

		if len(toolUses) == 0 {
			steered := b.finishOrContinue()
			if len(steered) == 0 {
				return finalResponse, nil
			}
			b.history = append(b.history, steeringUserMessage(steered))
			continue
		}

		toolResults, err := b.executeTools(ctx, toolUses, responder, perms)
		if err != nil {
			return finalResponse, errors.Wrap(err, "tool execution failed")
		}
		for _, m := range b.drainMailbox() {
			toolResults = append(toolResults, anthropic.NewTextBlock(steeringText(m)))
		}
		b.history = append(b.history, anthropic.NewUserMessage(toolResults...))
	}
}

func steeringUserMessage(msgs []string) anthropic.MessageParam {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msgs))
	for _, m := range msgs {
		blocks = append(blocks, anthropic.NewTextBlock(steeringText(m)))
	}
	return anthropic.NewUserMessage(blocks...)
}

func (b *Backend) buildParams() anthropic.MessageNewParams {
	maxTokens := int64(8192)
	if b.thinkingBudget > 0 && int64(b.thinkingBudget) >= maxTokens {
		maxTokens = int64(b.thinkingBudget) + 4096
	}
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(b.model),
		MaxTokens: maxTokens,
		Messages:  b.history,
	}

	if sys := b.effectiveSystemPrompt(); sys != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: sys},
		}
	}

	if len(b.tools) > 0 {
		params.Tools = b.tools
	}

	if b.thinkingBudget > 0 {
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(b.thinkingBudget))
	}

	return params
}

// splitContent walks resp.Content once and separates the text we surface
// to the user from the tool_use blocks we must execute next turn.
func splitContent(resp *anthropic.Message) (text string, tools []anthropic.ToolUseBlock) {
	for _, block := range resp.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			text += v.Text
		case anthropic.ToolUseBlock:
			tools = append(tools, v)
		}
	}
	return
}

func (b *Backend) executeTools(ctx context.Context, toolUses []anthropic.ToolUseBlock, responder core.Responder, perms core.PermissionChecker) ([]anthropic.ContentBlockParamUnion, error) {
	var results []anthropic.ContentBlockParamUnion

	for _, tu := range toolUses {
		slog.Info("executing tool", "name", tu.Name, "id", tu.ID)

		var input core.ToolInput
		if err := json.Unmarshal(tu.Input, &input); err != nil {
			results = append(results, anthropic.NewToolResultBlock(tu.ID, "Invalid input: "+err.Error(), true))
			continue
		}

		allow, reason := perms.Check(tu.Name, input)
		if !allow {
			results = append(results, anthropic.NewToolResultBlock(tu.ID, "Permission denied: "+reason, true))
			continue
		}

		deps := tools.Deps{Responder: responder, SkillStore: b.skillStore, MinimaxAPIKey: b.minimaxAPIKey}
		result, isError := tools.Execute(tu.Name, input, deps)
		results = append(results, buildToolResultBlock(tu.ID, result, isError))
	}

	return results, nil
}

// buildToolResultBlock turns an Execute result into a ContentBlockParamUnion.
// When the result starts with tools.ImageSentinel, the block is constructed
// with an ImageBlockParam so the model's vision encoder fires on the
// tool_result content.
func buildToolResultBlock(id, result string, isError bool) anthropic.ContentBlockParamUnion {
	if !isError && strings.HasPrefix(result, tools.ImageSentinel+"\t") {
		parts := strings.SplitN(result, "\t", 3)
		if len(parts) == 3 {
			imageBlock := anthropic.ImageBlockParam{
				Source: anthropic.ImageBlockParamSourceUnion{
					OfBase64: &anthropic.Base64ImageSourceParam{
						Data:      parts[2],
						MediaType: anthropic.Base64ImageSourceMediaType(parts[1]),
					},
				},
			}
			toolBlock := anthropic.ToolResultBlockParam{
				ToolUseID: id,
				Content: []anthropic.ToolResultBlockParamContentUnion{
					{OfImage: &imageBlock},
				},
			}
			return anthropic.ContentBlockParamUnion{OfToolResult: &toolBlock}
		}
	}
	return anthropic.NewToolResultBlock(id, result, isError)
}

// BackendFactory creates API backends
type BackendFactory struct {
	APIKey         string
	BaseURL        string
	Model          string
	DefaultWorkDir string
	SkillStore     skills.SkillStore
	MinimaxAPIKey  string
	Passive        bool
	Discord        bool
	// WhatsAppEnabled appends the media-handling addendum to the system prompt
	// so the model knows what to do with <attachment> tags in chat prompts.
	WhatsAppEnabled bool
	// ThinkingBudgetTokens > 0 enables extended thinking on every API call.
	ThinkingBudgetTokens int
}

var _ core.BackendFactory = (*BackendFactory)(nil)

func (f *BackendFactory) Create(workDir string) (core.Backend, error) {
	if workDir == "" {
		workDir = f.DefaultWorkDir
	}

	opts := []option.RequestOption{
		option.WithAPIKey(f.APIKey),
	}
	if f.BaseURL != "" {
		opts = append(opts, option.WithBaseURL(f.BaseURL))
	}

	client := anthropic.NewClient(opts...)

	var base string
	var apiTools []anthropic.ToolUnionParam
	if f.Passive {
		base = core.PassiveSystemPrompt()
		apiTools = buildPassiveTools()
	} else if f.Discord {
		base = "When you receive a message, first call react_emoji with '👀' to acknowledge. For longer tasks, use send_update to post progress updates."
		apiTools = buildDiscordTools()
	} else {
		base = "Use send_update to post progress updates for longer tasks."
		apiTools = buildChatTools()
	}
	if f.WhatsAppEnabled && !f.Passive {
		base += "\n" + core.WhatsAppMediaSystemPromptAddendum
	}
	systemPrompt := core.BuildSystemPrompt(base, f.SkillStore)

	return NewBackend(client, f.Model, systemPrompt, workDir, apiTools, f.SkillStore, f.MinimaxAPIKey, f.ThinkingBudgetTokens), nil
}

func buildToolParams(defs []core.ToolDef) []anthropic.ToolUnionParam {
	var tools []anthropic.ToolUnionParam
	for _, t := range defs {
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: convertInputSchema(t.InputSchema),
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return tools
}

func buildDiscordTools() []anthropic.ToolUnionParam {
	allTools := append(core.DiscordTools(), core.FileTools()...)
	allTools = append(allTools, core.SkillTools()...)
	return buildToolParams(allTools)
}

func buildChatTools() []anthropic.ToolUnionParam {
	allTools := append(core.ChatTools(), core.FileTools()...)
	allTools = append(allTools, core.SkillTools()...)
	return buildToolParams(allTools)
}

func buildPassiveTools() []anthropic.ToolUnionParam {
	return buildToolParams(core.FileTools())
}

func convertInputSchema(schema map[string]any) anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Type:       "object",
		Properties: schema["properties"],
	}
}
