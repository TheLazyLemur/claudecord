package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

var _ core.PermissionChecker = (*PermissionChecker)(nil)

var pathFields = []string{"file_path", "path", "directory"}

type PermissionChecker struct {
	allowedDirs []string
}

func NewPermissionChecker(allowedDirs []string) *PermissionChecker {
	// Clean all allowed dirs to normalize paths
	cleaned := make([]string, len(allowedDirs))
	for i, dir := range allowedDirs {
		cleaned[i] = filepath.Clean(dir)
	}
	return &PermissionChecker{allowedDirs: cleaned}
}

func (p *PermissionChecker) Check(toolName string, input map[string]any) (allow bool, reason string) {
	// Read auto-approves if path in allowed dirs
	if toolName == "Read" {
		paths := p.extractPaths(input)
		for _, path := range paths {
			if !p.isAllowed(path) {
				return false, fmt.Sprintf("path %s is outside allowed directories", path)
			}
		}
		return true, ""
	}

	// Skill tools always auto-approve
	if toolName == "Skill" || toolName == "LoadSkillSupporting" {
		return true, ""
	}

	// WebSearch auto-approves (read-only)
	if toolName == "WebSearch" {
		return true, ""
	}

	// Fetch auto-approves for GET, requires approval for mutating methods
	if toolName == "Fetch" {
		method, _ := input["method"].(string)
		if method == "" || strings.ToUpper(method) == "GET" {
			return true, ""
		}
		return false, "requires approval"
	}

	// everything else requires user approval
	return false, "requires approval"
}

func (p *PermissionChecker) extractPaths(input map[string]any) []string {
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

func (p *PermissionChecker) isAllowed(path string) bool {
	// Clean/resolve path to handle .. traversal
	cleanPath := filepath.Clean(path)

	for _, allowed := range p.allowedDirs {
		if cleanPath == allowed || strings.HasPrefix(cleanPath, allowed+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
