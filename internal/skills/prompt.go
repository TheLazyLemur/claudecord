package skills

import (
	"html"
	"strings"
)

// GenerateSkillsXML generates the <available_skills> XML block for system prompts.
func GenerateSkillsXML(skills []SkillMetadata) string {
	var b strings.Builder
	b.WriteString("<available_skills>\n")

	for _, s := range skills {
		b.WriteString("  <skill>\n")
		b.WriteString("    <name>")
		b.WriteString(html.EscapeString(s.Name))
		b.WriteString("</name>\n")
		b.WriteString("    <description>")
		b.WriteString(html.EscapeString(s.Description))
		b.WriteString("</description>\n")
		b.WriteString("    <location>")
		b.WriteString(html.EscapeString(s.Path))
		b.WriteString("</location>\n")
		b.WriteString("  </skill>\n")
	}

	b.WriteString("</available_skills>")
	return b.String()
}
