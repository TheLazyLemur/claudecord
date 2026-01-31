package api

import (
	"bytes"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
)

// executeToolByName dispatches to the appropriate tool executor
func executeToolByName(name string, input map[string]any, responder core.Responder, store skills.SkillStore) (string, bool) {
	switch name {
	case "react_emoji":
		return executeReactEmoji(input, responder)
	case "send_update":
		return executeSendUpdate(input, responder)
	case "Read":
		return executeRead(input)
	case "Bash":
		return executeBash(input)
	case "Skill":
		return executeSkill(input, store)
	case "LoadSkillSupporting":
		return executeLoadSkillSupporting(input, store)
	default:
		return "unknown tool: " + name, true
	}
}

func executeReactEmoji(input map[string]any, responder core.Responder) (string, bool) {
	emoji, ok := input["emoji"].(string)
	if !ok || emoji == "" {
		return "missing emoji argument", true
	}
	slog.Info("AddReaction", "emoji", emoji)
	if err := responder.AddReaction(emoji); err != nil {
		slog.Error("AddReaction failed", "error", err)
		return err.Error(), true
	}
	return "reaction added", false
}

func executeSendUpdate(input map[string]any, responder core.Responder) (string, bool) {
	msg, ok := input["message"].(string)
	if !ok || msg == "" {
		return "missing message argument", true
	}
	if err := responder.SendUpdate(msg); err != nil {
		slog.Error("SendUpdate failed", "error", err)
		return err.Error(), true
	}
	return "update sent", false
}

func executeRead(input map[string]any) (string, bool) {
	filePath, ok := input["file_path"].(string)
	if !ok || filePath == "" {
		return "missing file_path argument", true
	}

	// Clean the path
	filePath = filepath.Clean(filePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "error reading file: " + err.Error(), true
	}

	// Truncate if too long
	const maxLen = 50000
	if len(content) > maxLen {
		content = content[:maxLen]
		return string(content) + "\n... (truncated)", false
	}

	return string(content), false
}

func executeBash(input map[string]any) (string, bool) {
	command, ok := input["command"].(string)
	if !ok || command == "" {
		return "missing command argument", true
	}

	// Execute via sh -c
	cmd := exec.Command("sh", "-c", command)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}
	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("stderr: ")
		result.WriteString(stderr.String())
	}

	if err != nil {
		if result.Len() > 0 {
			result.WriteString("\n")
		}
		result.WriteString("exit error: ")
		result.WriteString(err.Error())
		return result.String(), true
	}

	// Truncate if too long
	output := result.String()
	const maxLen = 50000
	if len(output) > maxLen {
		output = output[:maxLen] + "\n... (truncated)"
	}

	return output, false
}

func executeSkill(input map[string]any, store skills.SkillStore) (string, bool) {
	name, ok := input["name"].(string)
	if !ok || name == "" {
		return "missing name argument", true
	}

	if store == nil {
		return "skill store not configured", true
	}

	skill, err := store.Load(name)
	if err != nil {
		return "skill not found: " + name, true
	}

	return skill.Instructions, false
}

func executeLoadSkillSupporting(input map[string]any, store skills.SkillStore) (string, bool) {
	name, ok := input["name"].(string)
	if !ok || name == "" {
		return "missing name argument", true
	}

	path, ok := input["path"].(string)
	if !ok || path == "" {
		return "missing path argument", true
	}

	if store == nil {
		return "skill store not configured", true
	}

	content, err := store.LoadSupporting(name, path)
	if err != nil {
		return "error loading supporting file: " + err.Error(), true
	}

	return string(content), false
}
