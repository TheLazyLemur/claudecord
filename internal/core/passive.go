package core

import (
	"context"
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

	backend, err := b.sessions.GetOrCreateSession()
	if err != nil {
		return errors.Wrap(err, "getting passive session")
	}

	combined := b.formatMessages(msgs)

	// Use a no-op responder since passive bot doesn't support discord tools
	responder := &noopResponder{}
	ctx := context.Background()

	response, err := backend.Converse(ctx, combined, responder, b.perms)
	if err != nil {
		return errors.Wrap(err, "conversing")
	}

	return b.maybePostResponse(channelID, msgs[0].MessageID, response)
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

// noopResponder is used by passive bot since it doesn't support discord tools
type noopResponder struct{}

func (n *noopResponder) SendTyping() error                            { return nil }
func (n *noopResponder) PostResponse(content string) error            { return nil }
func (n *noopResponder) AddReaction(emoji string) error               { return nil }
func (n *noopResponder) SendUpdate(message string) error              { return nil }
func (n *noopResponder) AskPermission(prompt string) (bool, error)    { return false, nil }
