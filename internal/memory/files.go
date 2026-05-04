// Package memory provides file-level helpers for the dashboard memory editor.
// All operations are constrained to a single memoryDir; absolute paths and
// parent-traversal segments are rejected.
package memory

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

// List returns relative paths of every regular file under memoryDir.
func List(memoryDir string) ([]string, error) {
	if memoryDir == "" {
		return nil, nil
	}
	var out []string
	err := filepath.WalkDir(memoryDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(memoryDir, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Read returns the contents of relPath inside memoryDir.
func Read(memoryDir, relPath string) (string, error) {
	abs, err := safePath(memoryDir, relPath)
	if err != nil {
		return "", err
	}
	body, err := os.ReadFile(abs)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// Write overwrites relPath inside memoryDir, creating parent directories as
// needed.
func Write(memoryDir, relPath, content string) error {
	abs, err := safePath(memoryDir, relPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	return os.WriteFile(abs, []byte(content), 0o644)
}

// Delete removes relPath inside memoryDir.
func Delete(memoryDir, relPath string) error {
	abs, err := safePath(memoryDir, relPath)
	if err != nil {
		return err
	}
	return os.Remove(abs)
}

func safePath(memoryDir, relPath string) (string, error) {
	if memoryDir == "" {
		return "", errors.New("memoryDir is empty")
	}
	if relPath == "" {
		return "", errors.New("path is empty")
	}
	if filepath.IsAbs(relPath) {
		return "", errors.Errorf("absolute path rejected: %q", relPath)
	}
	clean := filepath.Clean(relPath)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", errors.Errorf("path escape rejected: %q", relPath)
	}
	abs := filepath.Join(memoryDir, clean)
	rootAbs, err := filepath.Abs(memoryDir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	if resolved != rootAbs && !strings.HasPrefix(resolved, rootAbs+string(filepath.Separator)) {
		return "", errors.Errorf("path escape rejected: %q", relPath)
	}
	return abs, nil
}
