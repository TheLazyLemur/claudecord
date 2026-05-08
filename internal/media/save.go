package media

import (
	"crypto/rand"
	"encoding/hex"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/pkg/errors"
	"golang.org/x/text/unicode/norm"
)

// Attachment holds a decrypted or downloaded media payload.
type Attachment struct {
	MIME         string
	OriginalName string
	Bytes        []byte
}

// SaveAttachment writes the attachment under mediaDir with a collision-resistant
// name derived from now, the original filename, and a random suffix.
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

// sanitizeName strips separators, control chars, leading dots; NFC-normalizes;
// collapses whitespace; caps to 100 runes.
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
