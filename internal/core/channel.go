package core

import "context"

type SessionKey string

type Inbound struct {
	SessionKey SessionKey
	Text       string
	// Attachments carries decrypted media refs for the current message.
	// WhatsApp populates this; Discord currently does not.
	Attachments []AttachmentRef
	Reply       Outbound
	// Capabilities describes what the originating channel plugin supports.
	// Used to gate per-session tool registration (e.g. react_emoji).
	Capabilities Capabilities
}

type Capabilities struct {
	Reactions bool
}

type ChannelPlugin interface {
	ID() string
	Capabilities() Capabilities
	Start(ctx context.Context, deliver func(Inbound)) error
	Stop() error
}
