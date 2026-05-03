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
	assert.NotContains(t, out, "<text>")
	assert.Contains(t, out, `<attachment path="/p/photo.png" mime="image/png" />`)
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
	assert.Contains(t, out, `original_name="photo.png"`)
}

func TestRenderWhatsAppBatch_PreservesMessageOrder(t *testing.T) {
	out := RenderWhatsAppBatch([]BufferedMessage{
		{Content: "one"},
		{
			Attachments: []AttachmentRef{{Path: "/p/two.pdf", MIME: "application/pdf"}},
		},
		{Content: "three"},
	})

	// Each <message> appears exactly once, in order.
	idx1 := strings.Index(out, "one")
	idx2 := strings.Index(out, "two.pdf")
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
	out := RenderWhatsAppBatch([]BufferedMessage{
		{
			Attachments: []AttachmentRef{
				{
					Path:         `/p/a"b.png`,
					MIME:         "image/png",
					OriginalName: `weird"name.png`,
				},
			},
		},
	})
	// Quotes inside attribute values are escaped, so the attribute boundaries stay valid.
	assert.NotContains(t, out, `a"b.png"`)
	assert.Contains(t, out, "&#34;")
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
	// Both attachments inside a single <message>.
	assert.Equal(t, 1, strings.Count(out, "<message>"))
	assert.Equal(t, 2, strings.Count(out, "<attachment "))
}
