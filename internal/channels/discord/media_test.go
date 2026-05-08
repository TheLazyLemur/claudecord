package discord

import (
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stub Downloader ---

type stubDownloader struct {
	body []byte
	err  error
}

func (s *stubDownloader) Download(_ string) ([]byte, error) {
	return s.body, s.err
}

// --- helpers ---

func makeAttachment(id, filename, contentType, url string) *discordgo.MessageAttachment {
	return &discordgo.MessageAttachment{
		ID:          id,
		Filename:    filename,
		ContentType: contentType,
		URL:         url,
		Size:        len(url), // not used by extractAttachments
	}
}

func makeEvent(attachments []*discordgo.MessageAttachment) *discordgo.MessageCreate {
	return &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:          "msg-1",
			Attachments: attachments,
		},
	}
}

// --- tests ---

func TestExtractAttachments_ZeroAttachments_EmptyResult(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a MessageCreate with no attachments
	dir := t.TempDir()
	ev := makeEvent(nil)
	dl := &stubDownloader{}

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... the result is empty with no errors or skipped notices
	r.NoError(err)
	a.Empty(refs)
	a.Empty(skipped)
}

func TestExtractAttachments_ImageUnderCap_Kept(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a MessageCreate with a small image attachment
	dir := t.TempDir()
	payload := []byte("PNGDATA")
	dl := &stubDownloader{body: payload}
	ev := makeEvent([]*discordgo.MessageAttachment{
		makeAttachment("att-1", "photo.png", "image/png", "https://cdn.discordapp.com/photo.png"),
	})

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... one ref is returned, the file exists on disk, and nothing was skipped
	r.NoError(err)
	a.Empty(skipped)
	r.Len(refs, 1)
	a.Equal("image/png", refs[0].MIME)
	a.Equal("photo.png", refs[0].OriginalName)
	data, err := os.ReadFile(refs[0].Path)
	r.NoError(err)
	a.Equal(payload, data)
	// path must be under the media dir
	a.True(strings.HasPrefix(refs[0].Path, dir))
}

func TestExtractAttachments_ImageOverCap_DroppedWithNotice(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a MessageCreate with an oversized image attachment
	dir := t.TempDir()
	payload := make([]byte, MaxImageBytes+1)
	dl := &stubDownloader{body: payload}
	ev := makeEvent([]*discordgo.MessageAttachment{
		makeAttachment("att-2", "big.png", "image/png", "https://cdn.discordapp.com/big.png"),
	})

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... no ref is produced and a skipped notice names the file
	r.NoError(err)
	a.Empty(refs)
	r.Len(skipped, 1)
	a.Contains(skipped[0], "big.png")
	a.Contains(skipped[0], "skipped (too large)")
}

func TestExtractAttachments_DocUnderCap_Kept(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a MessageCreate with a small PDF attachment
	dir := t.TempDir()
	payload := []byte("%PDF-1.4")
	dl := &stubDownloader{body: payload}
	ev := makeEvent([]*discordgo.MessageAttachment{
		makeAttachment("att-3", "report.pdf", "application/pdf", "https://cdn.discordapp.com/report.pdf"),
	})

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... the PDF ref is returned with correct metadata
	r.NoError(err)
	a.Empty(skipped)
	r.Len(refs, 1)
	a.Equal("application/pdf", refs[0].MIME)
	a.Equal("report.pdf", refs[0].OriginalName)
	a.True(strings.HasPrefix(refs[0].Path, dir))
}

func TestExtractAttachments_DocOverCap_DroppedWithNotice(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a MessageCreate with an oversized document attachment
	dir := t.TempDir()
	payload := make([]byte, MaxDocBytes+1)
	dl := &stubDownloader{body: payload}
	ev := makeEvent([]*discordgo.MessageAttachment{
		makeAttachment("att-4", "huge.pdf", "application/pdf", "https://cdn.discordapp.com/huge.pdf"),
	})

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... no ref is produced and a skipped notice names the file
	r.NoError(err)
	a.Empty(refs)
	r.Len(skipped, 1)
	a.Contains(skipped[0], "huge.pdf")
	a.Contains(skipped[0], "skipped (too large)")
}

func TestExtractAttachments_DownloadError_Skipped(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a downloader that always returns an error
	dir := t.TempDir()
	dl := &stubDownloader{err: &downloadErr{}}
	ev := makeEvent([]*discordgo.MessageAttachment{
		makeAttachment("att-5", "oops.png", "image/png", "https://cdn.discordapp.com/oops.png"),
	})

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... no ref is produced, no fatal error, and a skipped notice is emitted
	r.NoError(err)
	a.Empty(refs)
	r.Len(skipped, 1)
	a.Contains(skipped[0], "oops.png")
}

type downloadErr struct{}

func (e *downloadErr) Error() string { return "network error" }

func TestExtractAttachments_MixedAttachments_CorrectSplit(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... two attachments: one small image (kept) and one oversized doc (skipped)
	dir := t.TempDir()
	goodPayload := []byte("IMGDATA")
	calls := 0
	dl := &multiStubDownloader{
		payloads: [][]byte{goodPayload, make([]byte, MaxDocBytes+1)},
	}
	_ = calls
	ev := makeEvent([]*discordgo.MessageAttachment{
		makeAttachment("att-6", "photo.jpg", "image/jpeg", "https://cdn.discordapp.com/photo.jpg"),
		makeAttachment("att-7", "big.pdf", "application/pdf", "https://cdn.discordapp.com/big.pdf"),
	})

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... one ref is kept and one notice is emitted
	r.NoError(err)
	r.Len(refs, 1)
	a.Equal("image/jpeg", refs[0].MIME)
	r.Len(skipped, 1)
	a.Contains(skipped[0], "big.pdf")
}

type multiStubDownloader struct {
	payloads [][]byte
	idx      int
}

func (m *multiStubDownloader) Download(_ string) ([]byte, error) {
	if m.idx >= len(m.payloads) {
		return nil, nil
	}
	b := m.payloads[m.idx]
	m.idx++
	return b, nil
}

func TestExtractAttachments_ContentTypeFromAttachment(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... an attachment whose ContentType includes a charset parameter
	dir := t.TempDir()
	dl := &stubDownloader{body: []byte("data")}
	ev := makeEvent([]*discordgo.MessageAttachment{
		makeAttachment("att-8", "file.txt", "text/plain; charset=utf-8", "https://cdn.discordapp.com/file.txt"),
	})

	// when
	// ... extractAttachments is called
	refs, skipped, err := extractAttachments(ev, dl, dir)

	// then
	// ... the MIME is the media-type without parameters, the ref is kept
	r.NoError(err)
	a.Empty(skipped)
	r.Len(refs, 1)
	a.Equal("text/plain", refs[0].MIME)
}

func TestHTTPDownloader_ImplementsInterface(t *testing.T) {
	// given
	// ... a concrete HTTPDownloader
	dl := &HTTPDownloader{Client: http.DefaultClient}

	// when / then
	// ... it satisfies the Downloader interface
	var _ Downloader = dl
}
