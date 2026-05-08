package core

import "context"

type SessionKey string

type Inbound struct {
	SessionKey SessionKey
	Text       string
	// Attachments carries media refs for the current message.
	// Populated by channels that translate inbound platform attachments (currently WhatsApp and Discord).
	Attachments []AttachmentRef
	Reply       Outbound
	// Capabilities describes what the originating channel plugin supports.
	// Used to gate per-session tool registration (e.g. react_emoji).
	Capabilities Capabilities
}

type Capabilities struct {
	Reactions bool
	Media     bool
}

type ChannelPlugin interface {
	ID() string
	Capabilities() Capabilities
	Start(ctx context.Context, deliver func(Inbound)) error
	Stop() error
}
