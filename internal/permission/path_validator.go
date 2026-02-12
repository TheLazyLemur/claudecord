package permission

import (
	"path/filepath"
	"strings"
)

var pathFields = []string{"file_path", "path", "directory"}

type PathValidator struct {
	allowedDirs []string
}

func NewPathValidator(allowedDirs []string) PathValidator {
	cleaned := make([]string, len(allowedDirs))
	for i, dir := range allowedDirs {
		cleaned[i] = filepath.Clean(dir)
	}
	return PathValidator{allowedDirs: cleaned}
}

func (v PathValidator) ExtractPaths(input map[string]any) []string {
	var paths []string
	for _, field := range pathFields {
		if val, ok := input[field]; ok {
			if s, ok := val.(string); ok && s != "" {
				paths = append(paths, s)
			}
		}
	}
	return paths
}

func (v PathValidator) IsAllowed(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, allowed := range v.allowedDirs {
		if cleanPath == allowed || strings.HasPrefix(cleanPath, allowed+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
