package cli

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/TheLazyLemur/claudecord/internal/tools"
	"github.com/pkg/errors"
)

var _ core.Backend = (*Backend)(nil)

// Backend wraps a CLI Process and handles the message loop
type Backend struct {
	proc       *Process
	skillStore skills.SkillStore
	mu         sync.Mutex
}

// NewBackend creates a Backend from a CLI Process
func NewBackend(proc *Process, skillStore skills.SkillStore) *Backend {
	return &Backend{proc: proc, skillStore: skillStore}
}

func (b *Backend) SessionID() string {
	return b.proc.SessionID()
}

func (b *Backend) Close() error {
	return b.proc.Close()
}

// Converse sends a message and processes responses until result
func (b *Backend) Converse(ctx context.Context, msg string, responder core.Responder, perms core.PermissionChecker) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if err := b.sendUserMessage(msg); err != nil {
		return "", errors.Wrap(err, "sending user message")
	}

	return b.processResponses(ctx, responder, perms)
}

func (b *Backend) sendUserMessage(content string) error {
	msg := UserMessage{
		Type:      "user",
		SessionID: b.proc.SessionID(),
		Message: UserMessageInner{
			Role:    "user",
			Content: content,
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, "marshaling user message")
	}
	return b.proc.Send(data)
}

func (b *Backend) processResponses(ctx context.Context, responder core.Responder, perms core.PermissionChecker) (string, error) {
	recvChan, err := b.proc.Receive()
	if err != nil {
		return "", errors.Wrap(err, "receiving")
	}

	var responseText string

	for {
		select {
		case <-ctx.Done():
			return responseText, ctx.Err()
		case data, ok := <-recvChan:
			if !ok {
				return responseText, nil
			}

			var msg CLIMessage
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			if msg.IsReplay {
				continue
			}

			slog.Info("CLI response", "type", msg.Type)

			switch msg.Type {
			case "assistant":
				text := extractTextFromAssistant(msg)
				if text != "" {
					if responseText != "" {
						responseText += "\n"
					}
					responseText += text
				}

			case "control_request":
				if err := b.handleControlRequest(msg, responder, perms); err != nil {
					return responseText, errors.Wrap(err, "handling control request")
				}

			case "result":
				slog.Info("got result", "len", len(responseText))
				return responseText, nil
			}
		}
	}
}

func extractTextFromAssistant(msg CLIMessage) string {
	var am AssistantMessage
	if err := json.Unmarshal(msg.Message, &am); err != nil {
		return ""
	}

	var text string
	for _, block := range am.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}
	return text
}

func (b *Backend) handleControlRequest(msg CLIMessage, responder core.Responder, perms core.PermissionChecker) error {
	// peek at subtype to route
	var peek struct {
		Subtype string `json:"subtype"`
	}
	if err := json.Unmarshal(msg.Request, &peek); err != nil {
		return nil
	}

	slog.Info("control_request", "subtype", peek.Subtype, "request_id", msg.RequestID)

	switch peek.Subtype {
	case "can_use_tool":
		return b.handleCanUseTool(msg, responder, perms)
	case "mcp_message":
		return b.handleMCPMessage(msg, responder, b.skillStore)
	}

	return nil
}

func (b *Backend) handleCanUseTool(msg CLIMessage, responder core.Responder, perms core.PermissionChecker) error {
	var req CanUseToolRequest
	if err := json.Unmarshal(msg.Request, &req); err != nil {
		return errors.Wrap(err, "unmarshaling can_use_tool request")
	}

	var input core.ToolInput
	if err := json.Unmarshal(req.Input, &input); err != nil {
		return errors.Wrap(err, "unmarshaling tool input")
	}

	allow, reason := tools.CheckPermission(req.ToolName, input, perms, responder)
	return b.sendPermissionResponse(msg.RequestID, req.ToolUseID, allow, reason, input)
}

func (b *Backend) sendPermissionResponse(requestID, toolUseID string, allow bool, reason string, input core.ToolInput) error {
	var innerResp any
	if allow {
		innerResp = PermissionAllow{
			Behavior:     "allow",
			ToolUseID:    toolUseID,
			UpdatedInput: input,
		}
	} else {
		innerResp = PermissionDeny{
			Behavior:  "deny",
			ToolUseID: toolUseID,
			Message:   reason,
			Interrupt: false,
		}
	}

	resp := ControlResponse{
		Type: "control_response",
		Response: ControlResponseInner{
			Subtype:   "success",
			RequestID: requestID,
			Response:  innerResp,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshaling permission response")
	}
	return b.proc.Send(data)
}

func (b *Backend) handleMCPMessage(msg CLIMessage, responder core.Responder, store skills.SkillStore) error {
	var req MCPMessageRequest
	if err := json.Unmarshal(msg.Request, &req); err != nil {
		return errors.Wrap(err, "unmarshaling mcp_message request")
	}

	if req.ServerName != "discord-tools" {
		return b.sendMCPResult(msg.RequestID, req.Message.ID, struct{}{})
	}

	slog.Info("MCP message", "server", req.ServerName, "method", req.Message.Method)

	var result any
	switch req.Message.Method {
	case "initialize":
		result = MCPInitResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    MCPCapability{Tools: map[string]any{}},
			ServerInfo:      MCPServerInfo{Name: "discord-tools", Version: "1.0.0"},
		}
	case "notifications/initialized":
		result = struct{}{}
	case "tools/list":
		result = MCPToolListResult{Tools: core.MCPTools}
	case "tools/call":
		return b.handleMCPToolCall(msg.RequestID, req.Message.ID, req.Message.Params, responder, store)
	default:
		result = struct{}{}
	}

	return b.sendMCPResult(msg.RequestID, req.Message.ID, result)
}

func (b *Backend) handleMCPToolCall(requestID string, jsonrpcID any, paramsRaw json.RawMessage, responder core.Responder, store skills.SkillStore) error {
	var params MCPToolCallParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return errors.Wrap(err, "unmarshaling tool call params")
	}

	var input core.ToolInput
	if len(params.Arguments) > 0 {
		if err := json.Unmarshal(params.Arguments, &input); err != nil {
			return b.sendMCPToolError(requestID, jsonrpcID, "invalid arguments: "+err.Error())
		}
	}

	deps := tools.Deps{Responder: responder, SkillStore: store}
	result, isErr := tools.Execute(params.Name, input, deps)

	if isErr {
		return b.sendMCPToolError(requestID, jsonrpcID, result)
	}
	return b.sendMCPResult(requestID, jsonrpcID, MCPToolCallResult{
		Content: []MCPTextContent{{Type: "text", Text: result}},
	})
}

func (b *Backend) sendMCPResult(requestID string, jsonrpcID any, result any) error {
	resp := MCPResponseWrapper{
		Type: "control_response",
		Response: MCPResponseWrapperInner{
			Subtype:   "success",
			RequestID: requestID,
			Response: MCPResponse{
				MCPResponse: MCPJSONRPCResponse{
					JSONRPC: "2.0",
					ID:      jsonrpcID,
					Result:  result,
				},
			},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshaling MCP result")
	}
	return b.proc.Send(data)
}

func (b *Backend) sendMCPToolError(requestID string, jsonrpcID any, errMsg string) error {
	return b.sendMCPResult(requestID, jsonrpcID, MCPToolCallResult{
		Content: []MCPTextContent{{Type: "text", Text: errMsg}},
		IsError: true,
	})
}

// BackendFactory creates CLI backends
type BackendFactory struct {
	DefaultWorkDir string
	AllowedDirs    []string
	InitTimeout    time.Duration
	SkillStore     skills.SkillStore
	Passive        bool
}

var _ core.BackendFactory = (*BackendFactory)(nil)

func (f *BackendFactory) Create(workDir string) (core.Backend, error) {
	if workDir == "" {
		workDir = f.DefaultWorkDir
	}

	var base string
	if f.Passive {
		base = core.PassiveSystemPrompt()
	}
	systemPrompt := core.BuildSystemPrompt(base, f.SkillStore)

	proc, err := NewProcessWithSystemPrompt(workDir, "", f.InitTimeout, systemPrompt)
	if err != nil {
		return nil, err
	}
	return NewBackend(proc, f.SkillStore), nil
}
