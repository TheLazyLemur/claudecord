package dashboard

// WSResponder implements core.Responder for the dashboard.
type WSResponder struct {
	hub       *Hub
	sessionID string
}

// NewWSResponder creates a responder that broadcasts to the hub.
func NewWSResponder(hub *Hub, sessionID string) *WSResponder {
	return &WSResponder{
		hub:       hub,
		sessionID: sessionID,
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
