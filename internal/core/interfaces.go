package core

type PermissionChecker interface {
	Check(toolName string, input ToolInput) (allow bool, reason string)
}

// Outbound is the per-message send-side of a channel. It is owned by a single
// Inbound and bound to the originating chat surface (Discord thread, WhatsApp
// chat, dashboard WebSocket). Every channel plugin's reply type implements
// this interface.
type Outbound interface {
	SendTyping() error
	PostResponse(content string) error
	AddReaction(emoji string) error
	SendUpdate(message string) error
}

type WhatsAppMessenger interface {
	SendText(chatJID, text string) error
	SendTyping(chatJID string) error
}
