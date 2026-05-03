package handler

import (
	"context"
	"fmt"
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

// DefaultBurstDelay is how long the WA handler waits for additional messages
// in the same chat before dispatching the batch to the bot.
const DefaultBurstDelay = 3 * time.Second

// WAClient is what WAHandler needs from the WhatsApp client wrapper.
type WAClient interface {
	core.WhatsAppMessenger
	Download(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error)
}

// WhatsAppClientWrapper wraps whatsmeow.Client, implements WAClient
type WhatsAppClientWrapper struct {
	client *whatsmeow.Client
}

func NewWhatsAppClientWrapper(client *whatsmeow.Client) *WhatsAppClientWrapper {
	return &WhatsAppClientWrapper{client: client}
}

func (c *WhatsAppClientWrapper) Download(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error) {
	return c.client.Download(ctx, msg)
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

// WAHandler handles whatsmeow events.
// Inbound messages are debounced per chat JID and dispatched to the bot as a
// single rendered prompt; attachments are decrypted into mediaDir before the
// flush fires.
type WAHandler struct {
	bot            BotInterface
	allowedSenders []string
	client         WAClient
	mediaDir       string
	buffer         *core.DebouncedBuffer
	now            func() time.Time

	// senderByChat tracks which sender should receive the bot's reply for a
	// given chat. Latest wins (set on each enqueue, read on flush).
	mu           sync.Mutex
	senderByChat map[string]string
}

func NewWAHandler(bot BotInterface, allowedSenders []string, client WAClient, mediaDir string) *WAHandler {
	h := &WAHandler{
		bot:            bot,
		allowedSenders: allowedSenders,
		client:         client,
		mediaDir:       mediaDir,
		now:            time.Now,
		senderByChat:   make(map[string]string),
	}
	h.buffer = core.NewDebouncedBuffer(DefaultBurstDelay, h.flush)
	return h
}

// SetBurstDelay overrides the default 3s debounce. Tests use this to fire
// flushes synchronously without waiting on real time.
func (h *WAHandler) SetBurstDelay(d time.Duration) {
	h.buffer.Stop()
	h.buffer = core.NewDebouncedBuffer(d, h.flush)
}

// Stop drains pending timers; called from main on shutdown.
func (h *WAHandler) Stop() {
	if h.buffer != nil {
		h.buffer.Stop()
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

	senderJID := v.Info.Sender.String()
	chatJID := v.Info.Chat.String()

	if !h.isSenderAllowed(v.Info.Sender, v.Info.SenderAlt) {
		slog.Info("unauthorized whatsapp sender", "sender", senderJID, "alt", v.Info.SenderAlt.String())
		return
	}

	plainText := extractText(v.Message)
	if plainText == "!new" {
		if err := h.bot.NewSession(""); err != nil {
			slog.Error("creating new whatsapp session", "error", err)
		}
		return
	}

	// Synchronous: ordering of decrypted attachments must match send order
	// inside the burst, so we don't fan out to per-message goroutines here.
	// The download blocks the whatsmeow dispatch goroutine briefly; for
	// realistic single-user traffic that's fine.
	caption, att, err := extractInbound(context.Background(), v, h.client)
	if err != nil {
		slog.Error("extracting inbound whatsapp media", "error", err)
		return
	}

	var attachments []core.AttachmentRef
	if att != nil {
		if len(att.Bytes) > SizeCap(att.MIME) {
			label := att.OriginalName
			if label == "" {
				label = att.MIME
			}
			if err := h.client.SendText(chatJID, "skipped (too large): "+label); err != nil {
				slog.Error("sending size-cap skip notice", "error", err)
			}
		} else {
			path, err := saveAttachment(h.mediaDir, att, h.now())
			if err != nil {
				slog.Error("saving whatsapp attachment", "error", err)
				return
			}
			attachments = []core.AttachmentRef{{
				Path:         path,
				MIME:         att.MIME,
				OriginalName: att.OriginalName,
			}}
		}
	}

	if caption == "" && len(attachments) == 0 {
		return
	}

	h.mu.Lock()
	h.senderByChat[chatJID] = senderJID
	h.mu.Unlock()

	h.buffer.Add(core.BufferedMessage{
		ChannelID:   chatJID,
		Content:     caption,
		AuthorID:    senderJID,
		Attachments: attachments,
	})
}

// flush is the DebouncedBuffer callback: render → dispatch to bot.
func (h *WAHandler) flush(chatJID string, msgs []core.BufferedMessage) {
	h.mu.Lock()
	senderJID := h.senderByChat[chatJID]
	delete(h.senderByChat, chatJID)
	h.mu.Unlock()

	if senderJID == "" && len(msgs) > 0 {
		senderJID = msgs[len(msgs)-1].AuthorID
	}

	prompt := core.RenderWhatsAppBatch(msgs)
	if prompt == "" {
		return
	}

	responder := core.NewWhatsAppResponder(h.client, chatJID, senderJID)
	if err := h.bot.HandleMessage(responder, prompt); err != nil {
		slog.Error("handling whatsapp batch", "error", err, "chat", chatJID, "msgs", fmt.Sprintf("%d", len(msgs)))
	}
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
