package core

type PermissionChecker interface {
	Check(toolName string, input ToolInput) (allow bool, reason string)
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

type WhatsAppMessenger interface {
	SendText(chatJID, text string) error
	SendTyping(chatJID string) error
}
