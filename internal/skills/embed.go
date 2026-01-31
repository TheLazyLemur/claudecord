package skills

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed builtin/*
var builtinSkills embed.FS

// DumpBuiltinSkills writes all embedded builtin skills to the target directory.
func DumpBuiltinSkills(targetDir string) error {
	return fs.WalkDir(builtinSkills, "builtin", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Get relative path without "builtin/" prefix
		relPath, _ := filepath.Rel("builtin", path)
		if relPath == "." {
			return nil
		}

		targetPath := filepath.Join(targetDir, relPath)

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}

		content, err := builtinSkills.ReadFile(path)
		if err != nil {
			return err
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}

		return os.WriteFile(targetPath, content, 0644)
	})
}

// DefaultSkillsDir returns the default directory for builtin skills.
func DefaultSkillsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claudecord", "skills", "builtin"), nil
}
