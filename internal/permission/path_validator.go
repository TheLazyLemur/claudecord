package permission

import (
	"path/filepath"
	"strings"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

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

func (v PathValidator) ExtractPaths(input core.ToolInput) []string {
	var paths []string
	if input.FilePath != "" {
		paths = append(paths, input.FilePath)
	}
	if input.Path != "" {
		paths = append(paths, input.Path)
	}
	if input.Directory != "" {
		paths = append(paths, input.Directory)
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
