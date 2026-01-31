package skills

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFSSkillStore_List(t *testing.T) {
	t.Run("lists all valid skills", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkill(t, dir, "skill-one", "First skill description.")
		createTestSkill(t, dir, "skill-two", "Second skill description.")

		store := NewFSSkillStore(dir)
		skills, err := store.List()

		require.NoError(t, err)
		assert.Len(t, skills, 2)

		names := []string{skills[0].Name, skills[1].Name}
		assert.Contains(t, names, "skill-one")
		assert.Contains(t, names, "skill-two")
	})

	t.Run("skips invalid skills", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkill(t, dir, "valid-skill", "Valid skill.")

		// Create invalid skill (missing description)
		invalidDir := filepath.Join(dir, "invalid-skill")
		require.NoError(t, os.MkdirAll(invalidDir, 0755))
		require.NoError(t, os.WriteFile(
			filepath.Join(invalidDir, "SKILL.md"),
			[]byte("---\nname: invalid-skill\n---\nNo desc."),
			0644,
		))

		store := NewFSSkillStore(dir)
		skills, err := store.List()

		require.NoError(t, err)
		assert.Len(t, skills, 1)
		assert.Equal(t, "valid-skill", skills[0].Name)
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()

		store := NewFSSkillStore(dir)
		skills, err := store.List()

		require.NoError(t, err)
		assert.Empty(t, skills)
	})

	t.Run("nonexistent directory", func(t *testing.T) {
		store := NewFSSkillStore("/nonexistent/path")
		skills, err := store.List()

		require.NoError(t, err)
		assert.Empty(t, skills)
	})
}

func TestFSSkillStore_Load(t *testing.T) {
	t.Run("loads full skill", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkillWithBody(t, dir, "my-skill", "My skill desc.", "# Do stuff\n\n1. Step one")

		store := NewFSSkillStore(dir)
		skill, err := store.Load("my-skill")

		require.NoError(t, err)
		assert.Equal(t, "my-skill", skill.Name)
		assert.Equal(t, "My skill desc.", skill.Description)
		assert.Contains(t, skill.Instructions, "# Do stuff")
		assert.Contains(t, skill.Instructions, "1. Step one")
	})

	t.Run("skill not found", func(t *testing.T) {
		dir := t.TempDir()

		store := NewFSSkillStore(dir)
		_, err := store.Load("nonexistent")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestFSSkillStore_LoadSupporting(t *testing.T) {
	t.Run("loads reference file", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkill(t, dir, "my-skill", "My skill.")
		createSupportingFile(t, dir, "my-skill", "references/API.md", "# API Reference\n\nEndpoints here.")

		store := NewFSSkillStore(dir)
		content, err := store.LoadSupporting("my-skill", "references/API.md")

		require.NoError(t, err)
		assert.Contains(t, string(content), "# API Reference")
	})

	t.Run("loads script file", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkill(t, dir, "my-skill", "My skill.")
		createSupportingFile(t, dir, "my-skill", "scripts/run.sh", "#!/bin/bash\necho hello")

		store := NewFSSkillStore(dir)
		content, err := store.LoadSupporting("my-skill", "scripts/run.sh")

		require.NoError(t, err)
		assert.Contains(t, string(content), "#!/bin/bash")
	})

	t.Run("prevents path traversal - parent dir", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkill(t, dir, "my-skill", "My skill.")

		store := NewFSSkillStore(dir)
		_, err := store.LoadSupporting("my-skill", "../other-skill/SKILL.md")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid path")
	})

	t.Run("prevents path traversal - absolute path", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkill(t, dir, "my-skill", "My skill.")

		store := NewFSSkillStore(dir)
		_, err := store.LoadSupporting("my-skill", "/etc/passwd")

		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid path")
	})

	t.Run("file not found", func(t *testing.T) {
		dir := t.TempDir()
		createTestSkill(t, dir, "my-skill", "My skill.")

		store := NewFSSkillStore(dir)
		_, err := store.LoadSupporting("my-skill", "references/nonexistent.md")

		require.Error(t, err)
	})

	t.Run("skill not found", func(t *testing.T) {
		dir := t.TempDir()

		store := NewFSSkillStore(dir)
		_, err := store.LoadSupporting("nonexistent", "references/file.md")

		require.Error(t, err)
	})
}

// Test helpers

func createTestSkill(t *testing.T, baseDir, name, description string) {
	t.Helper()
	createTestSkillWithBody(t, baseDir, name, description, "Default instructions.")
}

func createTestSkillWithBody(t *testing.T, baseDir, name, description, body string) {
	t.Helper()
	skillDir := filepath.Join(baseDir, name)
	require.NoError(t, os.MkdirAll(skillDir, 0755))

	content := "---\nname: " + name + "\ndescription: " + description + "\n---\n" + body
	require.NoError(t, os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0644))
}

func createSupportingFile(t *testing.T, baseDir, skillName, relPath, content string) {
	t.Helper()
	fullPath := filepath.Join(baseDir, skillName, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0755))
	require.NoError(t, os.WriteFile(fullPath, []byte(content), 0644))
}
