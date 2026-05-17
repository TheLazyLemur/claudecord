package discord

import (
	"fmt"
	"mime"
	"net/http"
	"time"

	"github.com/TheLazyLemur/switchboard/internal/core"
	"github.com/TheLazyLemur/switchboard/internal/media"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

const (
	MaxImageBytes = media.MaxImageBytes
	MaxDocBytes   = media.MaxDocBytes
)

// Downloader fetches raw bytes from a URL (Discord CDN).
type Downloader interface {
	Download(url string) ([]byte, error)
}

// HTTPDownloader is the production Downloader using a plain HTTP client.
type HTTPDownloader struct {
	Client *http.Client
}

func (d *HTTPDownloader) Download(url string) ([]byte, error) {
	resp, err := d.Client.Get(url) //nolint:noctx
	if err != nil {
		return nil, errors.Wrap(err, "http get")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("unexpected status %d for %s", resp.StatusCode, url)
	}
	buf := make([]byte, 0, 512)
	tmp := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf, nil
}

// parseMediaType strips parameters (e.g. "; charset=utf-8") from a MIME type.
func parseMediaType(ct string) string {
	if ct == "" {
		return ""
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return ct
	}
	return mt
}

// extractAttachments downloads each attachment from the Discord message, saves
// the ones within size caps to mediaDir, and returns their AttachmentRefs.
// Attachments that are too large or fail to download produce a "skipped" notice
// string instead of an error. The function only returns a non-nil error for
// unrecoverable conditions (e.g. disk write failure).
func extractAttachments(ev *discordgo.MessageCreate, dl Downloader, mediaDir string) ([]core.AttachmentRef, []string, error) {
	if ev == nil || len(ev.Attachments) == 0 {
		return nil, nil, nil
	}

	var refs []core.AttachmentRef
	var skipped []string

	for _, att := range ev.Attachments {
		mimeType := parseMediaType(att.ContentType)
		name := att.Filename
		if name == "" {
			name = att.ID
		}

		data, err := dl.Download(att.URL)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("skipped (download error): %s", name))
			continue
		}

		if len(data) > media.SizeCap(mimeType) {
			skipped = append(skipped, fmt.Sprintf("skipped (too large): %s", name))
			continue
		}

		mediaAtt := &media.Attachment{
			MIME:         mimeType,
			OriginalName: name,
			Bytes:        data,
		}
		path, err := media.SaveAttachment(mediaDir, mediaAtt, time.Now())
		if err != nil {
			return refs, skipped, errors.Wrap(err, "saving discord attachment")
		}

		refs = append(refs, core.AttachmentRef{
			Path:         path,
			MIME:         mimeType,
			OriginalName: name,
		})
	}

	return refs, skipped, nil
}
