package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDumpBuiltinSkills(t *testing.T) {
	t.Run("dumps embedded skills to directory", func(t *testing.T) {
		dir := t.TempDir()

		err := DumpBuiltinSkills(dir)

		require.NoError(t, err)

		// Check example skill was dumped
		skillPath := filepath.Join(dir, "example", "SKILL.md")
		content, err := os.ReadFile(skillPath)
		require.NoError(t, err)
		assert.Contains(t, string(content), "name: example")

		// Check email skills were dumped
		sendPath := filepath.Join(dir, "send-email", "SKILL.md")
		_, err = os.ReadFile(sendPath)
		require.NoError(t, err)

		receivePath := filepath.Join(dir, "receive-email", "SKILL.md")
		_, err = os.ReadFile(receivePath)
		require.NoError(t, err)

		// Check scripts were dumped
		sendScript := filepath.Join(dir, "send-email", "scripts", "send.sh")
		_, err = os.Stat(sendScript)
		assert.NoError(t, err)
	})

	t.Run("overwrites existing files", func(t *testing.T) {
		dir := t.TempDir()

		// Create existing file
		skillDir := filepath.Join(dir, "example")
		require.NoError(t, os.MkdirAll(skillDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("old content"), 0644))

		err := DumpBuiltinSkills(dir)

		require.NoError(t, err)
		content, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
		assert.NotContains(t, string(content), "old content")
		assert.Contains(t, string(content), "name: example")
	})

	t.Run("creates directory if not exists", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nested", "path")

		err := DumpBuiltinSkills(dir)

		require.NoError(t, err)
		_, err = os.Stat(filepath.Join(dir, "example", "SKILL.md"))
		assert.NoError(t, err)
	})
}

func TestDefaultSkillsDir(t *testing.T) {
	t.Run("returns path under home dir", func(t *testing.T) {
		dir, err := DefaultSkillsDir()

		require.NoError(t, err)
		assert.Contains(t, dir, ".claudecord")
		assert.Contains(t, dir, "skills")
		assert.Contains(t, dir, "builtin")
	})
}
