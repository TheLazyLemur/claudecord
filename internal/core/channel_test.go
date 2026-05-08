package core

import (
	"context"
	"testing"
)

func TestChannelPlugin_BasicShape(t *testing.T) {
	// given
	// ... a fake plugin that records its lifecycle
	var started, stopped bool
	p := fakePlugin{
		id:   "fake",
		caps: Capabilities{Reactions: true, Typing: true},
		start: func(ctx context.Context, deliver func(Inbound)) error {
			started = true
			deliver(Inbound{SessionKey: "fake:1", Text: "hi"})
			return nil
		},
		stop: func() error { stopped = true; return nil },
	}

	// when
	// ... the plugin is started and then stopped
	var received Inbound
	_ = p.Start(context.Background(), func(in Inbound) { received = in })
	_ = p.Stop()

	// then
	// ... start ran, stop ran, the inbound was delivered with the expected key
	if !started || !stopped {
		t.Fatalf("lifecycle not invoked: started=%v stopped=%v", started, stopped)
	}
	if received.SessionKey != "fake:1" || received.Text != "hi" {
		t.Fatalf("unexpected inbound: %+v", received)
	}
	if p.ID() != "fake" || !p.Capabilities().Reactions {
		t.Fatalf("metadata mismatch")
	}
}

type fakePlugin struct {
	id    string
	caps  Capabilities
	start func(context.Context, func(Inbound)) error
	stop  func() error
}

func (f fakePlugin) ID() string                                        { return f.id }
func (f fakePlugin) Capabilities() Capabilities                        { return f.caps }
func (f fakePlugin) Start(ctx context.Context, d func(Inbound)) error { return f.start(ctx, d) }
func (f fakePlugin) Stop() error                                       { return f.stop() }
