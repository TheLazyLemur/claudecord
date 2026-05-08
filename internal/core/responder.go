package core

const MaxDiscordMessageLen = 2000

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

