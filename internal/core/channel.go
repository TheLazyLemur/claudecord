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
}

type Capabilities struct {
	Reactions bool
	// Typing indicates the plugin can send typing indicators. Currently
	// unused in dispatch — every active plugin supports typing, so the flag
	// has no signal. Retained for future plugins that may lack typing support.
	Typing bool
}

type ChannelPlugin interface {
	ID() string
	Capabilities() Capabilities
	Start(ctx context.Context, deliver func(Inbound)) error
	Stop() error
}
