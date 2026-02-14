package permission

import (
	"fmt"
	"strings"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

var _ core.PermissionChecker = (*PermissionChecker)(nil)

type PermissionChecker struct {
	PathValidator
}

func NewPermissionChecker(allowedDirs []string) *PermissionChecker {
	return &PermissionChecker{PathValidator: NewPathValidator(allowedDirs)}
}

func (p *PermissionChecker) Check(toolName string, input core.ToolInput) (allow bool, reason string) {
	// Read auto-approves if path in allowed dirs
	if toolName == "Read" {
		paths := p.ExtractPaths(input)
		for _, path := range paths {
			if !p.IsAllowed(path) {
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
		if input.Method == "" || strings.ToUpper(input.Method) == "GET" {
			return true, ""
		}
		return false, "requires approval"
	}

	// everything else requires user approval
	return false, "requires approval"
}
