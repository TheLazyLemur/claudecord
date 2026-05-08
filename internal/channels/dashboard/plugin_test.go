package dashboard

import (
	"context"
	"testing"

	dash "github.com/TheLazyLemur/claudecord/internal/dashboard"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

// stubChatCallbackSetter records the callback passed to it.
type stubChatCallbackSetter struct {
	cb func(sessionID, text string)
}

func (s *stubChatCallbackSetter) SetChatCallback(cb func(sessionID, text string)) {
	s.cb = cb
}

func TestPlugin_Start_RegistersChatCallbackOnServer(t *testing.T) {
	// given
	// ... a plugin configured with a stub server
	setter := &stubChatCallbackSetter{}
	p := New(Config{Server: setter})

	// when
	// ... Start is called
	_ = p.Start(context.Background(), func(_ core.Inbound) {})

	// then
	// ... the server's chat callback is now set to the plugin's HandleChat
	if setter.cb == nil {
		t.Fatalf("expected chat callback to be registered on server after Start")
	}
}

func TestPlugin_Start_RegisteredCallback_InvokesHandleChat(t *testing.T) {
	// given
	// ... a plugin with a registered callback and a captured inbound
	setter := &stubChatCallbackSetter{}
	p := New(Config{Server: setter})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... the registered callback is invoked
	setter.cb("sess-42", "hello via callback")

	// then
	// ... an inbound is dispatched with the correct session key and text
	if got.SessionKey != "dashboard:sess-42" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
	if got.Text != "hello via callback" {
		t.Fatalf("text: %q", got.Text)
	}
}

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

func TestPlugin_Inbound_CapabilitiesMatchPluginCapabilities(t *testing.T) {
	// given
	// ... a plugin and a captured inbound
	p := New(Config{})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... HandleChat dispatches an inbound
	p.HandleChat("sess-1", "hello")

	// then
	// ... the inbound's Capabilities exactly match p.Capabilities()
	if got.Capabilities != p.Capabilities() {
		t.Fatalf("capabilities mismatch: inbound=%+v plugin=%+v", got.Capabilities, p.Capabilities())
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
	// ... reactions no, updates yes
	if caps.Reactions {
		t.Fatalf("unexpected caps: %+v", caps)
	}
	if !caps.Updates {
		t.Fatalf("expected Updates=true, got %+v", caps)
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
