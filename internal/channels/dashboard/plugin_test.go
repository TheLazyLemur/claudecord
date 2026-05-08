package dashboard

import (
	"context"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

func TestPlugin_DeliversWithSessionUUIDKey(t *testing.T) {
	// given
	// ... a plugin and a captured inbound
	p := New(Config{})
	var got core.Inbound

	// when
	// ... the plugin delivers a synthetic inbound with a session id
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })
	p.deliverForTest(core.Inbound{SessionKey: SessionKey("abc-123"), Text: "hi"})

	// then
	// ... the SessionKey is dashboard-prefixed
	if got.SessionKey != "dashboard:abc-123" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
}

func TestPlugin_Capabilities(t *testing.T) {
	// given
	// ... a default plugin
	p := New(Config{})

	// when
	// ... capabilities are queried
	caps := p.Capabilities()

	// then
	// ... typing yes, reactions no
	if caps.Reactions || !caps.Typing {
		t.Fatalf("unexpected caps: %+v", caps)
	}
}

func TestPlugin_ID(t *testing.T) {
	// given
	// ... a default plugin
	p := New(Config{})

	// when
	// ... ID is queried
	id := p.ID()

	// then
	// ... it is "dashboard"
	if id != "dashboard" {
		t.Fatalf("id: %q", id)
	}
}
