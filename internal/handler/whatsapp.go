package handler

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/pkg/errors"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// WAClient is what WAHandler needs from the WhatsApp client wrapper
type WAClient interface {
	core.WhatsAppMessenger
	HandleIncomingReply(senderJID, text string) bool
}

// replyWaiter holds state for a pending reply wait
type replyWaiter struct {
	senderJID string
	result    chan string
}

// WhatsAppClientWrapper wraps whatsmeow.Client, implements WAClient
type WhatsAppClientWrapper struct {
	client  *whatsmeow.Client
	mu      sync.Mutex
	waiter  *replyWaiter
	timeout time.Duration
}

func NewWhatsAppClientWrapper(client *whatsmeow.Client) *WhatsAppClientWrapper {
	return &WhatsAppClientWrapper{
		client:  client,
		timeout: 60 * time.Second,
	}
}

func (c *WhatsAppClientWrapper) SendText(chatJID, text string) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return errors.Wrap(err, "parsing chat JID")
	}
	msg := &waE2E.Message{Conversation: proto.String(text)}
	_, err = c.client.SendMessage(context.Background(), jid, msg)
	return errors.Wrap(err, "sending whatsapp message")
}

func (c *WhatsAppClientWrapper) SendTyping(chatJID string) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return errors.Wrap(err, "parsing chat JID")
	}
	ctx := context.Background()
	if err := c.client.SendPresence(ctx, types.PresenceAvailable); err != nil {
		return errors.Wrap(err, "sending presence")
	}
	return errors.Wrap(
		c.client.SendChatPresence(ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText),
		"sending chat presence",
	)
}

func (c *WhatsAppClientWrapper) WaitForReply(senderJID string) (string, error) {
	c.mu.Lock()
	c.waiter = &replyWaiter{
		senderJID: senderJID,
		result:    make(chan string, 1),
	}
	w := c.waiter
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		c.waiter = nil
		c.mu.Unlock()
	}()

	select {
	case reply := <-w.result:
		return reply, nil
	case <-time.After(c.timeout):
		return "", errors.New("timeout waiting for reply")
	}
}

func (c *WhatsAppClientWrapper) HandleIncomingReply(senderJID, text string) bool {
	c.mu.Lock()
	w := c.waiter
	c.mu.Unlock()

	if w == nil {
		return false
	}
	if w.senderJID != senderJID {
		return false
	}

	select {
	case w.result <- text:
	default:
	}
	return true
}

// WAHandler handles whatsmeow events
type WAHandler struct {
	bot            BotInterface
	allowedSenders []string
	client         WAClient
}

func NewWAHandler(bot BotInterface, allowedSenders []string, client WAClient) *WAHandler {
	return &WAHandler{
		bot:            bot,
		allowedSenders: allowedSenders,
		client:         client,
	}
}

// isSenderAllowed checks if a sender matches the allowed list.
// Matches against full JID string or just the User (number) part of Sender and SenderAlt.
func (h *WAHandler) isSenderAllowed(sender, senderAlt types.JID) bool {
	for _, allowed := range h.allowedSenders {
		if sender.String() == allowed || senderAlt.String() == allowed {
			return true
		}
		if sender.User == allowed || senderAlt.User == allowed {
			return true
		}
		// strip @suffix from allowed entry for comparison
		if idx := strings.IndexByte(allowed, '@'); idx > 0 {
			num := allowed[:idx]
			if sender.User == num || senderAlt.User == num {
				return true
			}
		}
	}
	return false
}

func (h *WAHandler) HandleEvent(evt interface{}) {
	v, ok := evt.(*events.Message)
	if !ok {
		return
	}

	text := extractText(v.Message)
	if text == "" {
		return
	}

	senderJID := v.Info.Sender.String()
	chatJID := v.Info.Chat.String()

	// check sender against allowed list — match full JID or just the user/number part
	if !h.isSenderAllowed(v.Info.Sender, v.Info.SenderAlt) {
		slog.Info("unauthorized whatsapp sender", "sender", senderJID, "alt", v.Info.SenderAlt.String())
		return
	}

	// check if this is a permission flow reply (try both JID forms — LID vs phone)
	if h.client.HandleIncomingReply(senderJID, text) || h.client.HandleIncomingReply(v.Info.SenderAlt.String(), text) {
		return
	}

	if text == "!new" {
		if err := h.bot.NewSession(""); err != nil {
			slog.Error("creating new whatsapp session", "error", err)
		}
		return
	}

	// run in goroutine — HandleEvent is called on whatsmeow's single event
	// dispatch goroutine, so blocking here prevents reply events from arriving
	go func() {
		responder := core.NewWhatsAppResponder(h.client, chatJID, senderJID)
		if err := h.bot.HandleMessage(responder, text); err != nil {
			slog.Error("handling whatsapp message", "error", err)
		}
	}()
}

func extractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	text := msg.GetConversation()
	if text == "" {
		if ext := msg.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}
	}
	return text
}
