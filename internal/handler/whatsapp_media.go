package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/pkg/errors"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"golang.org/x/text/unicode/norm"
)

const (
	MaxImageBytes = 10 * 1024 * 1024
	MaxDocBytes   = 50 * 1024 * 1024
)

// Downloader is the subset of whatsmeow.Client we need for decrypting media.
type Downloader interface {
	Download(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error)
}

type Attachment struct {
	MIME         string
	OriginalName string
	Bytes        []byte
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
func ExtractInbound(ctx context.Context, msg *events.Message, dl Downloader) (string, *Attachment, error) {
	if msg == nil || msg.Message == nil {
		return "", nil, nil
	}

	if img := msg.Message.GetImageMessage(); img != nil {
		bytes, err := dl.Download(ctx, img)
		if err != nil {
			return "", nil, errors.Wrap(err, "downloading image")
		}
		return img.GetCaption(), &Attachment{
			MIME:  img.GetMimetype(),
			Bytes: bytes,
		}, nil
	}

	if doc := msg.Message.GetDocumentMessage(); doc != nil {
		bytes, err := dl.Download(ctx, doc)
		if err != nil {
			return "", nil, errors.Wrap(err, "downloading document")
		}
		return doc.GetCaption(), &Attachment{
			MIME:         doc.GetMimetype(),
			OriginalName: doc.GetFileName(),
			Bytes:        bytes,
		}, nil
	}

	return ExtractText(msg.Message), nil, nil
}

// isImageMIME reports whether att should be size-capped at MaxImageBytes.
func isImageMIME(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

// SizeCap returns the byte limit for an attachment given its MIME family.
func SizeCap(mime string) int {
	if isImageMIME(mime) {
		return MaxImageBytes
	}
	return MaxDocBytes
}

// SaveAttachment decrypts → writes the attachment under mediaDir with a
// collision-resistant name. now is injected so tests can freeze time.
func SaveAttachment(mediaDir string, att *Attachment, now time.Time) (string, error) {
	if att == nil {
		return "", errors.New("nil attachment")
	}

	rand4, err := randHex(2)
	if err != nil {
		return "", errors.Wrap(err, "generating random suffix")
	}

	stem, ext := chooseStemAndExt(att)
	if stem == "" {
		fallback, err := randHex(4)
		if err != nil {
			return "", errors.Wrap(err, "generating fallback stem")
		}
		stem = fallback
	}

	name := now.UTC().Format(time.RFC3339) + "_" + stem + "_" + rand4 + ext
	path := filepath.Join(mediaDir, name)

	if err := os.WriteFile(path, att.Bytes, 0o600); err != nil {
		return "", errors.Wrap(err, "writing attachment")
	}
	return path, nil
}

// chooseStemAndExt produces (sanitized-original-or-empty, .ext).
// Stem is empty when sanitization left nothing usable; caller falls back to a
// random hex stem in that case.
func chooseStemAndExt(att *Attachment) (string, string) {
	stem := ""
	ext := ""

	if att.OriginalName != "" {
		stem = sanitizeName(stripExt(att.OriginalName))
		if e := filepath.Ext(att.OriginalName); validExt(e) {
			ext = e
		}
	}

	if ext == "" {
		ext = extFromMIME(att.MIME)
	}
	if ext == "" {
		ext = ".bin"
	}
	return stem, ext
}

func stripExt(name string) string {
	e := filepath.Ext(name)
	if e == "" {
		return name
	}
	return strings.TrimSuffix(name, e)
}

func validExt(e string) bool {
	if e == "" || e == "." {
		return false
	}
	for _, r := range e[1:] {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return len(e) <= 10
}

func extFromMIME(m string) string {
	if m == "" {
		return ""
	}
	exts, _ := mime.ExtensionsByType(m)
	if len(exts) > 0 {
		return exts[0]
	}
	return ""
}

// sanitizeName: strip separators + leading dots + control chars,
// NFC-normalize, collapse whitespace, cap to 100 runes.
func sanitizeName(s string) string {
	s = norm.NFC.String(s)

	var b strings.Builder
	prevSpace := false
	for _, r := range s {
		switch {
		case r == '/' || r == '\\' || r == os.PathSeparator:
			continue
		case unicode.IsSpace(r):
			if !prevSpace && b.Len() > 0 {
				b.WriteByte(' ')
				prevSpace = true
			}
		case unicode.IsControl(r):
			continue
		default:
			b.WriteRune(r)
			prevSpace = false
		}
	}

	out := strings.TrimLeft(b.String(), ".")
	out = strings.TrimSpace(out)

	if rs := []rune(out); len(rs) > 100 {
		out = string(rs[:100])
	}
	return out
}

func randHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
