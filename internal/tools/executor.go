package tools

import (
	"bytes"
	"context"
	"encoding/base64"
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

const maxOutputLen = 50000

// ImageSentinel marks a Read result as an image payload that the API backend
// should turn into an anthropic.ImageBlockParam tool_result. Format:
//
//	__CLAUDECORD_IMAGE__\t<mime>\t<base64>
//
// Tab-delimited so the second field can't collide with arbitrary MIME chars.
const ImageSentinel = "__CLAUDECORD_IMAGE__"

var httpClient = &http.Client{Timeout: 30 * time.Second}
var bashTimeout = 2 * time.Minute
var webSearchEndpoint = "https://api.search.brave.com/res/v1/web/search"

func truncateOutput(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen] + "\n... (truncated)"
	}
	return s
}

// Deps holds all dependencies needed by tool executors.
type Deps struct {
	Outbound        core.Outbound
	SkillStore      skills.SkillStore
	WebSearchAPIKey string
}

// Execute dispatches to the appropriate tool executor. Returns (result, isError).
func Execute(name string, input core.ToolInput, deps Deps) (string, bool) {
	switch name {
	case "react_emoji":
		return executeReactEmoji(input, deps.Outbound)
	case "send_update":
		return executeSendUpdate(input, deps.Outbound)
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
		return executeWebSearch(input, deps.WebSearchAPIKey)
	default:
		return "unknown tool: " + name, true
	}
}

func executeReactEmoji(input core.ToolInput, responder core.Outbound) (string, bool) {
	if input.Emoji == "" {
		return "missing emoji argument", true
	}
	slog.Info("AddReaction", "emoji", input.Emoji)
	if err := responder.AddReaction(input.Emoji); err != nil {
		slog.Error("AddReaction failed", "error", err)
		return err.Error(), true
	}
	return "reaction added", false
}

func executeSendUpdate(input core.ToolInput, responder core.Outbound) (string, bool) {
	if input.Message == "" {
		return "missing message argument", true
	}
	if err := responder.SendUpdate(input.Message); err != nil {
		slog.Error("SendUpdate failed", "error", err)
		return err.Error(), true
	}
	return "update sent", false
}

func executeRead(input core.ToolInput) (string, bool) {
	if input.FilePath == "" {
		return "missing file_path argument", true
	}

	filePath := filepath.Clean(input.FilePath)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return "error reading file: " + err.Error(), true
	}

	if mime := detectImageMIME(filePath, content); mime != "" {
		return ImageSentinel + "\t" + mime + "\t" + base64.StdEncoding.EncodeToString(content), false
	}

	return truncateOutput(string(content), maxOutputLen), false
}

// detectImageMIME returns a normalized image MIME ("image/png" etc.) when the
// file looks like one of the supported image types — extension first, then a
// content sniff. Returns "" for non-images so callers fall through to text.
func detectImageMIME(path string, content []byte) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	}
	sniff := http.DetectContentType(content)
	switch {
	case strings.HasPrefix(sniff, "image/jpeg"):
		return "image/jpeg"
	case strings.HasPrefix(sniff, "image/png"):
		return "image/png"
	case strings.HasPrefix(sniff, "image/webp"):
		return "image/webp"
	case strings.HasPrefix(sniff, "image/gif"):
		return "image/gif"
	}
	return ""
}

func executeBash(input core.ToolInput) (string, bool) {
	if input.Command == "" {
		return "missing command argument", true
	}

	ctx, cancel := context.WithTimeout(context.Background(), bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", input.Command)
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

	return truncateOutput(result.String(), maxOutputLen), false
}

func executeSkill(input core.ToolInput, store skills.SkillStore) (string, bool) {
	if input.Name == "" {
		return "missing name argument", true
	}

	if store == nil {
		return "skill store not configured", true
	}

	skill, err := store.Load(input.Name)
	if err != nil {
		return "skill not found: " + input.Name, true
	}

	return skill.Instructions, false
}

func executeLoadSkillSupporting(input core.ToolInput, store skills.SkillStore) (string, bool) {
	if input.Name == "" {
		return "missing name argument", true
	}
	if input.Path == "" {
		return "missing path argument", true
	}

	if store == nil {
		return "skill store not configured", true
	}

	content, err := store.LoadSupporting(input.Name, input.Path)
	if err != nil {
		return "error loading supporting file: " + err.Error(), true
	}

	return string(content), false
}

func executeFetch(input core.ToolInput) (string, bool) {
	if input.URL == "" {
		return "missing url argument", true
	}

	method := input.Method
	if method == "" {
		method = "GET"
	}
	method = strings.ToUpper(method)

	var bodyReader io.Reader
	if input.Body != "" {
		bodyReader = strings.NewReader(input.Body)
	}

	req, err := http.NewRequest(method, input.URL, bodyReader)
	if err != nil {
		return "error creating request: " + err.Error(), true
	}

	for k, v := range input.Headers {
		req.Header.Set(k, v)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "error making request: " + err.Error(), true
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "error reading response: " + err.Error(), true
	}

	return truncateOutput(string(respBody), maxOutputLen), resp.StatusCode >= 400
}

func executeWebSearch(input core.ToolInput, apiKey string) (string, bool) {
	if input.Query == "" {
		return "missing query argument", true
	}

	if apiKey == "" {
		return "WEB_SEARCH_API_KEY not configured", true
	}

	req, err := http.NewRequest(http.MethodGet, webSearchEndpoint, nil)
	if err != nil {
		return "error creating request: " + err.Error(), true
	}
	q := req.URL.Query()
	q.Set("q", input.Query)
	req.URL.RawQuery = q.Encode()

	req.Header.Set("X-Subscription-Token", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
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
		Web struct {
			Results []struct {
				Title         string   `json:"title"`
				URL           string   `json:"url"`
				Description   string   `json:"description"`
				ExtraSnippets []string `json:"extra_snippets"`
			} `json:"results"`
		} `json:"web"`
	}

	if err := json.Unmarshal(respBody, &searchResp); err != nil {
		return "error parsing response: " + err.Error(), true
	}

	if len(searchResp.Web.Results) == 0 {
		return "No results.", false
	}

	var result strings.Builder
	result.WriteString("Search results:\n\n")

	for i, r := range searchResp.Web.Results {
		result.WriteString(fmt.Sprintf("%d. %s\n", i+1, r.Title))
		result.WriteString("   " + r.URL + "\n")
		if r.Description != "" {
			result.WriteString("   " + r.Description + "\n")
		}
		result.WriteString("\n")
	}

	return result.String(), false
}
