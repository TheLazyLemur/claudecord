package core

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/pkg/errors"
)

const passiveSystemPrompt = `You are a helpful programming assistant passively listening to a Discord channel.

You will receive messages from the channel. Your job is to determine if any messages contain programming or technical questions that you can help with.

IMPORTANT RULES:
1. Only respond if you are confident the messages contain a programming/technical question
2. If unsure or if messages are just casual chat, respond with exactly: [NO_RESPONSE]
3. Keep answers concise and helpful
4. You are in READ-ONLY mode - you can read files but CANNOT write, edit, or execute commands
5. Focus on explaining concepts, answering questions, and pointing to relevant documentation

If you decide to respond, provide a helpful answer. If not, respond with [NO_RESPONSE] (exactly this text, nothing else).`

const noResponseMarker = "[NO_RESPONSE]"

type PassiveBot struct {
	sessions *SessionManager
	discord  DiscordClient
	perms    PermissionChecker
	mu       sync.Mutex
}

func NewPassiveBot(sessions *SessionManager, discord DiscordClient, perms PermissionChecker) *PassiveBot {
	return &PassiveBot{
		sessions: sessions,
		discord:  discord,
		perms:    perms,
	}
}

func (b *PassiveBot) HandleBufferedMessages(channelID string, msgs []BufferedMessage) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(msgs) == 0 {
		return nil
	}

	slog.Info("PassiveBot handling messages", "channel", channelID, "count", len(msgs))

	proc, err := b.sessions.GetOrCreateSession()
	if err != nil {
		return errors.Wrap(err, "getting passive session")
	}

	combined := b.formatMessages(msgs)
	if err := b.sendUserMessage(proc, combined); err != nil {
		return errors.Wrap(err, "sending to passive session")
	}

	return b.processResponses(proc, channelID, msgs[0].MessageID)
}

func (b *PassiveBot) formatMessages(msgs []BufferedMessage) string {
	var sb strings.Builder
	for i, m := range msgs {
		if i > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("[%s]: %s", m.AuthorID, m.Content))
	}
	return sb.String()
}

func (b *PassiveBot) sendUserMessage(proc CLIProcess, content string) error {
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

func (b *PassiveBot) processResponses(proc CLIProcess, channelID, firstMessageID string) error {
	recvChan, err := proc.Receive()
	if err != nil {
		return errors.Wrap(err, "receiving")
	}

	var responseText string

	for {
		data, ok := <-recvChan
		if !ok {
			break
		}

		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}

		if isReplay, ok := msg["isReplay"].(bool); ok && isReplay {
			continue
		}

		msgType, _ := msg["type"].(string)

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
			if err := b.handleControlRequest(proc, msg); err != nil {
				return errors.Wrap(err, "handling control request")
			}

		case "result":
			slog.Info("PassiveBot got result", "responseLen", len(responseText))
			return b.maybePostResponse(channelID, firstMessageID, responseText)
		}
	}

	return nil
}

func (b *PassiveBot) handleControlRequest(proc CLIProcess, msg map[string]any) error {
	requestID, _ := msg["request_id"].(string)
	request, ok := msg["request"].(map[string]any)
	if !ok {
		return nil
	}

	subtype, _ := request["subtype"].(string)
	if subtype != "can_use_tool" {
		return nil
	}

	toolName, _ := request["tool_name"].(string)
	toolUseID, _ := request["tool_use_id"].(string)
	input, _ := request["input"].(map[string]any)

	allow, reason := b.perms.Check(toolName, input)
	return b.sendPermissionResponse(proc, requestID, toolUseID, allow, reason, input)
}

func (b *PassiveBot) sendPermissionResponse(proc CLIProcess, requestID, toolUseID string, allow bool, reason string, input map[string]any) error {
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

func (b *PassiveBot) maybePostResponse(channelID, firstMessageID, response string) error {
	response = strings.TrimSpace(response)
	if response == "" || response == noResponseMarker {
		slog.Info("PassiveBot: no response needed")
		return nil
	}

	threadID, err := b.discord.StartThread(channelID, firstMessageID, "Help")
	if err != nil {
		return errors.Wrap(err, "starting thread")
	}

	if len(response) > maxDiscordMessageLen {
		for len(response) > 0 {
			chunk := response
			if len(chunk) > maxDiscordMessageLen {
				chunk = response[:maxDiscordMessageLen]
				response = response[maxDiscordMessageLen:]
			} else {
				response = ""
			}
			if err := b.discord.SendMessage(threadID, chunk); err != nil {
				return errors.Wrap(err, "sending to thread")
			}
		}
		return nil
	}

	return b.discord.SendMessage(threadID, response)
}

func (b *PassiveBot) NewSession() error {
	return b.sessions.NewSession("")
}

// PassiveSystemPrompt returns the system prompt for passive sessions
func PassiveSystemPrompt() string {
	return passiveSystemPrompt
}
