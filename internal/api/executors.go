package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
)

// executeToolByName dispatches to the appropriate tool executor
func executeToolByName(name string, input map[string]any, responder core.Responder, store skills.SkillStore, minimaxAPIKey string) (string, bool) {
	switch name {
	case "react_emoji":
		return executeReactEmoji(input, responder)
	case "send_update":
		return executeSendUpdate(input, responder)
	case "Read":
		return executeRead(input)
	case "Bash":
		return executeBash(input)
	case "Fetch":
		return executeFetch(input)
	case "Skill":
		return executeSkill(input, store)
	case "LoadSkillSupporting":
		return executeLoadSkillSupporting(input, store)
	case "WebSearch":
		return executeWebSearch(input, minimaxAPIKey)
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

func executeFetch(input map[string]any) (string, bool) {
	url, ok := input["url"].(string)
	if !ok || url == "" {
		return "missing url argument", true
	}

	method, _ := input["method"].(string)
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	var bodyReader io.Reader
	if body, ok := input["body"].(string); ok && body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return "error creating request: " + err.Error(), true
	}

	if headers, ok := input["headers"].(map[string]any); ok {
		for k, v := range headers {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "error making request: " + err.Error(), true
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "error reading response: " + err.Error(), true
	}

	const maxLen = 50000
	result := string(respBody)
	if len(result) > maxLen {
		result = result[:maxLen] + "\n... (truncated)"
	}

	return result, resp.StatusCode >= 400
}

func executeWebSearch(input map[string]any, apiKey string) (string, bool) {
	query, ok := input["query"].(string)
	if !ok || query == "" {
		return "missing query argument", true
	}

	if apiKey == "" {
		return "MINIMAX_API_KEY not configured", true
	}

	// Build request body
	reqBody := `{"q":"` + strings.ReplaceAll(query, `"`, `\"`) + `"}`

	req, err := http.NewRequest("POST", "https://api.minimax.io/v1/coding_plan/search", strings.NewReader(reqBody))
	if err != nil {
		return "error creating request: " + err.Error(), true
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "error making request: " + err.Error(), true
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "error reading response: " + err.Error(), true
	}

	if resp.StatusCode >= 400 {
		return "search failed: " + string(respBody), true
	}

	// Parse and format response
	var searchResp struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
			Date    string `json:"date"`
		} `json:"organic"`
		RelatedSearches []struct {
			Query string `json:"query"`
		} `json:"related_searches"`
	}

	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return "error parsing response: " + err.Error(), true
	}

	var result strings.Builder
	result.WriteString("Search results:\n\n")

	for i, r := range searchResp.Organic {
		result.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		result.WriteString("   " + r.Link + "\n")
		if r.Date != "" {
			result.WriteString("   Date: " + r.Date + "\n")
		}
		if r.Snippet != "" {
			result.WriteString("   " + r.Snippet + "\n")
		}
		result.WriteString("\n")
	}

	if len(searchResp.RelatedSearches) > 0 {
		var related []string
		for _, r := range searchResp.RelatedSearches {
			related = append(related, r.Query)
		}
		result.WriteString("Related searches: " + strings.Join(related, ", ") + "\n")
	}

	return result.String(), false
}
