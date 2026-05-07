package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func registerTestClient(hub *Hub) *Client {
	c := &Client{
		hub:  hub,
		send: make(chan []byte, 16),
	}
	hub.register <- c
	time.Sleep(10 * time.Millisecond)
	return c
}

func drainOne(c *Client) (Message, bool) {
	select {
	case raw := <-c.send:
		var m Message
		_ = json.Unmarshal(raw, &m)
		return m, true
	case <-time.After(100 * time.Millisecond):
		return Message{}, false
	}
}

func TestWSResponder_PostResponse_BroadcastsWhenCurrent(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a responder whose session is still current
	hub := NewHub()
	go hub.Run()
	c := registerTestClient(hub)
	r := NewWSResponder(hub, "abc", func() bool { return true })

	// when
	// ... PostResponse is called
	err := r.PostResponse("hello")

	// then
	// ... typing(off) and chat messages are broadcast
	a.NoError(err)
	m1, ok1 := drainOne(c)
	a.True(ok1)
	a.Equal("typing", m1.Type)
	m2, ok2 := drainOne(c)
	a.True(ok2)
	a.Equal("chat", m2.Type)
	a.Equal("hello", m2.Content)
	a.Equal("abc", m2.SessionID)
}

func TestWSResponder_PostResponse_DropsWhenStale(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a responder whose session is no longer current
	hub := NewHub()
	go hub.Run()
	c := registerTestClient(hub)
	r := NewWSResponder(hub, "abc", func() bool { return false })

	// when
	// ... PostResponse is called
	err := r.PostResponse("stale")

	// then
	// ... nothing is broadcast
	a.NoError(err)
	_, ok := drainOne(c)
	a.False(ok, "should not broadcast for stale session")
}

func TestWSResponder_SendTyping_DropsWhenStale(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a stale responder
	hub := NewHub()
	go hub.Run()
	c := registerTestClient(hub)
	r := NewWSResponder(hub, "abc", func() bool { return false })

	// when
	// ... SendTyping is called
	err := r.SendTyping()

	// then
	// ... nothing is broadcast
	a.NoError(err)
	_, ok := drainOne(c)
	a.False(ok)
}

func TestWSResponder_SendUpdate_DropsWhenStale(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a stale responder
	hub := NewHub()
	go hub.Run()
	c := registerTestClient(hub)
	r := NewWSResponder(hub, "abc", func() bool { return false })

	// when
	// ... SendUpdate is called
	err := r.SendUpdate("partial")

	// then
	// ... nothing is broadcast
	a.NoError(err)
	_, ok := drainOne(c)
	a.False(ok)
}

func TestWSResponder_NilIsCurrent_AlwaysBroadcasts(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a responder constructed without an isCurrent check
	hub := NewHub()
	go hub.Run()
	c := registerTestClient(hub)
	r := NewWSResponder(hub, "abc", nil)

	// when
	// ... SendUpdate is called
	err := r.SendUpdate("partial")

	// then
	// ... the message is broadcast
	a.NoError(err)
	m, ok := drainOne(c)
	a.True(ok)
	a.Equal("chat", m.Type)
}
