package dashboard

import (
	"context"
	"sync"

	"github.com/TheLazyLemur/claudecord/internal/core"
	dash "github.com/TheLazyLemur/claudecord/internal/dashboard"
)

// Config holds dependencies for the dashboard plugin.
type Config struct {
	Hub *dash.Hub
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
	return core.Capabilities{Typing: true, Reactions: false}
}

func (p *Plugin) Start(_ context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Stop() error { return nil }

// HandleChat is the entry point called by the dashboard Server when a chat
// message arrives. It constructs the Inbound and dispatches it to deliver.
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
		SessionKey: SessionKey(sessionID),
		Text:       text,
		Reply:      out,
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

// SessionKey returns the canonical session key for a dashboard session UUID.
func SessionKey(sessionUUID string) core.SessionKey {
	return core.SessionKey("dashboard:" + sessionUUID)
}
