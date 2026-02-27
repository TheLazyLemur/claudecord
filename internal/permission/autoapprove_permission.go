package permission

import (
	"fmt"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

var _ core.PermissionChecker = (*AutoApprovePermissionChecker)(nil)

type AutoApprovePermissionChecker struct {
	PathValidator
}

func NewAutoApprovePermissionChecker(allowedDirs []string) *AutoApprovePermissionChecker {
	return &AutoApprovePermissionChecker{PathValidator: NewPathValidator(allowedDirs)}
}

func (p *AutoApprovePermissionChecker) Check(toolName string, input core.ToolInput) (allow bool, reason string) {
	paths := p.ExtractPaths(input)
	for _, path := range paths {
		if !p.IsAllowed(path) {
			return false, fmt.Sprintf("path %s is outside allowed directories", path)
		}
	}
	return true, ""
}
