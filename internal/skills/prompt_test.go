package skills

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGenerateSkillsXML(t *testing.T) {
	t.Run("generates valid xml", func(t *testing.T) {
		skills := []SkillMetadata{
			{Name: "commit", Description: "Create git commits.", Path: "/skills/commit/SKILL.md"},
			{Name: "review", Description: "Review code changes.", Path: "/skills/review/SKILL.md"},
		}

		xml := GenerateSkillsXML(skills)

		assert.Contains(t, xml, "<available_skills>")
		assert.Contains(t, xml, "</available_skills>")
		assert.Contains(t, xml, "<skill>")
		assert.Contains(t, xml, "<name>commit</name>")
		assert.Contains(t, xml, "<description>Create git commits.</description>")
		assert.Contains(t, xml, "<location>/skills/commit/SKILL.md</location>")
		assert.Contains(t, xml, "<name>review</name>")
	})

	t.Run("empty skills list", func(t *testing.T) {
		xml := GenerateSkillsXML(nil)

		assert.Contains(t, xml, "<available_skills>")
		assert.Contains(t, xml, "</available_skills>")
		assert.NotContains(t, xml, "<skill>")
	})

	t.Run("escapes xml special chars", func(t *testing.T) {
		skills := []SkillMetadata{
			{Name: "test", Description: "Use <tags> & stuff.", Path: "/path"},
		}

		xml := GenerateSkillsXML(skills)

		assert.Contains(t, xml, "&lt;tags&gt;")
		assert.Contains(t, xml, "&amp;")
	})

	t.Run("single skill", func(t *testing.T) {
		skills := []SkillMetadata{
			{Name: "solo", Description: "Single skill.", Path: "/skills/solo/SKILL.md"},
		}

		xml := GenerateSkillsXML(skills)

		assert.Equal(t, 1, strings.Count(xml, "<skill>"))
		assert.Equal(t, 1, strings.Count(xml, "</skill>"))
	})
}
