package tools

import "github.com/TheLazyLemur/claudecord/internal/core"

// CheckPermission delegates entirely to perms. There is no interactive
// fall-back; tools either pass the path/scope check or are denied.
func CheckPermission(toolName string, input core.ToolInput, perms core.PermissionChecker) (bool, string) {
	return perms.Check(toolName, input)
}
