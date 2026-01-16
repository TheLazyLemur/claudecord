package core

type CLIProcess interface {
	Send(msg []byte) error
	Receive() (<-chan []byte, error)
	Close() error
	SessionID() string
}

type PermissionChecker interface {
	Check(toolName string, input map[string]any) (allow bool, reason string)
}

type DiscordClient interface {
	SendMessage(channelID, content string) error
	SendThread(channelID, content string) (threadID string, err error)
	SendTyping(channelID string) error
}
