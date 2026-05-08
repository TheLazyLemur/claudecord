package whatsapp

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/media"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// DefaultBurstDelay is how long the plugin waits for additional messages in
// the same chat before dispatching the batch to the bot.
const DefaultBurstDelay = 3 * time.Second

// Config holds dependencies for the WhatsApp plugin.
type Config struct {
	// Messenger handles outbound text/typing to WhatsApp.
	Messenger core.WhatsAppMessenger
	// Downloader handles media decryption. May be nil when media support is
	// not needed (e.g. in unit tests that only test text paths).
	Downloader     Downloader
	AllowedSenders []string
	MediaDir       string
}

// Plugin implements core.ChannelPlugin for WhatsApp.
type Plugin struct {
	cfg     Config
	mu      sync.Mutex
	deliver func(core.Inbound)
	buffer  *core.DebouncedBuffer
	now     func() time.Time
}

// New constructs a Plugin from cfg.
func New(cfg Config) *Plugin {
	p := &Plugin{cfg: cfg, now: time.Now}
	p.buffer = core.NewDebouncedBuffer(DefaultBurstDelay, p.flush)
	return p
}

func (p *Plugin) ID() string { return "whatsapp" }

func (p *Plugin) Capabilities() core.Capabilities {
	return core.Capabilities{Reactions: false}
}

func (p *Plugin) Start(ctx context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Stop() error {
	if p.buffer != nil {
		p.buffer.Stop()
	}
	return nil
}

// HandleEvent is registered with whatsmeow as the event handler.
func (p *Plugin) HandleEvent(evt interface{}) {
	v, ok := evt.(*events.Message)
	if !ok {
		return
	}

	senderJID := v.Info.Sender.String()
	chatJID := v.Info.Chat.String()

	if !p.isSenderAllowed(v.Info.Sender, v.Info.SenderAlt) {
		slog.Info("unauthorized whatsapp sender", "sender", senderJID, "alt", v.Info.SenderAlt.String())
		return
	}

	caption, att, err := ExtractInbound(context.Background(), v, p.cfg.Downloader)
	if err != nil {
		slog.Error("extracting inbound whatsapp media", "error", err)
		return
	}

	attachments, ok := p.materializeAttachment(chatJID, att)
	if !ok {
		return
	}
	if caption == "" && len(attachments) == 0 {
		return
	}

	p.buffer.Add(core.BufferedMessage{
		ChannelID:   chatJID,
		Content:     caption,
		AuthorID:    senderJID,
		Attachments: attachments,
	})
}

func (p *Plugin) isSenderAllowed(sender, senderAlt types.JID) bool {
	for _, allowed := range p.cfg.AllowedSenders {
		if sender.String() == allowed || senderAlt.String() == allowed {
			return true
		}
		if sender.User == allowed || senderAlt.User == allowed {
			return true
		}
		if idx := strings.IndexByte(allowed, '@'); idx > 0 {
			num := allowed[:idx]
			if sender.User == num || senderAlt.User == num {
				return true
			}
		}
	}
	return false
}

func (p *Plugin) materializeAttachment(chatJID string, att *media.Attachment) ([]core.AttachmentRef, bool) {
	if att == nil {
		return nil, true
	}
	if len(att.Bytes) > media.SizeCap(att.MIME) {
		label := att.OriginalName
		if label == "" {
			label = att.MIME
		}
		if err := p.cfg.Messenger.SendText(chatJID, "skipped (too large): "+label); err != nil {
			slog.Error("sending size-cap skip notice", "error", err)
		}
		return nil, true
	}
	path, err := media.SaveAttachment(p.cfg.MediaDir, att, p.now())
	if err != nil {
		slog.Error("saving whatsapp attachment", "error", err)
		return nil, false
	}
	return []core.AttachmentRef{{
		Path:         path,
		MIME:         att.MIME,
		OriginalName: att.OriginalName,
	}}, true
}

func (p *Plugin) flush(chatJID string, msgs []core.BufferedMessage) {
	if len(msgs) == 0 {
		return
	}

	prompt := core.RenderWhatsAppBatch(msgs)
	if prompt == "" {
		return
	}

	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d == nil {
		return
	}

	out := NewOutbound(p.cfg.Messenger, chatJID)
	d(core.Inbound{
		SessionKey:   SessionKey(chatJID),
		Text:         prompt,
		Reply:        out,
		Capabilities: p.Capabilities(),
	})
}

// deliverForTest is the test seam used to inject inbounds directly.
func (p *Plugin) deliverForTest(in core.Inbound) {
	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d != nil {
		d(in)
	}
}

// setBurstDelay overrides the default debounce delay. Tests use this.
func (p *Plugin) setBurstDelay(d time.Duration) {
	p.buffer.Stop()
	p.buffer = core.NewDebouncedBuffer(d, p.flush)
}

// SessionKey returns the canonical session key for a chat JID.
func SessionKey(chatJID string) core.SessionKey {
	return core.SessionKey("whatsapp:" + chatJID)
}
