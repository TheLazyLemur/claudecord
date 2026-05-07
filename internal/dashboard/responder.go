package dashboard

// WSResponder implements core.Responder for the dashboard.
type WSResponder struct {
	hub       *Hub
	sessionID string
	isCurrent func() bool
}

// NewWSResponder creates a responder that broadcasts to the hub. If isCurrent
// is non-nil it is consulted before each broadcast; when it returns false the
// broadcast is dropped so a turn that outlived its session does not bleed
// into the next one.
func NewWSResponder(hub *Hub, sessionID string, isCurrent func() bool) *WSResponder {
	return &WSResponder{
		hub:       hub,
		sessionID: sessionID,
		isCurrent: isCurrent,
	}
}

func (r *WSResponder) stale() bool {
	return r.isCurrent != nil && !r.isCurrent()
}

// SendTyping broadcasts typing indicator.
func (r *WSResponder) SendTyping() error {
	if r.stale() {
		return nil
	}
	active := true
	r.hub.Broadcast(Message{
		Type:   "typing",
		Active: &active,
	})
	return nil
}

// PostResponse broadcasts final response.
func (r *WSResponder) PostResponse(content string) error {
	if r.stale() {
		return nil
	}
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
	if r.stale() {
		return nil
	}
	r.hub.Broadcast(Message{
		Type:      "chat",
		Role:      "assistant",
		Content:   message,
		SessionID: r.sessionID,
	})
	return nil
}
