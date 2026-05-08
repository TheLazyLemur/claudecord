package discord

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

const maxDiscordMessageLen = 2000

// messageEvent is the platform-agnostic shape of a Discord MessageCreate that
// the plugin's tests and adapter both speak. Real discordgo events get
// translated into this struct in Start (Task 9).
type messageEvent struct {
	AuthorID    string
	ChannelID   string
	ParentID    string // populated when IsThread is true
	MessageID   string
	Content     string
	IsThread    bool
	IsDM        bool
	Attachments []*discordgo.MessageAttachment
}

// Config holds the bot configuration fields needed by the plugin.
// BotID is used by the Task 9 discordgo handler to filter the bot's own messages.
type Config struct {
	Token        string
	BotID        string
	AllowedUsers []string
	// MediaDir is the directory Discord attachments are saved to. When empty,
	// attachment processing is disabled.
	MediaDir string
	// Downloader fetches raw bytes from Discord CDN URLs. When nil and MediaDir
	// is set, New installs an HTTPDownloader with a 30 s timeout.
	Downloader Downloader
}

// Plugin implements core.ChannelPlugin for Discord.
type Plugin struct {
	cfg     Config
	session sessionForPlugin
	threads *threadRegistry
	mu      sync.Mutex
	deliver func(core.Inbound)
}

// sessionForPlugin is the slice of *discordgo.Session the plugin needs at
// runtime. Defined as an interface so plugin_test can mock it.
type sessionForPlugin interface {
	discordSession
	MessageThreadStartComplex(channelID, messageID, name string) (string, error)
}

// New constructs a Plugin with a caller-owned session. The production caller
// opens a real discordgo session via Connect(token), wraps it with WrapSession,
// and sets BotID from dg.State.User.ID before calling Start. Tests pass a mock
// that satisfies sessionForPlugin. Pass nil only when the plugin will be used
// solely for capability queries (e.g. probes built before connecting).
func New(cfg Config, s sessionForPlugin) *Plugin {
	if cfg.MediaDir != "" && cfg.Downloader == nil {
		cfg.Downloader = &HTTPDownloader{
			Client: &http.Client{Timeout: 30 * time.Second},
		}
	}
	return &Plugin{cfg: cfg, session: s, threads: newThreadRegistry()}
}

func (p *Plugin) ID() string { return "discord" }

func (p *Plugin) Capabilities() core.Capabilities {
	return core.Capabilities{Reactions: true, Media: p.cfg.MediaDir != "", Updates: true}
}

func (p *Plugin) Start(ctx context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()

	if p.session == nil {
		return errors.New("discord plugin started without a session")
	}
	dg, ok := p.session.(sessionAdapter)
	if !ok {
		// Test session injected — register nothing on the real discordgo session.
		return nil
	}

	dg.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		ev, ok := translateMessageCreate(m, p.cfg.BotID, dg.State.Channel)
		if !ok {
			return
		}
		p.handleMessage(ev)
	})

	return nil
}

// translateMessageCreate converts a discordgo MessageCreate into the
// platform-agnostic messageEvent the plugin's handleMessage consumes.
// lookupChannel allows the State lookup to be stubbed in tests.
func translateMessageCreate(m *discordgo.MessageCreate, botID string, lookupChannel func(string) (*discordgo.Channel, error)) (messageEvent, bool) {
	if m.Author == nil || m.Author.ID == botID {
		return messageEvent{}, false
	}
	ev := messageEvent{
		AuthorID:    m.Author.ID,
		ChannelID:   m.ChannelID,
		MessageID:   m.ID,
		Content:     m.Content,
		Attachments: m.Attachments,
	}
	// Discord delivers DMs with GuildID == "".
	if m.GuildID == "" {
		ev.IsDM = true
	}
	// Channel state tells us whether we're inside a thread.
	if lookupChannel != nil {
		if ch, err := lookupChannel(m.ChannelID); err == nil && ch.IsThread() {
			ev.IsThread = true
			ev.ParentID = ch.ParentID
		}
	}
	return ev, true
}

func (p *Plugin) Stop() error {
	if dg, ok := p.session.(sessionAdapter); ok && dg.Session != nil {
		return dg.Session.Close()
	}
	return nil
}

func (p *Plugin) handleMessage(ev messageEvent) {
	if !p.userAllowed(ev.AuthorID) {
		return
	}
	cleaned, ok := stripMention(ev.Content)
	if !ok {
		return
	}

	// Extract attachments when media processing is configured.
	var refs []core.AttachmentRef
	if p.cfg.MediaDir != "" && p.cfg.Downloader != nil && len(ev.Attachments) > 0 {
		mc := &discordgo.MessageCreate{Message: &discordgo.Message{Attachments: ev.Attachments}}
		var skipped []string
		var err error
		refs, skipped, err = extractAttachments(mc, p.cfg.Downloader, p.cfg.MediaDir)
		if err != nil {
			slog.Error("extracting discord attachments", "error", err)
		}
		if len(skipped) > 0 {
			notice := strings.Join(skipped, "\n")
			if cleaned == "" {
				cleaned = notice
			} else {
				cleaned = notice + "\n" + cleaned
			}
		}
	}

	threadID, err := p.resolveThread(ev)
	if err != nil {
		if reactErr := p.session.MessageReactionAdd(ev.ChannelID, ev.MessageID, "❌"); reactErr != nil {
			slog.Warn("discord reaction add failed", "channel", ev.ChannelID, "message", ev.MessageID, "error", reactErr)
		}
		return
	}

	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d == nil {
		return
	}

	d(core.Inbound{
		SessionKey:   sessionKey(ev, threadID),
		Text:         cleaned,
		Attachments:  refs,
		Reply:        newOutbound(p.session, threadID, ev.MessageID, maxDiscordMessageLen),
		Capabilities: p.Capabilities(),
	})
}

func (p *Plugin) resolveThread(ev messageEvent) (string, error) {
	if ev.IsDM {
		return ev.ChannelID, nil
	}
	if ev.IsThread && p.threads.owns(ev.ChannelID) {
		return ev.ChannelID, nil
	}
	parent := ev.ChannelID
	if ev.IsThread {
		parent = ev.ParentID
	}
	tid, err := p.session.MessageThreadStartComplex(parent, ev.MessageID, threadName(ev.Content))
	if err != nil {
		slog.Warn("discord create thread failed", "channel", parent, "message", ev.MessageID, "error", err)
		return "", errors.Wrap(err, "creating thread")
	}
	p.threads.markOwned(tid)
	return tid, nil
}

func sessionKey(ev messageEvent, threadID string) core.SessionKey {
	if ev.IsDM {
		return core.SessionKey("discord:dm:" + ev.AuthorID)
	}
	return core.SessionKey("discord:thread:" + threadID)
}

func (p *Plugin) userAllowed(userID string) bool {
	for _, u := range p.cfg.AllowedUsers {
		if u == userID {
			return true
		}
	}
	return false
}

func stripMention(content string) (string, bool) {
	const tag = "@claude"
	trim := strings.TrimSpace(content)
	if !strings.HasPrefix(strings.ToLower(trim), tag) {
		return "", false
	}
	return strings.TrimSpace(trim[len(tag):]), true
}

func threadName(content string) string {
	t := strings.TrimSpace(content)
	if r := []rune(t); len(r) > 50 {
		t = string(r[:50])
	}
	if t == "" {
		t = "Claude"
	}
	return t
}

// WrapSession wraps a *discordgo.Session into the sessionForPlugin interface
// that Plugin expects. Used by the production callsite after Connect returns.
func WrapSession(dg *discordgo.Session) sessionForPlugin {
	return sessionAdapter{dg}
}

// sessionAdapter wraps *discordgo.Session to satisfy sessionForPlugin.
type sessionAdapter struct{ *discordgo.Session }

func (s sessionAdapter) ChannelMessageSend(channelID, content string) error {
	_, err := s.Session.ChannelMessageSend(channelID, content)
	return err
}

func (s sessionAdapter) ChannelTyping(channelID string) error {
	return s.Session.ChannelTyping(channelID)
}

func (s sessionAdapter) MessageReactionAdd(channelID, messageID, emoji string) error {
	return s.Session.MessageReactionAdd(channelID, messageID, emoji)
}

func (s sessionAdapter) MessageThreadStartComplex(channelID, messageID, name string) (string, error) {
	t, err := s.Session.MessageThreadStartComplex(channelID, messageID, &discordgo.ThreadStart{
		Name:                name,
		AutoArchiveDuration: 60,
	})
	if err != nil {
		return "", err
	}
	return t.ID, nil
}
