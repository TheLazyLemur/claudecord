package whatsapp

import (
	"context"

	"github.com/TheLazyLemur/claudecord/internal/media"
	"github.com/pkg/errors"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
)

// Downloader is the subset of whatsmeow.Client we need for decrypting media.
type Downloader interface {
	Download(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error)
}

// ExtractText returns the plain text body of a WhatsApp message.
func ExtractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	text := msg.GetConversation()
	if text == "" {
		if ext := msg.GetExtendedTextMessage(); ext != nil {
			text = ext.GetText()
		}
	}
	return text
}

// ExtractInbound pulls caption + attachment out of a WhatsApp message event.
// Returns (caption, nil, nil) for plain text. Returns (caption, att, nil) for
// supported media (image, document). Other media types yield (caption, nil, nil) —
// out of scope for this implementation.
func ExtractInbound(ctx context.Context, msg *events.Message, dl Downloader) (string, *media.Attachment, error) {
	if msg == nil || msg.Message == nil {
		return "", nil, nil
	}

	if img := msg.Message.GetImageMessage(); img != nil {
		bytes, err := dl.Download(ctx, img)
		if err != nil {
			return "", nil, errors.Wrap(err, "downloading image")
		}
		return img.GetCaption(), &media.Attachment{
			MIME:  img.GetMimetype(),
			Bytes: bytes,
		}, nil
	}

	if doc := msg.Message.GetDocumentMessage(); doc != nil {
		bytes, err := dl.Download(ctx, doc)
		if err != nil {
			return "", nil, errors.Wrap(err, "downloading document")
		}
		return doc.GetCaption(), &media.Attachment{
			MIME:         doc.GetMimetype(),
			OriginalName: doc.GetFileName(),
			Bytes:        bytes,
		}, nil
	}

	return ExtractText(msg.Message), nil, nil
}
