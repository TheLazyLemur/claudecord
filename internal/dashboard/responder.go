package dashboard

import (
	"sync"

	"github.com/google/uuid"
)

// WSResponder implements core.Responder for the dashboard.
type WSResponder struct {
	hub       *Hub
	sessionID string

	// Permission handling
	permMu       sync.Mutex
	permChannels map[string]chan bool
}

// NewWSResponder creates a responder that broadcasts to the hub.
func NewWSResponder(hub *Hub, sessionID string) *WSResponder {
	return &WSResponder{
		hub:          hub,
		sessionID:    sessionID,
		permChannels: make(map[string]chan bool),
	}
}

// SendTyping broadcasts typing indicator.
func (r *WSResponder) SendTyping() error {
	active := true
	r.hub.Broadcast(Message{
		Type:   "typing",
		Active: &active,
	})
	return nil
}

// PostResponse broadcasts final response.
func (r *WSResponder) PostResponse(content string) error {
	// Stop typing
	active := false
	r.hub.Broadcast(Message{
		Type:   "typing",
		Active: &active,
	})

	r.hub.Broadcast(Message{
		Type:      "chat",
		Role:      "assistant",
		Content:   content,
		SessionID: r.sessionID,
	})
	return nil
}

// AddReaction is a no-op for dashboard.
func (r *WSResponder) AddReaction(emoji string) error {
	return nil
}

// SendUpdate broadcasts an incremental update.
func (r *WSResponder) SendUpdate(message string) error {
	r.hub.Broadcast(Message{
		Type:      "chat",
		Role:      "assistant",
		Content:   message,
		SessionID: r.sessionID,
	})
	return nil
}

// AskPermission broadcasts permission request and blocks until response.
func (r *WSResponder) AskPermission(prompt string) (bool, error) {
	id := uuid.NewString()

	// Create response channel
	ch := make(chan bool, 1)
	r.permMu.Lock()
	r.permChannels[id] = ch
	r.permMu.Unlock()

	defer func() {
		r.permMu.Lock()
		delete(r.permChannels, id)
		r.permMu.Unlock()
	}()

	// Broadcast permission request
	r.hub.Broadcast(Message{
		Type:   "permission",
		ID:     id,
		Prompt: prompt,
	})

	// Block until response
	approved := <-ch
	return approved, nil
}

// HandlePermissionResponse routes permission response to waiting goroutine.
func (r *WSResponder) HandlePermissionResponse(id string, approved bool) {
	r.permMu.Lock()
	ch, ok := r.permChannels[id]
	r.permMu.Unlock()

	if ok {
		select {
		case ch <- approved:
		default:
		}
	}
}
