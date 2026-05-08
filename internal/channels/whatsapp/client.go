package whatsapp

import (
	"context"

	"github.com/pkg/errors"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// ClientWrapper wraps whatsmeow.Client, implementing both Downloader
// and core.WhatsAppMessenger.
type ClientWrapper struct {
	client *whatsmeow.Client
}

// NewClientWrapper constructs a ClientWrapper from a whatsmeow.Client.
func NewClientWrapper(client *whatsmeow.Client) *ClientWrapper {
	return &ClientWrapper{client: client}
}

func (c *ClientWrapper) Download(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error) {
	return c.client.Download(ctx, msg)
}

func (c *ClientWrapper) SendText(chatJID, text string) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return errors.Wrap(err, "parsing chat JID")
	}
	msg := &waE2E.Message{Conversation: proto.String(text)}
	_, err = c.client.SendMessage(context.Background(), jid, msg)
	return errors.Wrap(err, "sending whatsapp message")
}

func (c *ClientWrapper) SendTyping(chatJID string) error {
	jid, err := types.ParseJID(chatJID)
	if err != nil {
		return errors.Wrap(err, "parsing chat JID")
	}
	ctx := context.Background()
	if err := c.client.SendPresence(ctx, types.PresenceAvailable); err != nil {
		return errors.Wrap(err, "sending presence")
	}
	return errors.Wrap(
		c.client.SendChatPresence(ctx, jid, types.ChatPresenceComposing, types.ChatPresenceMediaText),
		"sending chat presence",
	)
}
