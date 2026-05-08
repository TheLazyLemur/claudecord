package whatsapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAttachment_FilenameWithOriginalName(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... an attachment with an original filename
	dir := t.TempDir()
	now := time.Date(2026, 5, 1, 12, 30, 45, 0, time.UTC)
	att := &Attachment{
		MIME:         "image/png",
		OriginalName: "photo.png",
		Bytes:        []byte("PNGDATA"),
	}

	// when
	// ... the attachment is saved
	path, err := SaveAttachment(dir, att, now)
	r.NoError(err)

	// then
	// ... the filename contains the timestamp + original stem + extension
	base := filepath.Base(path)
	a.True(strings.HasPrefix(base, "2026-05-01T12:30:45Z_photo_"), "got %s", base)
	a.True(strings.HasSuffix(base, ".png"), "got %s", base)

	data, err := os.ReadFile(path)
	r.NoError(err)
	a.Equal([]byte("PNGDATA"), data)
}

func TestSaveAttachment_RandomStemWhenSanitizedEmpty(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... an attachment whose original name sanitizes to empty
	dir := t.TempDir()
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	att := &Attachment{
		MIME:         "image/jpeg",
		OriginalName: "../../",
		Bytes:        []byte("data"),
	}

	// when
	// ... the attachment is saved
	path, err := SaveAttachment(dir, att, now)
	r.NoError(err)

	// then
	// ... the stem is random hex with no path traversal
	base := filepath.Base(path)
	a.NotContains(base, "..")
	a.NotContains(base, "/")
	// stem fell back to random hex; ext came from MIME (jpg/jpeg/jfif on different systems).
	a.Regexp(`^2026-01-02T03:04:05Z_[0-9a-f]{8}_[0-9a-f]{4}\.\w+$`, base)
}

func TestSaveAttachment_ExtFromMIME(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... an attachment with no file extension in the original name
	dir := t.TempDir()
	att := &Attachment{
		MIME:         "image/png",
		OriginalName: "noext",
		Bytes:        []byte("d"),
	}

	// when
	// ... the attachment is saved
	path, err := SaveAttachment(dir, att, time.Now())
	r.NoError(err)

	// then
	// ... the extension is derived from the MIME type
	a.True(strings.HasSuffix(path, ".png"), "got %s", path)
}

func TestSaveAttachment_BinFallbackWhenMIMEUnknown(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... an attachment with an unrecognised MIME type
	dir := t.TempDir()
	att := &Attachment{
		MIME:         "application/x-totally-unknown-mime",
		OriginalName: "thing",
		Bytes:        []byte("d"),
	}

	// when
	// ... the attachment is saved
	path, err := SaveAttachment(dir, att, time.Now())
	r.NoError(err)

	// then
	// ... the extension falls back to .bin
	a.True(strings.HasSuffix(path, ".bin"), "got %s", path)
}

func TestSaveAttachment_FilePermissions(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a minimal attachment
	dir := t.TempDir()
	att := &Attachment{
		MIME:  "image/png",
		Bytes: []byte("d"),
	}

	// when
	// ... the attachment is saved
	path, err := SaveAttachment(dir, att, time.Now())
	r.NoError(err)

	// then
	// ... the file has mode 0600
	info, err := os.Stat(path)
	r.NoError(err)
	a.Equal(os.FileMode(0o600), info.Mode().Perm())
}

func TestSanitizeName_StripsControlAndSeparators(t *testing.T) {
	a := assert.New(t)

	// given
	// ... names with control characters and path separators

	// when / then
	// ... they are stripped
	a.Equal("foo", sanitizeName("..foo"))
	a.Equal("foobar", sanitizeName("foo/bar"))
	a.Equal("foobar", sanitizeName("foo\\bar"))
	a.Equal("foobar", sanitizeName("foo\x00\x07bar"))
}

func TestSanitizeName_CollapsesWhitespace(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a name with multiple spaces and tabs

	// when
	// ... the name is sanitized
	result := sanitizeName("  foo   bar\tbaz  ")

	// then
	// ... whitespace is collapsed and trimmed
	a.Equal("foo bar baz", result)
}

func TestSanitizeName_CapsTo100Runes(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a 200-rune string

	// when
	// ... the name is sanitized
	long := strings.Repeat("a", 200)

	// then
	// ... it is capped to 100
	a.Len([]rune(sanitizeName(long)), 100)
}

func TestSanitizeName_UnicodeNFC(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a decomposed unicode string (U+0065 U+0301)
	decomposed := "é"

	// when
	// ... the name is sanitized
	out := sanitizeName(decomposed)

	// then
	// ... it is NFC-normalized to U+00E9
	a.Equal("é", out)
}

func TestSanitizeName_AllSeparatorsYieldsEmpty(t *testing.T) {
	a := assert.New(t)

	// given / when / then
	// ... all-separator names sanitize to empty
	a.Equal("", sanitizeName("///"))
	a.Equal("", sanitizeName("..."))
}

func TestSizeCap_ImageVsDoc(t *testing.T) {
	a := assert.New(t)

	// given / when / then
	// ... image MIMEs get the image cap; others get the doc cap
	a.Equal(MaxImageBytes, SizeCap("image/png"))
	a.Equal(MaxImageBytes, SizeCap("image/jpeg"))
	a.Equal(MaxDocBytes, SizeCap("application/pdf"))
	a.Equal(MaxDocBytes, SizeCap(""))
}

func TestValidExt(t *testing.T) {
	a := assert.New(t)

	// given / when / then
	// ... extension validity rules
	a.True(validExt(".png"))
	a.True(validExt(".jpeg"))
	a.True(validExt(".pdf"))
	a.False(validExt(""))
	a.False(validExt("."))
	a.False(validExt(".has-dash"))
	a.False(validExt(".verylongextension12"))
}
