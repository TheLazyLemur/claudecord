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
	msg := map[string]any{
		"type":       "user",
		"session_id": b.proc.SessionID(),
		"message": map[string]any{
			"role":    "user",
			"content": content,
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

			var msg map[string]any
			if err := json.Unmarshal(data, &msg); err != nil {
				continue
			}

			// skip replay messages
			if isReplay, ok := msg["isReplay"].(bool); ok && isReplay {
				continue
			}

			msgType, _ := msg["type"].(string)
			slog.Info("CLI response", "type", msgType)

			switch msgType {
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

func extractTextFromAssistant(msg map[string]any) string {
	message, ok := msg["message"].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := message["content"].([]any)
	if !ok {
		return ""
	}

	var text string
	for _, block := range content {
		b, ok := block.(map[string]any)
		if !ok {
			continue
		}
		if b["type"] == "text" {
			if t, ok := b["text"].(string); ok {
				text += t
			}
		}
	}
	return text
}

func (b *Backend) handleControlRequest(msg map[string]any, responder core.Responder, perms core.PermissionChecker) error {
	requestID, _ := msg["request_id"].(string)
	request, ok := msg["request"].(map[string]any)
	if !ok {
		return nil
	}

	subtype, _ := request["subtype"].(string)
	slog.Info("control_request", "subtype", subtype, "request_id", requestID)

	switch subtype {
	case "can_use_tool":
		return b.handleCanUseTool(requestID, request, responder, perms)
	case "mcp_message":
		return b.handleMCPMessage(requestID, request, responder, b.skillStore)
	}

	return nil
}

func (b *Backend) handleCanUseTool(requestID string, request map[string]any, responder core.Responder, perms core.PermissionChecker) error {
	toolName, _ := request["tool_name"].(string)
	toolUseID, _ := request["tool_use_id"].(string)
	input, _ := request["input"].(map[string]any)
	allow, reason := tools.CheckPermission(toolName, input, perms, responder)

	return b.sendPermissionResponse(requestID, toolUseID, allow, reason, input)
}

func (b *Backend) sendPermissionResponse(requestID, toolUseID string, allow bool, reason string, input map[string]any) error {
	var innerResp map[string]any
	if allow {
		innerResp = map[string]any{
			"behavior":     "allow",
			"toolUseID":    toolUseID,
			"updatedInput": input,
		}
	} else {
		innerResp = map[string]any{
			"behavior":  "deny",
			"toolUseID": toolUseID,
			"message":   reason,
			"interrupt": false,
		}
	}

	resp := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response":   innerResp,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshaling permission response")
	}
	return b.proc.Send(data)
}

func (b *Backend) handleMCPMessage(requestID string, request map[string]any, responder core.Responder, store skills.SkillStore) error {
	serverName, _ := request["server_name"].(string)
	message, _ := request["message"].(map[string]any)
	jsonrpcID := message["id"]
	method, _ := message["method"].(string)

	if serverName != "discord-tools" {
		return b.sendMCPResult(requestID, jsonrpcID, map[string]any{})
	}

	slog.Info("MCP message", "server", serverName, "method", method)

	var result any
	switch method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "discord-tools", "version": "1.0.0"},
		}
	case "notifications/initialized":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": core.MCPTools}
	case "tools/call":
		params, _ := message["params"].(map[string]any)
		return b.handleMCPToolCall(requestID, jsonrpcID, params, responder, store)
	default:
		result = map[string]any{}
	}

	return b.sendMCPResult(requestID, jsonrpcID, result)
}

func (b *Backend) handleMCPToolCall(requestID string, jsonrpcID any, params map[string]any, responder core.Responder, store skills.SkillStore) error {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	deps := tools.Deps{Responder: responder, SkillStore: store}
	result, isErr := tools.Execute(toolName, args, deps)

	if isErr {
		return b.sendMCPToolError(requestID, jsonrpcID, result)
	}
	return b.sendMCPResult(requestID, jsonrpcID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": result}},
	})
}

func (b *Backend) sendMCPResult(requestID string, jsonrpcID any, result any) error {
	resp := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response": map[string]any{
				"mcp_response": map[string]any{
					"jsonrpc": "2.0",
					"id":      jsonrpcID,
					"result":  result,
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
	return b.sendMCPResult(requestID, jsonrpcID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": errMsg}},
		"isError": true,
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
