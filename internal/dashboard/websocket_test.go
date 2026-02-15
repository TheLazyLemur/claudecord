package dashboard

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHub_Broadcast_EvictsSlowClient(t *testing.T) {
	a := assert.New(t)

	hub := NewHub()
	go hub.Run()

	// register a client with a full send buffer (capacity 1)
	slow := &Client{
		hub:  hub,
		send: make(chan []byte, 1),
	}
	hub.register <- slow
	time.Sleep(10 * time.Millisecond)

	// fill the buffer so next broadcast can't deliver
	slow.send <- []byte("fill")

	// broadcast should evict the slow client, not deadlock
	done := make(chan struct{})
	go func() {
		hub.Broadcast(Message{Type: "test"})
		close(done)
	}()

	select {
	case <-done:
		// success — didn't deadlock
	case <-time.After(2 * time.Second):
		t.Fatal("broadcast deadlocked on slow client")
	}

	// slow client's send channel should be closed
	time.Sleep(10 * time.Millisecond)
	_, open := <-slow.send // drain the "fill" message
	a.True(open)           // first read gets the buffered message
	_, open = <-slow.send  // second read on closed channel
	a.False(open)
}

func TestHub_BroadcastSticky_CachesMessage(t *testing.T) {
	a := assert.New(t)

	hub := NewHub()
	go hub.Run()

	a.Nil(hub.Sticky())

	hub.BroadcastSticky(Message{Type: "whatsapp_qr", Content: "qr-code-data"})
	time.Sleep(10 * time.Millisecond)

	a.NotNil(hub.Sticky())
	a.Contains(string(hub.Sticky()), "qr-code-data")
}

func TestHub_ClearSticky(t *testing.T) {
	a := assert.New(t)

	hub := NewHub()
	go hub.Run()

	hub.BroadcastSticky(Message{Type: "whatsapp_qr", Content: "qr-code-data"})
	time.Sleep(10 * time.Millisecond)
	a.NotNil(hub.Sticky())

	hub.ClearSticky()
	a.Nil(hub.Sticky())
}

func TestHub_Broadcast_ConcurrentDoesNotDeadlock(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// register several clients with small buffers
	for i := 0; i < 5; i++ {
		c := &Client{
			hub:  hub,
			send: make(chan []byte, 1),
		}
		hub.register <- c
	}
	time.Sleep(10 * time.Millisecond)

	// blast concurrent broadcasts — would deadlock with the old RLock→Lock pattern
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hub.Broadcast(Message{Type: "test"})
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("concurrent broadcasts deadlocked")
	}
}
