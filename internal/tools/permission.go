package tools

import (
	"log/slog"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

// CheckPermission runs the auto-check, falls back to asking user.
func CheckPermission(toolName string, input map[string]any, perms core.PermissionChecker, responder core.Responder) (bool, string) {
	allow, reason := perms.Check(toolName, input)
	if allow {
		return true, ""
	}

	prompt := FormatPermissionPrompt(toolName, input)
	userApproved, err := responder.AskPermission(prompt)
	if err != nil {
		slog.Warn("asking permission", "error", err)
	}
	if userApproved {
		return true, ""
	}
	return false, reason
}

// FormatPermissionPrompt builds a human-readable permission prompt for a tool call.
func FormatPermissionPrompt(toolName string, input map[string]any) string {
	prompt := "Allow **" + toolName + "**?"
	if cmd, ok := input["command"].(string); ok {
		if len(cmd) > 100 {
			cmd = cmd[:100] + "..."
		}
		prompt += "\n`" + cmd + "`"
	}
	if path, ok := input["file_path"].(string); ok {
		prompt += "\n`" + path + "`"
	}
	return prompt
}
