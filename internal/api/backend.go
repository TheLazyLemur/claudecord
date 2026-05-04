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

// Backend implements core.Backend using Anthropic API
type Backend struct {
	client        anthropic.Client
	model         string
	sessionID     string
	history       []anthropic.MessageParam
	tools         []anthropic.ToolUnionParam
	systemPrompt  string
	workDir       string
	skillStore    skills.SkillStore
	minimaxAPIKey string
	mu            sync.Mutex
}

// NewBackend creates an API backend. workDir is checked for an AGENTS.md
// file on every API call; its contents are appended to the system prompt.
func NewBackend(client anthropic.Client, model, systemPrompt, workDir string, tools []anthropic.ToolUnionParam, skillStore skills.SkillStore, minimaxAPIKey string) *Backend {
	if model == "" {
		model = config.DefaultModel
	}
	return &Backend{
		client:        client,
		model:         model,
		sessionID:     generateSessionID(),
		history:       []anthropic.MessageParam{},
		tools:         tools,
		systemPrompt:  systemPrompt,
		workDir:       workDir,
		skillStore:    skillStore,
		minimaxAPIKey: minimaxAPIKey,
	}
}

// effectiveSystemPrompt re-reads AGENTS.md from workDir on each call so live
// edits to the file land in the next turn without restarting the session.
func (b *Backend) effectiveSystemPrompt() string {
	return core.AppendAgentsContext(b.systemPrompt, core.LoadAgentsContext(b.workDir))
}

func generateSessionID() string {
	return "api-" + time.Now().Format("20060102-150405")
}

func (b *Backend) SessionID() string {
	return b.sessionID
}

func (b *Backend) Close() error {
	return nil
}

// Converse sends a message and handles the response loop
func (b *Backend) Converse(ctx context.Context, msg string, responder core.Responder, perms core.PermissionChecker) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Add user message to history
	b.history = append(b.history, anthropic.NewUserMessage(anthropic.NewTextBlock(msg)))

	return b.runConversationLoop(ctx, responder, perms)
}

func (b *Backend) runConversationLoop(ctx context.Context, responder core.Responder, perms core.PermissionChecker) (string, error) {
	var finalResponse string

	for {
		resp, err := b.callAPI(ctx)
		if err != nil {
			return finalResponse, errors.Wrap(err, "API call failed")
		}

		// Extract text response
		text := extractTextFromResponse(resp)
		if text != "" {
			if finalResponse != "" {
				finalResponse += "\n"
			}
			finalResponse += text
		}

		// Add assistant message to history
		b.history = append(b.history, resp.ToParam())

		// Check for tool use
		toolUses := extractToolUses(resp)
		if len(toolUses) == 0 {
			// No tools, we're done
			return finalResponse, nil
		}

		// Execute tools
		toolResults, err := b.executeTools(ctx, toolUses, responder, perms, b.skillStore, b.minimaxAPIKey)
		if err != nil {
			return finalResponse, errors.Wrap(err, "tool execution failed")
		}

		// Add tool results to history
		b.history = append(b.history, anthropic.NewUserMessage(toolResults...))

		// Check stop reason
		if resp.StopReason == "end_turn" {
			return finalResponse, nil
		}
	}
}

func (b *Backend) callAPI(ctx context.Context) (*anthropic.Message, error) {
	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(b.model),
		MaxTokens: 8192,
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

	return b.client.Messages.New(ctx, params)
}

func extractTextFromResponse(resp *anthropic.Message) string {
	var text string
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			text += b.Text
		}
	}
	return text
}

func extractToolUses(resp *anthropic.Message) []anthropic.ToolUseBlock {
	var tools []anthropic.ToolUseBlock
	for _, block := range resp.Content {
		if tu, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			tools = append(tools, tu)
		}
	}
	return tools
}

func (b *Backend) executeTools(ctx context.Context, toolUses []anthropic.ToolUseBlock, responder core.Responder, perms core.PermissionChecker, store skills.SkillStore, minimaxAPIKey string) ([]anthropic.ContentBlockParamUnion, error) {
	var results []anthropic.ContentBlockParamUnion

	for _, tu := range toolUses {
		slog.Info("executing tool", "name", tu.Name, "id", tu.ID)

		var input core.ToolInput
		if err := json.Unmarshal(tu.Input, &input); err != nil {
			results = append(results, anthropic.NewToolResultBlock(tu.ID, "Invalid input: "+err.Error(), true))
			continue
		}

		allow, reason := tools.CheckPermission(tu.Name, input, perms)
		if !allow {
			results = append(results, anthropic.NewToolResultBlock(tu.ID, "Permission denied: "+reason, true))
			continue
		}

		deps := tools.Deps{Responder: responder, SkillStore: store, MinimaxAPIKey: minimaxAPIKey}
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
	AllowedDirs    []string
	DefaultWorkDir string
	SkillStore     skills.SkillStore
	MinimaxAPIKey  string
	Passive        bool
	Discord        bool
	// WhatsAppEnabled appends the media-handling addendum to the system prompt
	// so the model knows what to do with <attachment> tags in chat prompts.
	WhatsAppEnabled bool
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

	return NewBackend(client, f.Model, systemPrompt, workDir, apiTools, f.SkillStore, f.MinimaxAPIKey), nil
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
