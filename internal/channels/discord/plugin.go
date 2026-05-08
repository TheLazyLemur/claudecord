package discord

import (
	"context"
	"strings"
	"sync"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/pkg/errors"
)

const maxDiscordMessageLen = 2000

// messageEvent is the platform-agnostic shape of a Discord MessageCreate that
// the plugin's tests and adapter both speak. Real discordgo events get
// translated into this struct in Start (Task 9).
type messageEvent struct {
	AuthorID  string
	ChannelID string
	ParentID  string // populated when IsThread is true
	MessageID string
	Content   string
	IsThread  bool
	IsDM      bool
}

// Config holds the bot configuration fields needed by the plugin.
// BotID is used by the Task 9 discordgo handler to filter the bot's own messages.
type Config struct {
	Token        string
	BotID        string
	AllowedUsers []string
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

func New(cfg Config) *Plugin {
	return &Plugin{cfg: cfg, threads: newThreadRegistry()}
}

func (p *Plugin) ID() string { return "discord" }

func (p *Plugin) Capabilities() core.Capabilities {
	return core.Capabilities{Reactions: true, Typing: true}
}

func (p *Plugin) Start(ctx context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()
	// Real discordgo wiring lands in Task 9; tests drive handleMessageForTest.
	return nil
}

func (p *Plugin) Stop() error { return nil }

// handleMessageForTest is the seam tests use. Production discordgo handler
// (registered in Start by Task 9) translates a *discordgo.MessageCreate into a
// messageEvent and forwards here.
func (p *Plugin) handleMessageForTest(ev messageEvent) {
	p.handleMessage(ev)
}

func (p *Plugin) handleMessage(ev messageEvent) {
	if !p.userAllowed(ev.AuthorID) {
		return
	}
	cleaned, ok := stripMention(ev.Content)
	if !ok {
		return
	}

	threadID, err := p.resolveThread(ev)
	if err != nil {
		return
	}

	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d == nil {
		return
	}

	d(core.Inbound{
		SessionKey: sessionKey(ev, threadID),
		Text:       cleaned,
		Reply:      newOutbound(p.session, threadID, ev.MessageID, maxDiscordMessageLen),
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
	if len(t) > 50 {
		t = t[:50]
	}
	if t == "" {
		t = "Claude"
	}
	return t
}

// newPluginForTest constructs a plugin with a pre-injected session, bypassing
// the discordgo Open path. Test-only.
func newPluginForTest(s sessionForPlugin, botID string, allowed []string) *Plugin {
	p := New(Config{BotID: botID, AllowedUsers: allowed})
	p.session = s
	return p
}
