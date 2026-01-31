package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SkillStore provides access to skills.
type SkillStore interface {
	List() ([]SkillMetadata, error)
	Load(name string) (*Skill, error)
	LoadSupporting(name, path string) ([]byte, error)
}

// FSSkillStore reads skills from the filesystem.
type FSSkillStore struct {
	baseDir string
}

// NewFSSkillStore creates a store reading from the given directory.
func NewFSSkillStore(baseDir string) *FSSkillStore {
	return &FSSkillStore{baseDir: baseDir}
}

// List returns metadata for all valid skills in the directory.
func (s *FSSkillStore) List() ([]SkillMetadata, error) {
	entries, err := os.ReadDir(s.baseDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading skills dir: %w", err)
	}

	var skills []SkillMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		skillPath := filepath.Join(s.baseDir, entry.Name(), "SKILL.md")
		content, err := os.ReadFile(skillPath)
		if err != nil {
			continue // skip dirs without SKILL.md
		}

		meta, err := ParseMetadata(string(content), skillPath)
		if err != nil {
			continue // skip invalid skills
		}

		skills = append(skills, *meta)
	}

	return skills, nil
}

// Load returns the full skill including instructions.
func (s *FSSkillStore) Load(name string) (*Skill, error) {
	skillPath := filepath.Join(s.baseDir, name, "SKILL.md")
	content, err := os.ReadFile(skillPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("skill not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("reading skill: %w", err)
	}

	return ParseSkill(string(content), skillPath)
}

// LoadSupporting reads a supporting file from a skill directory.
func (s *FSSkillStore) LoadSupporting(name, relPath string) ([]byte, error) {
	if err := validateRelativePath(relPath); err != nil {
		return nil, err
	}

	skillDir := filepath.Join(s.baseDir, name)
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); os.IsNotExist(err) {
		return nil, fmt.Errorf("skill not found: %s", name)
	}

	fullPath := filepath.Join(skillDir, relPath)

	// Double-check path is still within skill directory after resolution
	resolved, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, fmt.Errorf("resolving path: %w", err)
	}
	skillDirAbs, _ := filepath.Abs(skillDir)
	if !strings.HasPrefix(resolved, skillDirAbs+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid path: escapes skill directory")
	}

	return os.ReadFile(fullPath)
}

func validateRelativePath(p string) error {
	if filepath.IsAbs(p) {
		return fmt.Errorf("invalid path: must be relative")
	}
	if strings.Contains(p, "..") {
		return fmt.Errorf("invalid path: cannot contain ..")
	}
	return nil
}
