package core

const maxDiscordMessageLen = 2000

// DiscordResponder sends responses to Discord
type DiscordResponder struct {
	client    DiscordClient
	channelID string
	messageID string
	threadID  string
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
	if len(content) > maxDiscordMessageLen {
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
