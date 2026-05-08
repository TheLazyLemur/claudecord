package core

import "context"

type SessionKey string

type Inbound struct {
	SessionKey  SessionKey
	Text        string
	Attachments []AttachmentRef
	Reply       Outbound
}

// Outbound is the send-side of a channel for a single inbound message.
// It's an alias of Responder so existing Backend.Converse signatures continue
// to work; the new name documents intent at call sites that build channel
// plugins.
type Outbound = Responder

type Capabilities struct {
	Reactions bool
	Typing    bool
}

type ChannelPlugin interface {
	ID() string
	Capabilities() Capabilities
	Start(ctx context.Context, deliver func(Inbound)) error
	Stop() error
}
