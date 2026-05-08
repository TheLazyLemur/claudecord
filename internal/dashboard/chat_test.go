package dashboard

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBackend is a minimal core.Backend for testing.
type fakeBackend struct {
	sessionID string
}

func (f *fakeBackend) Converse(_ context.Context, _ string, _ core.Outbound, _ core.PermissionChecker) (string, error) {
	return "", nil
}
func (f *fakeBackend) SessionID() string { return f.sessionID }
func (f *fakeBackend) Close() error      { return nil }

// fakeBackendFactory returns the same fakeBackend every time.
type fakeBackendFactory struct {
	backend *fakeBackend
}

func (f *fakeBackendFactory) Create(_ string, _ core.Capabilities) (core.Backend, error) {
	return f.backend, nil
}

// collectSessionBroadcasts drains client.send for up to 50ms and returns all
// decoded Messages with Type == "session".
func collectSessionBroadcasts(client *Client) []Message {
	var out []Message
	deadline := time.After(50 * time.Millisecond)
	for {
		select {
		case data, ok := <-client.send:
			if !ok {
				return out
			}
			var m Message
			if err := json.Unmarshal(data, &m); err == nil && m.Type == "session" {
				out = append(out, m)
			}
		case <-deadline:
			return out
		}
	}
}

func TestHandleChat_SessionBroadcast_OnlyOnSessionIDChange(t *testing.T) {
	// given
	// ... a hub with a registered client and a session manager with a fixed session ID
	hub := NewHub()
	go hub.Run()

	client := &Client{hub: hub, send: make(chan []byte, 64)}
	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	backend := &fakeBackend{sessionID: "session-abc"}
	mgr := core.NewSessionManager(&fakeBackendFactory{backend: backend}, nil)

	s := &Server{
		hub:          hub,
		sessionMgr:   mgr,
		chatCallback: func(_, _ string) {},
	}

	// when
	// ... two chat messages arrive with the same underlying session ID
	s.handleChat("first message")
	s.handleChat("second message")
	time.Sleep(20 * time.Millisecond)

	// then
	// ... exactly one session broadcast is emitted
	msgs := collectSessionBroadcasts(client)
	assert.Len(t, msgs, 1, "expected one session broadcast for same session ID")
	require.NotEmpty(t, msgs)
	assert.Equal(t, "session-abc", msgs[0].SessionID)
}

func TestHandleChat_SessionBroadcast_EmittedOnSessionIDChange(t *testing.T) {
	// given
	// ... a hub with a registered client and a session manager whose backend session ID changes
	hub := NewHub()
	go hub.Run()

	client := &Client{hub: hub, send: make(chan []byte, 64)}
	hub.register <- client
	time.Sleep(10 * time.Millisecond)

	backend := &fakeBackend{sessionID: "session-first"}
	mgr := core.NewSessionManager(&fakeBackendFactory{backend: backend}, nil)

	s := &Server{
		hub:          hub,
		sessionMgr:   mgr,
		chatCallback: func(_, _ string) {},
	}

	// when
	// ... a message is sent, then the session ID changes, then another message is sent
	s.handleChat("first message")
	time.Sleep(10 * time.Millisecond)
	backend.sessionID = "session-second"
	s.handleChat("second message")
	time.Sleep(20 * time.Millisecond)

	// then
	// ... two session broadcasts are emitted, one per distinct session ID
	msgs := collectSessionBroadcasts(client)
	require.Len(t, msgs, 2, "expected two session broadcasts, one per session ID")
	assert.Equal(t, "session-first", msgs[0].SessionID)
	assert.Equal(t, "session-second", msgs[1].SessionID)
}
