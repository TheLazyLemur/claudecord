package core

type CLIProcess interface {
	Send(msg []byte) error
	// Receive returns a channel that streams messages until Close() is called or an error occurs.
	Receive() (<-chan []byte, error)
	Close() error
	SessionID() string
}

type PermissionChecker interface {
	Check(toolName string, input map[string]any) (allow bool, reason string)
}

type DiscordClient interface {
	SendMessage(channelID, content string) error
	CreateThread(channelID, content string) (threadID string, err error)
	StartThread(channelID, messageID, name string) (threadID string, err error)
	SendTyping(channelID string) error
	AddReaction(channelID, messageID, emoji string) error
}

type Responder interface {
	SendTyping() error
	PostResponse(content string) error
	AddReaction(emoji string) error
	SendUpdate(message string) error
}

type EmailClient interface {
	Send(to, subject, body string) error
}
