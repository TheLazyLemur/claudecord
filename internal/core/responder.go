package core

import "strings"

const MaxDiscordMessageLen = 2000
const MaxWhatsAppMessageLen = 65536

// ChunkMessage splits content into chunks of at most maxLen bytes.
func ChunkMessage(content string, maxLen int) []string {
	if content == "" {
		return nil
	}
	var chunks []string
	for len(content) > 0 {
		chunk := content
		if len(chunk) > maxLen {
			chunk = content[:maxLen]
			content = content[maxLen:]
		} else {
			content = ""
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}

// DiscordResponder sends responses to Discord
type DiscordResponder struct {
	client    DiscordClient
	channelID string
	messageID string
	threadID  string
	userID    string
}

func NewDiscordResponder(client DiscordClient, channelID, messageID string) *DiscordResponder {
	return &DiscordResponder{
		client:    client,
		channelID: channelID,
		messageID: messageID,
	}
}

func (r *DiscordResponder) SendTyping() error {
	return r.client.SendTyping(r.channelID)
}

func (r *DiscordResponder) PostResponse(content string) error {
	if len(content) > MaxDiscordMessageLen {
		_, err := r.client.CreateThread(r.channelID, content)
		return err
	}
	return r.client.SendMessage(r.channelID, content)
}

func (r *DiscordResponder) AddReaction(emoji string) error {
	return r.client.AddReaction(r.channelID, r.messageID, emoji)
}

func (r *DiscordResponder) SendUpdate(message string) error {
	if r.threadID == "" {
		tid, err := r.client.StartThread(r.channelID, r.messageID, "Updates")
		if err != nil {
			return err
		}
		r.threadID = tid
	}
	return r.client.SendMessage(r.threadID, message)
}

func (r *DiscordResponder) SetUserID(userID string) {
	r.userID = userID
}

func (r *DiscordResponder) AskPermission(prompt string) (bool, error) {
	msg := prompt + " React ✅ or ❌"
	msgID, err := r.client.SendMessageReturningID(r.channelID, msg)
	if err != nil {
		return false, err
	}

	// add reaction options
	r.client.AddReaction(r.channelID, msgID, "✅")
	r.client.AddReaction(r.channelID, msgID, "❌")

	emoji, err := r.client.WaitForReaction(r.channelID, msgID, []string{"✅", "❌"}, r.userID)
	if err != nil {
		return false, err
	}

	return emoji == "✅", nil
}

// EmailResponder sends responses via email
type EmailResponder struct {
	client  EmailClient
	to      string
	subject string
}

func NewEmailResponder(client EmailClient, to, subject string) *EmailResponder {
	return &EmailResponder{
		client:  client,
		to:      to,
		subject: subject,
	}
}

func (r *EmailResponder) SendTyping() error {
	return nil // no-op for email
}

func (r *EmailResponder) PostResponse(content string) error {
	return r.client.Send(r.to, r.subject, content)
}

func (r *EmailResponder) AddReaction(emoji string) error {
	return nil // no-op for email
}

func (r *EmailResponder) SendUpdate(message string) error {
	return nil // no-op for email
}

func (r *EmailResponder) AskPermission(prompt string) (bool, error) {
	return false, nil // auto-deny for email (no interactive channel)
}

// WhatsAppResponder sends responses via WhatsApp
type WhatsAppResponder struct {
	client    WhatsAppMessenger
	chatJID   string
	senderJID string
}

func NewWhatsAppResponder(client WhatsAppMessenger, chatJID, senderJID string) *WhatsAppResponder {
	return &WhatsAppResponder{
		client:    client,
		chatJID:   chatJID,
		senderJID: senderJID,
	}
}

func (r *WhatsAppResponder) SendTyping() error {
	return r.client.SendTyping(r.chatJID)
}

func (r *WhatsAppResponder) PostResponse(content string) error {
	chunks := ChunkMessage(content, MaxWhatsAppMessageLen)
	for _, chunk := range chunks {
		if err := r.client.SendText(r.chatJID, chunk); err != nil {
			return err
		}
	}
	return nil
}

func (r *WhatsAppResponder) AddReaction(emoji string) error {
	return nil
}

func (r *WhatsAppResponder) SendUpdate(message string) error {
	return r.client.SendText(r.chatJID, message)
}

func (r *WhatsAppResponder) AskPermission(prompt string) (bool, error) {
	if err := r.client.SendText(r.chatJID, prompt+"\nReply yes/no"); err != nil {
		return false, err
	}
	reply, err := r.client.WaitForReply(r.senderJID)
	if err != nil {
		return false, err
	}
	lower := strings.ToLower(strings.TrimSpace(reply))
	return lower == "y" || lower == "yes", nil
}
