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
	var threadID string // tracks thread for send_update calls

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
			if err := b.handleControlRequest(proc, msg, channelID, messageID, &threadID); err != nil {
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

func (b *Bot) handleControlRequest(proc CLIProcess, msg map[string]any, channelID, messageID string, threadID *string) error {
	requestID, _ := msg["request_id"].(string)
	request, ok := msg["request"].(map[string]any)
	if !ok {
		return nil
	}

	subtype, _ := request["subtype"].(string)
	slog.Info("control_request", "subtype", subtype, "request_id", requestID)
	switch subtype {
	case "can_use_tool":
		toolName, _ := request["tool_name"].(string)
		toolUseID, _ := request["tool_use_id"].(string)
		input, _ := request["input"].(map[string]any)
		allow, reason := b.perms.Check(toolName, input)
		return b.sendPermissionResponse(proc, requestID, toolUseID, allow, reason, input)

	case "mcp_message":
		return b.handleMCPMessage(proc, requestID, request, channelID, messageID, threadID)
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

// MCPTools defines the MCP tool schema for discord-tools server
var MCPTools = []map[string]any{
	{
		"name":        "react_emoji",
		"description": "Add emoji reaction to current Discord message. Call this first when you receive a message.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"emoji": map[string]any{
					"type":        "string",
					"description": "Unicode emoji character (e.g. 👀, 👍, 🚀)",
				},
			},
			"required": []string{"emoji"},
		},
	},
	{
		"name":        "send_update",
		"description": "Send a progress update message to a thread on the original Discord message. Use this to keep the user informed about what you're doing.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "The update message to send",
				},
			},
			"required": []string{"message"},
		},
	},
}

func (b *Bot) handleMCPMessage(proc CLIProcess, requestID string, request map[string]any, channelID, messageID string, threadID *string) error {
	serverName, _ := request["server_name"].(string)
	message, _ := request["message"].(map[string]any)
	jsonrpcID := message["id"] // can be number or string
	method, _ := message["method"].(string)

	if serverName != "discord-tools" {
		return b.sendMCPResult(proc, requestID, jsonrpcID, map[string]any{})
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
		result = map[string]any{"tools": MCPTools}
	case "tools/call":
		params, _ := message["params"].(map[string]any)
		return b.handleMCPToolCall(proc, requestID, jsonrpcID, params, channelID, messageID, threadID)
	default:
		result = map[string]any{}
	}

	return b.sendMCPResult(proc, requestID, jsonrpcID, result)
}

func (b *Bot) handleMCPToolCall(proc CLIProcess, requestID string, jsonrpcID any, params map[string]any, channelID, messageID string, threadID *string) error {
	toolName, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch toolName {
	case "react_emoji":
		emoji, ok := args["emoji"].(string)
		if !ok || emoji == "" {
			return b.sendMCPToolError(proc, requestID, jsonrpcID, "missing emoji argument")
		}
		slog.Info("AddReaction", "channelID", channelID, "messageID", messageID, "emoji", emoji)
		if err := b.discord.AddReaction(channelID, messageID, emoji); err != nil {
			slog.Error("AddReaction failed", "error", err)
			return b.sendMCPToolError(proc, requestID, jsonrpcID, err.Error())
		}
		return b.sendMCPResult(proc, requestID, jsonrpcID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "reaction added"}},
		})

	case "send_update":
		msg, ok := args["message"].(string)
		if !ok || msg == "" {
			return b.sendMCPToolError(proc, requestID, jsonrpcID, "missing message argument")
		}
		// create thread if needed
		if *threadID == "" {
			tid, err := b.discord.StartThread(channelID, messageID, "Updates")
			if err != nil {
				slog.Error("StartThread failed", "error", err)
				return b.sendMCPToolError(proc, requestID, jsonrpcID, err.Error())
			}
			*threadID = tid
			slog.Info("Created thread", "threadID", tid)
		}
		// send message to thread
		if err := b.discord.SendMessage(*threadID, msg); err != nil {
			slog.Error("SendMessage to thread failed", "error", err)
			return b.sendMCPToolError(proc, requestID, jsonrpcID, err.Error())
		}
		return b.sendMCPResult(proc, requestID, jsonrpcID, map[string]any{
			"content": []map[string]any{{"type": "text", "text": "update sent"}},
		})

	default:
		return b.sendMCPToolError(proc, requestID, jsonrpcID, "unknown tool: "+toolName)
	}
}

func (b *Bot) sendMCPResult(proc CLIProcess, requestID string, jsonrpcID any, result any) error {
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
	return proc.Send(data)
}

func (b *Bot) sendMCPToolError(proc CLIProcess, requestID string, jsonrpcID any, errMsg string) error {
	return b.sendMCPResult(proc, requestID, jsonrpcID, map[string]any{
		"content": []map[string]any{{"type": "text", "text": errMsg}},
		"isError": true,
	})
}
