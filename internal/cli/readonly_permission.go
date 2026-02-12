package cli

import (
	"fmt"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

var _ core.PermissionChecker = (*ReadOnlyPermissionChecker)(nil)

var readOnlyTools = map[string]bool{
	"Read":      true,
	"Glob":      true,
	"Grep":      true,
	"WebFetch":  true,
	"WebSearch": true,
}

type ReadOnlyPermissionChecker struct {
	PathValidator
}

func NewReadOnlyPermissionChecker(allowedDirs []string) *ReadOnlyPermissionChecker {
	return &ReadOnlyPermissionChecker{PathValidator: NewPathValidator(allowedDirs)}
}

func (p *ReadOnlyPermissionChecker) Check(toolName string, input map[string]any) (allow bool, reason string) {
	if !readOnlyTools[toolName] {
		return false, fmt.Sprintf("read-only mode: %s not allowed", toolName)
	}

	paths := p.ExtractPaths(input)
	for _, path := range paths {
		if !p.IsAllowed(path) {
			return false, fmt.Sprintf("path %s is outside allowed directories", path)
		}
	}
	return true, ""
}
