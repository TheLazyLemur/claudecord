package tools

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

func requireString(input map[string]any, key string) (string, string, bool) {
	val, ok := input[key].(string)
	if !ok || val == "" {
		return "", "missing " + key + " argument", true
	}
	return val, "", false
}

func truncateOutput(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n... (truncated)"
	}
	return s
}

// Deps holds all dependencies needed by tool executors.
type Deps struct {
	Responder     core.Responder
	SkillStore    skills.SkillStore
	MinimaxAPIKey string
}

// Execute dispatches to the appropriate tool executor. Returns (result, isError).
func Execute(name string, input map[string]any, deps Deps) (string, bool) {
	switch name {
	case "react_emoji":
		return executeReactEmoji(input, deps.Responder)
	case "send_update":
		return executeSendUpdate(input, deps.Responder)
	case "Read":
		return executeRead(input)
	case "Bash":
		return executeBash(input)
	case "Fetch":
		return executeFetch(input)
	case "Skill":
		return executeSkill(input, deps.SkillStore)
	case "LoadSkillSupporting":
		return executeLoadSkillSupporting(input, deps.SkillStore)
	case "WebSearch":
		return executeWebSearch(input, deps.MinimaxAPIKey)
	default:
		return "unknown tool: " + name, true
	}
}

func executeReactEmoji(input map[string]any, responder core.Responder) (string, bool) {
	emoji, errMsg, isErr := requireString(input, "emoji")
	if isErr {
		return errMsg, true
	}
	slog.Info("AddReaction", "emoji", emoji)
	if err := responder.AddReaction(emoji); err != nil {
		slog.Error("AddReaction failed", "error", err)
		return err.Error(), true
	}
	return "reaction added", false
}

func executeSendUpdate(input map[string]any, responder core.Responder) (string, bool) {
	msg, errMsg, isErr := requireString(input, "message")
	if isErr {
		return errMsg, true
	}
	if err := responder.SendUpdate(msg); err != nil {
		slog.Error("SendUpdate failed", "error", err)
		return err.Error(), true
	}
	return "update sent", false
}

func executeRead(input map[string]any) (string, bool) {
	filePath, errMsg, isErr := requireString(input, "file_path")
	if isErr {
		return errMsg, true
	}

	filePath = filepath.Clean(filePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "error reading file: " + err.Error(), true
	}

	return truncateOutput(string(content), 50000), false
}

func executeBash(input map[string]any) (string, bool) {
	command, errMsg, isErr := requireString(input, "command")
	if isErr {
		return errMsg, true
	}

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

	return truncateOutput(result.String(), 50000), false
}

func executeSkill(input map[string]any, store skills.SkillStore) (string, bool) {
	name, errMsg, isErr := requireString(input, "name")
	if isErr {
		return errMsg, true
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
	name, errMsg, isErr := requireString(input, "name")
	if isErr {
		return errMsg, true
	}
	path, errMsg, isErr := requireString(input, "path")
	if isErr {
		return errMsg, true
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
	url, errMsg, isErr := requireString(input, "url")
	if isErr {
		return errMsg, true
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

	return truncateOutput(string(respBody), 50000), resp.StatusCode >= 400
}

func executeWebSearch(input map[string]any, apiKey string) (string, bool) {
	query, errMsg, isErr := requireString(input, "query")
	if isErr {
		return errMsg, true
	}

	if apiKey == "" {
		return "MINIMAX_API_KEY not configured", true
	}

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
