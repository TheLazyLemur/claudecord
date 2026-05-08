package dashboard

import (
	"context"
	"testing"

	dash "github.com/TheLazyLemur/claudecord/internal/dashboard"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

func TestPlugin_DeliversWithSessionUUIDKey(t *testing.T) {
	// given
	// ... a plugin with a real hub and a captured inbound
	hub := dash.NewHub()
	go hub.Run()
	p := New(Config{Hub: hub})
	var got core.Inbound

	// when
	// ... HandleChat is called with a session id and message text
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })
	p.HandleChat("abc-123", "hi")

	// then
	// ... the SessionKey is dashboard-prefixed and text is preserved
	if got.SessionKey != "dashboard:abc-123" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
	if got.Text != "hi" {
		t.Fatalf("text: %q", got.Text)
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
	// ... reactions no
	if caps.Reactions {
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
