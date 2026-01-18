package cli

import (
	"fmt"
	"path/filepath"
	"strings"

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
	allowedDirs []string
}

func NewReadOnlyPermissionChecker(allowedDirs []string) *ReadOnlyPermissionChecker {
	cleaned := make([]string, len(allowedDirs))
	for i, dir := range allowedDirs {
		cleaned[i] = filepath.Clean(dir)
	}
	return &ReadOnlyPermissionChecker{allowedDirs: cleaned}
}

func (p *ReadOnlyPermissionChecker) Check(toolName string, input map[string]any) (allow bool, reason string) {
	if !readOnlyTools[toolName] {
		return false, fmt.Sprintf("read-only mode: %s not allowed", toolName)
	}

	paths := p.extractPaths(input)
	for _, path := range paths {
		if !p.isAllowed(path) {
			return false, fmt.Sprintf("path %s is outside allowed directories", path)
		}
	}
	return true, ""
}

func (p *ReadOnlyPermissionChecker) extractPaths(input map[string]any) []string {
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

func (p *ReadOnlyPermissionChecker) isAllowed(path string) bool {
	cleanPath := filepath.Clean(path)
	for _, allowed := range p.allowedDirs {
		if cleanPath == allowed || strings.HasPrefix(cleanPath, allowed+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
