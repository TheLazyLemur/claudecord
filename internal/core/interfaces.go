package core

type PermissionChecker interface {
	Check(toolName string, input ToolInput) (allow bool, reason string)
}

type DiscordClient interface {
	SendMessage(channelID, content string) error
	SendMessageReturningID(channelID, content string) (messageID string, err error)
	CreateThread(channelID, content string) (threadID string, err error)
	StartThread(channelID, messageID, name string) (threadID string, err error)
	SendTyping(channelID string) error
	AddReaction(channelID, messageID, emoji string) error
	WaitForReaction(channelID, messageID string, emojis []string, userID string) (emoji string, err error)
}

type Responder interface {
	SendTyping() error
	PostResponse(content string) error
	AddReaction(emoji string) error
	SendUpdate(message string) error
	AskPermission(prompt string) (approved bool, err error)
}

type EmailClient interface {
	Send(to, subject, body string) error
}
