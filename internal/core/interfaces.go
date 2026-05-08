package core

type PermissionChecker interface {
	Check(toolName string, input ToolInput) (allow bool, reason string)
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
