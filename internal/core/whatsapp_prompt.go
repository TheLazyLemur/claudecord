package core

import (
	"encoding/xml"
	"strings"
)

// RenderWhatsAppBatch renders a batch of buffered WhatsApp messages as the
// XML-tagged prompt body the model sees. Empty-caption messages still produce
// a <message> wrapper. Empty batch → empty string.
func RenderWhatsAppBatch(msgs []BufferedMessage) string {
	if len(msgs) == 0 {
		return ""
	}

	var b strings.Builder
	for i, m := range msgs {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString("<message>\n")
		if m.Content != "" {
			b.WriteString("  <text>")
			b.WriteString(escapeXML(m.Content))
			b.WriteString("</text>\n")
		}
		b.WriteString("</message>")
	}
	return b.String()
}

func escapeXML(s string) string {
	var sb strings.Builder
	xml.EscapeText(&sb, []byte(s))
	return sb.String()
}

func escapeXMLAttr(s string) string {
	// Same logic as escapeXML; xml.EscapeText also escapes the chars relevant
	// in attribute values (&, <, >, "). We need to also escape ' for attrs.
	out := escapeXML(s)
	out = strings.ReplaceAll(out, "'", "&#39;")
	return out
}

// WhatsAppMediaSystemPromptAddendum is appended to the chat system prompt when
// the WhatsApp channel is active. It tells the model how to consume the
// <attachment> tags emitted by RenderWhatsAppBatch.
const WhatsAppMediaSystemPromptAddendum = `
When the user message contains <attachment> tags, treat each tag as an inbound
file the user just sent. For image MIME types, call Read on the path attribute
to view the image. For other MIME types, consult <available_skills> for one
whose description matches the mime attribute and follow its instructions.`
