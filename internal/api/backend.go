package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/TheLazyLemur/claudecord/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/pkg/errors"
)

var _ core.Backend = (*Backend)(nil)

const defaultModel = "claude-sonnet-4-20250514"

// Backend implements core.Backend using Anthropic API
type Backend struct {
	client        anthropic.Client
	sessionID     string
	history       []anthropic.MessageParam
	tools         []anthropic.ToolUnionParam
	systemPrompt  string
	skillStore    skills.SkillStore
	minimaxAPIKey string
	mu            sync.Mutex
}

// NewBackend creates an API backend
func NewBackend(client anthropic.Client, systemPrompt string, tools []anthropic.ToolUnionParam, skillStore skills.SkillStore, minimaxAPIKey string) *Backend {
	return &Backend{
		client:        client,
		sessionID:     generateSessionID(),
		history:       []anthropic.MessageParam{},
		tools:         tools,
		systemPrompt:  systemPrompt,
		skillStore:    skillStore,
		minimaxAPIKey: minimaxAPIKey,
	}
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
		Model:     defaultModel,
		MaxTokens: 8192,
		Messages:  b.history,
	}

	if b.systemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: b.systemPrompt},
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

		// Convert input to map
		var input map[string]any
		if err := json.Unmarshal(tu.Input, &input); err != nil {
			results = append(results, anthropic.NewToolResultBlock(tu.ID, "Invalid input: "+err.Error(), true))
			continue
		}

		// Check permissions
		allow, reason := tools.CheckPermission(tu.Name, input, perms, responder)
		if !allow {
			results = append(results, anthropic.NewToolResultBlock(tu.ID, "Permission denied: "+reason, true))
			continue
		}

		// Execute the tool
		deps := tools.Deps{Responder: responder, SkillStore: store, MinimaxAPIKey: minimaxAPIKey}
		result, isError := tools.Execute(tu.Name, input, deps)
		results = append(results, anthropic.NewToolResultBlock(tu.ID, result, isError))
	}

	return results, nil
}

// BackendFactory creates API backends
type BackendFactory struct {
	APIKey         string
	BaseURL        string
	AllowedDirs    []string
	DefaultWorkDir string
	SkillStore     skills.SkillStore
	MinimaxAPIKey  string
	Passive        bool
}

var _ core.BackendFactory = (*BackendFactory)(nil)

func (f *BackendFactory) Create(workDir string) (core.Backend, error) {
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
	} else {
		base = "When you receive a message, first call react_emoji with '👀' to acknowledge. For longer tasks, use send_update to post progress updates."
		apiTools = buildTools()
	}
	systemPrompt := core.BuildSystemPrompt(base, f.SkillStore)

	return NewBackend(client, systemPrompt, apiTools, f.SkillStore, f.MinimaxAPIKey), nil
}

func buildTools() []anthropic.ToolUnionParam {
	allTools := append(core.DiscordTools(), core.FileTools()...)
	allTools = append(allTools, core.SkillTools()...)

	var tools []anthropic.ToolUnionParam
	for _, t := range allTools {
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: convertInputSchema(t.InputSchema),
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &tool})
	}

	return tools
}

func buildPassiveTools() []anthropic.ToolUnionParam {
	var tools []anthropic.ToolUnionParam
	for _, t := range core.FileTools() {
		tool := anthropic.ToolParam{
			Name:        t.Name,
			Description: anthropic.String(t.Description),
			InputSchema: convertInputSchema(t.InputSchema),
		}
		tools = append(tools, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return tools
}

func convertInputSchema(schema map[string]any) anthropic.ToolInputSchemaParam {
	return anthropic.ToolInputSchemaParam{
		Type:       "object",
		Properties: schema["properties"],
	}
}
