package skills

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSkill(t *testing.T) {
	t.Run("valid skill with all fields", func(t *testing.T) {
		content := `---
name: test-skill
description: A test skill for unit testing.
license: MIT
metadata:
  author: test-org
  version: "1.0"
---
# Instructions

Do the thing.

## Steps
1. First step
2. Second step
`
		skill, err := ParseSkill(content, "/path/to/test-skill/SKILL.md")

		require.NoError(t, err)
		assert.Equal(t, "test-skill", skill.Name)
		assert.Equal(t, "A test skill for unit testing.", skill.Description)
		assert.Equal(t, "/path/to/test-skill/SKILL.md", skill.Path)
		assert.Contains(t, skill.Instructions, "# Instructions")
		assert.Contains(t, skill.Instructions, "Do the thing.")
		assert.Contains(t, skill.Instructions, "1. First step")
	})

	t.Run("minimal valid skill", func(t *testing.T) {
		content := `---
name: minimal
description: Minimal skill.
---
Just do it.
`
		skill, err := ParseSkill(content, "/skills/minimal/SKILL.md")

		require.NoError(t, err)
		assert.Equal(t, "minimal", skill.Name)
		assert.Equal(t, "Minimal skill.", skill.Description)
		assert.Equal(t, "Just do it.\n", skill.Instructions)
	})

	t.Run("missing name", func(t *testing.T) {
		content := `---
description: No name field.
---
Instructions here.
`
		_, err := ParseSkill(content, "/path/SKILL.md")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("missing description", func(t *testing.T) {
		content := `---
name: no-desc
---
Instructions here.
`
		_, err := ParseSkill(content, "/path/SKILL.md")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "description")
	})

	t.Run("no frontmatter", func(t *testing.T) {
		content := `Just markdown without frontmatter.`

		_, err := ParseSkill(content, "/path/SKILL.md")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "frontmatter")
	})

	t.Run("invalid yaml", func(t *testing.T) {
		content := `---
name: [invalid yaml
description: broken
---
Instructions.
`
		_, err := ParseSkill(content, "/path/SKILL.md")

		require.Error(t, err)
	})

	t.Run("empty instructions allowed", func(t *testing.T) {
		content := `---
name: empty-body
description: Skill with empty body.
---
`
		skill, err := ParseSkill(content, "/path/SKILL.md")

		require.NoError(t, err)
		assert.Equal(t, "empty-body", skill.Name)
		assert.Empty(t, skill.Instructions)
	})

	t.Run("invalid name format - uppercase", func(t *testing.T) {
		content := `---
name: InvalidName
description: Name has uppercase.
---
Instructions.
`
		_, err := ParseSkill(content, "/path/SKILL.md")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "name")
	})

	t.Run("invalid name format - starts with hyphen", func(t *testing.T) {
		content := `---
name: -invalid
description: Name starts with hyphen.
---
Instructions.
`
		_, err := ParseSkill(content, "/path/SKILL.md")

		require.Error(t, err)
	})

	t.Run("invalid name format - consecutive hyphens", func(t *testing.T) {
		content := `---
name: invalid--name
description: Name has consecutive hyphens.
---
Instructions.
`
		_, err := ParseSkill(content, "/path/SKILL.md")

		require.Error(t, err)
	})
}

func TestParseMetadata(t *testing.T) {
	t.Run("extracts only metadata", func(t *testing.T) {
		content := `---
name: meta-test
description: Test metadata extraction.
---
Long instructions that should not be loaded.
`
		meta, err := ParseMetadata(content, "/path/meta-test/SKILL.md")

		require.NoError(t, err)
		assert.Equal(t, "meta-test", meta.Name)
		assert.Equal(t, "Test metadata extraction.", meta.Description)
		assert.Equal(t, "/path/meta-test/SKILL.md", meta.Path)
	})
}
