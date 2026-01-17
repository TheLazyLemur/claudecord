package core

import (
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/pkg/errors"
)

const maxDiscordMessageLen = 2000

// Bot orchestrates CLI sessions, Discord posting, and permission checks
type Bot struct {
	sessions *SessionManager
	discord  DiscordClient
	perms    PermissionChecker
	mu       sync.Mutex // serialize message handling
}

// NewBot creates a bot with the given dependencies
func NewBot(sessions *SessionManager, discord DiscordClient, perms PermissionChecker) *Bot {
	return &Bot{
		sessions: sessions,
		discord:  discord,
		perms:    perms,
	}
}

// HandleMessage processes a Discord message, sends to CLI, handles responses
func (b *Bot) HandleMessage(channelID, messageID, userMessage string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	slog.Info("HandleMessage start", "msg", userMessage)
	b.discord.SendTyping(channelID)

	slog.Info("getting session")
	proc, err := b.sessions.GetOrCreateSession()
	if err != nil {
		return errors.Wrap(err, "getting session")
	}
	slog.Info("got session", "sessionID", proc.SessionID())

	slog.Info("sending user message to CLI")
	if err := b.sendUserMessage(proc, userMessage); err != nil {
		return errors.Wrap(err, "sending user message")
	}
	slog.Info("sent user message, processing responses")

	return b.processResponses(proc, channelID, messageID)
}

// NewSession starts a fresh CLI session with optional working directory
func (b *Bot) NewSession(workDir string) error {
	return b.sessions.NewSession(workDir)
}

func (b *Bot) sendUserMessage(proc CLIProcess, content string) error {
	msg := map[string]any{
		"type":       "user",
		"session_id": proc.SessionID(),
		"message": map[string]any{
			"role":    "user",
			"content": content,
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return errors.Wrap(err, "marshaling user message")
	}
	return proc.Send(data)
}

func (b *Bot) processResponses(proc CLIProcess, channelID, messageID string) error {
	recvChan, err := proc.Receive()
	if err != nil {
		return errors.Wrap(err, "receiving")
	}

	var responseText string

	for {
		data, ok := <-recvChan
		if !ok {
			// channel closed, process ended
			break
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
			if err := b.handleControlRequest(proc, msg, channelID, messageID); err != nil {
				return errors.Wrap(err, "handling control request")
			}

		case "result":
			// turn complete, post response if any
			slog.Info("got result, posting response", "len", len(responseText))
			if responseText != "" {
				if err := b.postResponse(channelID, responseText); err != nil {
					return errors.Wrap(err, "posting response")
				}
			}
			return nil
		}
	}

	return nil
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

func (b *Bot) handleControlRequest(proc CLIProcess, msg map[string]any, channelID, messageID string) error {
	requestID, _ := msg["request_id"].(string)
	request, ok := msg["request"].(map[string]any)
	if !ok {
		return nil
	}

	subtype, _ := request["subtype"].(string)
	switch subtype {
	case "can_use_tool":
		toolName, _ := request["tool_name"].(string)
		toolUseID, _ := request["tool_use_id"].(string)
		input, _ := request["input"].(map[string]any)
		allow, reason := b.perms.Check(toolName, input)
		return b.sendPermissionResponse(proc, requestID, toolUseID, allow, reason, input)

	case "mcp_message":
		return b.handleMCPMessage(proc, requestID, request, channelID, messageID)
	}

	return nil
}

func (b *Bot) sendPermissionResponse(proc CLIProcess, requestID, toolUseID string, allow bool, reason string, input map[string]any) error {
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
	return proc.Send(data)
}

func (b *Bot) postResponse(channelID, content string) error {
	if len(content) > maxDiscordMessageLen {
		_, err := b.discord.CreateThread(channelID, content)
		return err
	}
	return b.discord.SendMessage(channelID, content)
}

// MCP error codes
const (
	mcpErrInvalidRequest = -32600
	mcpErrMethodNotFound = -32601
	mcpErrInvalidParams  = -32602
	mcpErrServerError    = -32000
)

func (b *Bot) handleMCPMessage(proc CLIProcess, requestID string, request map[string]any, channelID, messageID string) error {
	serverName, _ := request["server_name"].(string)
	message, _ := request["message"].(map[string]any)
	jsonrpcID, _ := message["id"].(float64)

	if serverName != "discord-tools" {
		return b.sendMCPError(proc, requestID, int(jsonrpcID), mcpErrInvalidRequest, "unknown server: "+serverName)
	}

	method, _ := message["method"].(string)
	if method != "tools/call" {
		return b.sendMCPError(proc, requestID, int(jsonrpcID), mcpErrMethodNotFound, "unknown method: "+method)
	}

	params, _ := message["params"].(map[string]any)
	return b.handleMCPToolCall(proc, requestID, int(jsonrpcID), params, channelID, messageID)
}

func (b *Bot) handleMCPToolCall(proc CLIProcess, requestID string, jsonrpcID int, params map[string]any, channelID, messageID string) error {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	if toolName != "react_emoji" {
		return b.sendMCPError(proc, requestID, jsonrpcID, mcpErrMethodNotFound, "unknown tool: "+toolName)
	}

	emoji, ok := args["emoji"].(string)
	if !ok || emoji == "" {
		return b.sendMCPError(proc, requestID, jsonrpcID, mcpErrInvalidParams, "missing emoji argument")
	}

	if err := b.discord.AddReaction(channelID, messageID, emoji); err != nil {
		return b.sendMCPError(proc, requestID, jsonrpcID, mcpErrServerError, err.Error())
	}

	return b.sendMCPToolResult(proc, requestID, jsonrpcID, "reaction added")
}

func (b *Bot) sendMCPToolResult(proc CLIProcess, requestID string, jsonrpcID int, text string) error {
	resp := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response": map[string]any{
				"jsonrpc": "2.0",
				"id":      jsonrpcID,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": text},
					},
				},
			},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshaling MCP result")
	}
	return proc.Send(data)
}

func (b *Bot) sendMCPError(proc CLIProcess, requestID string, jsonrpcID int, code int, message string) error {
	resp := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response": map[string]any{
				"jsonrpc": "2.0",
				"id":      jsonrpcID,
				"error": map[string]any{
					"code":    code,
					"message": message,
				},
			},
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return errors.Wrap(err, "marshaling MCP error")
	}
	return proc.Send(data)
}
