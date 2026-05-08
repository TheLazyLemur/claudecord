package dashboard

import (
	"context"
	"sync"

	"github.com/TheLazyLemur/claudecord/internal/core"
	dash "github.com/TheLazyLemur/claudecord/internal/dashboard"
)

// ChatCallbackSetter is implemented by any server that accepts a chat
// callback registration. The dashboard Server satisfies this interface.
type ChatCallbackSetter interface {
	SetChatCallback(func(sessionID, text string))
}

// Config holds dependencies for the dashboard plugin.
type Config struct {
	Hub    *dash.Hub
	Server ChatCallbackSetter
}

// Plugin implements core.ChannelPlugin for the dashboard WebSocket interface.
type Plugin struct {
	cfg     Config
	mu      sync.Mutex
	deliver func(core.Inbound)
}

// New constructs a Plugin from cfg.
func New(cfg Config) *Plugin { return &Plugin{cfg: cfg} }

func (p *Plugin) ID() string { return "dashboard" }

func (p *Plugin) Capabilities() core.Capabilities {
	return core.Capabilities{Reactions: false, Updates: true}
}

func (p *Plugin) Start(_ context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()
	if p.cfg.Server != nil {
		p.cfg.Server.SetChatCallback(p.HandleChat)
	}
	return nil
}

func (p *Plugin) Stop() error { return nil }

// HandleChat is the entry point called by the dashboard Server when a chat
// message arrives. It constructs the Inbound and dispatches it to deliver.
//
// The sessionID parameter identifies the active backend session for the WS
// responder only. It is intentionally NOT folded into the SessionKey: the
// backend's SessionID rotates on every NewSession, so using it for routing
// would make the bot treat every dashboard message as a fresh session.
func (p *Plugin) HandleChat(sessionID, text string) {
	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d == nil {
		return
	}

	var out core.Outbound
	if p.cfg.Hub != nil {
		out = dash.NewWSResponder(p.cfg.Hub, sessionID)
	}

	d(core.Inbound{
		SessionKey:   SessionKey(),
		Text:         text,
		Reply:        out,
		Capabilities: p.Capabilities(),
	})
}

// SessionKey returns the stable dashboard session key. The dashboard is
// single-user/single-channel so one constant key is correct.
func SessionKey() core.SessionKey {
	return core.SessionKey("dashboard")
}
