package whatsapp

import "github.com/TheLazyLemur/switchboard/internal/core"

const maxMessageLen = 65536

// Outbound sends responses to a WhatsApp chat.
type Outbound struct {
	client  core.WhatsAppMessenger
	chatJID string
}

func NewOutbound(client core.WhatsAppMessenger, chatJID string) *Outbound {
	return &Outbound{client: client, chatJID: chatJID}
}

func (r *Outbound) SendTyping() error {
	return r.client.SendTyping(r.chatJID)
}

func (r *Outbound) PostResponse(content string) error {
	chunks := core.ChunkMessage(content, maxMessageLen)
	for _, chunk := range chunks {
		if err := r.client.SendText(r.chatJID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (r *Outbound) AddReaction(_ string) error { return nil }

func (r *Outbound) SendUpdate(message string) error {
	return r.client.SendText(r.chatJID, message)
}
