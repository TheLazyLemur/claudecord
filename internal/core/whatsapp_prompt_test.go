package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderWhatsAppBatch_Empty(t *testing.T) {
	assert.Equal(t, "", RenderWhatsAppBatch(nil))
	assert.Equal(t, "", RenderWhatsAppBatch([]BufferedMessage{}))
}

func TestRenderWhatsAppBatch_TextOnly(t *testing.T) {
	out := RenderWhatsAppBatch([]BufferedMessage{
		{Content: "hello"},
	})
	assert.Equal(t, "<message>\n  <text>hello</text>\n</message>", out)
}

func TestRenderWhatsAppBatch_AttachmentOnlyOmitsTextElement(t *testing.T) {
	out := RenderWhatsAppBatch([]BufferedMessage{
		{
			Attachments: []AttachmentRef{
				{Path: "/p/photo.png", MIME: "image/png"},
			},
		},
	})
	// Attachment-only message: no text element, no attachment tag (tags now
	// rendered by the API backend via Inbound.Attachments).
	assert.NotContains(t, out, "<text>")
	assert.NotContains(t, out, "<attachment")
}

func TestRenderWhatsAppBatch_TextAndAttachment(t *testing.T) {
	out := RenderWhatsAppBatch([]BufferedMessage{
		{
			Content: "look",
			Attachments: []AttachmentRef{
				{Path: "/p/photo.png", MIME: "image/png", OriginalName: "photo.png"},
			},
		},
	})
	assert.Contains(t, out, "<text>look</text>")
	// Attachment tags are no longer inline; they are rendered by the API backend.
	assert.NotContains(t, out, "<attachment")
}

func TestRenderWhatsAppBatch_PreservesMessageOrder(t *testing.T) {
	out := RenderWhatsAppBatch([]BufferedMessage{
		{Content: "one"},
		{Content: "two"},
		{Content: "three"},
	})

	// Each <message> appears exactly once, in order.
	idx1 := strings.Index(out, "one")
	idx2 := strings.Index(out, "two")
	idx3 := strings.Index(out, "three")
	assert.True(t, idx1 < idx2 && idx2 < idx3, "out=%q", out)
}

func TestRenderWhatsAppBatch_XMLEscapesText(t *testing.T) {
	out := RenderWhatsAppBatch([]BufferedMessage{
		{Content: `<script>alert("x") & 'y'</script>`},
	})
	assert.NotContains(t, out, "<script>")
	assert.Contains(t, out, "&lt;script&gt;")
	assert.Contains(t, out, "&amp;")
}

func TestRenderWhatsAppBatch_XMLEscapesAttributes(t *testing.T) {
	// Attachment tags are rendered by the API backend (renderUserMessage), not
	// by RenderWhatsAppBatch. This test verifies text content with special chars
	// is still XML-escaped in the <text> element.
	out := RenderWhatsAppBatch([]BufferedMessage{
		{
			Content: `say "hello" & goodbye`,
		},
	})
	assert.NotContains(t, out, `"hello"`)
	assert.Contains(t, out, "&amp;")
}

func TestRenderWhatsAppBatch_MultipleAttachmentsSameMessage(t *testing.T) {
	out := RenderWhatsAppBatch([]BufferedMessage{
		{
			Content: "two pics",
			Attachments: []AttachmentRef{
				{Path: "/p/a.png", MIME: "image/png"},
				{Path: "/p/b.png", MIME: "image/png"},
			},
		},
	})
	// Attachment tags are no longer emitted inline; the message wrapper is present.
	assert.Equal(t, 1, strings.Count(out, "<message>"))
	assert.Equal(t, 0, strings.Count(out, "<attachment "))
}
