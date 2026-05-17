package permission

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TheLazyLemur/switchboard/internal/core"
)

var _ core.PermissionChecker = (*Checker)(nil)

var readOnlyTools = map[string]bool{
	"Read":      true,
	"Glob":      true,
	"Grep":      true,
	"WebFetch":  true,
	"WebSearch": true,
}

// Checker enforces path containment against allowedDirs and, when
// readOnly is set, restricts tool calls to a fixed read-only set.
type Checker struct {
	allowedDirs []string
	readOnly    bool
}

func NewAutoApprovePermissionChecker(allowedDirs []string) *Checker {
	return &Checker{allowedDirs: cleanDirs(allowedDirs)}
}

func NewReadOnlyPermissionChecker(allowedDirs []string) *Checker {
	return &Checker{allowedDirs: cleanDirs(allowedDirs), readOnly: true}
}

func cleanDirs(dirs []string) []string {
	cleaned := make([]string, len(dirs))
	for i, dir := range dirs {
		cleaned[i] = filepath.Clean(dir)
	}
	return cleaned
}

func (c *Checker) Check(toolName string, input core.ToolInput) (bool, string) {
	if c.readOnly && !readOnlyTools[toolName] {
		return false, fmt.Sprintf("read-only mode: %s not allowed", toolName)
	}
	for _, path := range extractPaths(input) {
		if !c.isAllowed(path) {
			return false, fmt.Sprintf("path %s is outside allowed directories", path)
		}
	}
	return true, ""
}

func (c *Checker) isAllowed(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, allowed := range c.allowedDirs {
		if cleanPath == allowed || strings.HasPrefix(cleanPath, allowed+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func extractPaths(input core.ToolInput) []string {
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
