package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildChatTools_ExcludesReactEmojiWhenReactionsFalse(t *testing.T) {
	// given
	// ... reactions disabled

	// when
	// ... chat tools are built
	tools := buildChatTools(false)

	// then
	// ... react_emoji is absent
	for _, tool := range tools {
		if tool.OfTool != nil {
			assert.NotEqual(t, "react_emoji", tool.OfTool.Name)
		}
	}
}

func TestBuildChatTools_IncludesReactEmojiWhenReactionsTrue(t *testing.T) {
	// given
	// ... reactions enabled

	// when
	// ... chat tools are built
	apiTools := buildChatTools(true)

	// then
	// ... react_emoji is present
	found := false
	for _, tool := range apiTools {
		if tool.OfTool != nil && tool.OfTool.Name == "react_emoji" {
			found = true
			break
		}
	}
	assert.True(t, found, "expected react_emoji in tool list")
}

func TestBackendFactory_Create_IncludesReactEmojiWhenEnabled(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// given
	// ... a factory with EnableReactions set
	factory := &BackendFactory{
		APIKey:          "test",
		DefaultWorkDir:  t.TempDir(),
		EnableReactions: true,
	}

	// when
	// ... a backend is created
	backend, err := factory.Create("")
	r.NoError(err)

	// then
	// ... the backend's tool list contains react_emoji
	apiBackend, ok := backend.(*Backend)
	r.True(ok)
	found := false
	for _, tool := range apiBackend.tools {
		if tool.OfTool != nil && tool.OfTool.Name == "react_emoji" {
			found = true
			break
		}
	}
	a.True(found, "expected react_emoji in tool list when EnableReactions is true")
}

func TestBackendFactory_Create_ExcludesReactEmojiWhenDisabled(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// given
	// ... a factory with EnableReactions false (default)
	factory := &BackendFactory{
		APIKey:         "test",
		DefaultWorkDir: t.TempDir(),
	}

	// when
	// ... a backend is created
	backend, err := factory.Create("")
	r.NoError(err)

	// then
	// ... react_emoji is absent from the tool list
	apiBackend, ok := backend.(*Backend)
	r.True(ok)
	for _, tool := range apiBackend.tools {
		if tool.OfTool != nil {
			a.NotEqual("react_emoji", tool.OfTool.Name)
		}
	}
}

func TestBuildToolResultBlock_TextPath(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	block := buildToolResultBlock("call-1", "plain text result", false)

	r.NotNil(block.OfToolResult)
	tr := block.OfToolResult
	a.Equal("call-1", tr.ToolUseID)
	r.Len(tr.Content, 1)
	a.NotNil(tr.Content[0].OfText)
	a.Nil(tr.Content[0].OfImage)
	a.Equal("plain text result", tr.Content[0].OfText.Text)
}

func TestBuildToolResultBlock_ImageSentinelTurnsIntoImageBlock(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	sentinel := tools.ImageSentinel + "\timage/png\tBASE64DATA"
	block := buildToolResultBlock("call-2", sentinel, false)

	r.NotNil(block.OfToolResult)
	tr := block.OfToolResult
	a.Equal("call-2", tr.ToolUseID)
	r.Len(tr.Content, 1)
	r.NotNil(tr.Content[0].OfImage)
	a.Nil(tr.Content[0].OfText)

	src := tr.Content[0].OfImage.Source.OfBase64
	r.NotNil(src)
	a.Equal("BASE64DATA", src.Data)
	a.Equal(anthropic.Base64ImageSourceMediaType("image/png"), src.MediaType)
}

func TestBuildToolResultBlock_MalformedSentinelFallsBackToText(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// Missing third field; SplitN gives < 3 parts so we fall through to text.
	bad := tools.ImageSentinel + "\timage/png"
	block := buildToolResultBlock("call-3", bad, false)

	r.NotNil(block.OfToolResult)
	r.Len(block.OfToolResult.Content, 1)
	a.NotNil(block.OfToolResult.Content[0].OfText)
}

func TestBuildToolResultBlock_ErrorIsPlainText(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// Even if the result happens to start with the sentinel, an error result
	// is reported as text (so the model sees the error verbatim).
	block := buildToolResultBlock("call-4", tools.ImageSentinel+"\timage/png\tdata", true)

	r.NotNil(block.OfToolResult)
	r.Len(block.OfToolResult.Content, 1)
	a.NotNil(block.OfToolResult.Content[0].OfText)
	a.True(block.OfToolResult.IsError.Valid())
	a.True(block.OfToolResult.IsError.Value)
}

func TestEffectiveSystemPrompt_NoAgentsFile(t *testing.T) {
	dir := t.TempDir()
	b := &Backend{systemPrompt: "BASE", workDir: dir}
	if got := b.effectiveSystemPrompt(); got != "BASE" {
		t.Fatalf("expected BASE, got %q", got)
	}
}

func TestEffectiveSystemPrompt_AgentsFileMerged(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("RULES"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &Backend{systemPrompt: "BASE", workDir: dir}
	got := b.effectiveSystemPrompt()
	if got == "BASE" {
		t.Fatalf("expected merged output, got base unchanged")
	}
	if !strings.Contains(got, "BASE") || !strings.Contains(got, "RULES") || !strings.Contains(got, "<agents_md>") {
		t.Fatalf("expected merged output to contain base, agents body, and tag, got %q", got)
	}
}

func TestEffectiveSystemPrompt_AgentsFileRefreshedPerCall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, []byte("V1"), 0o644); err != nil {
		t.Fatal(err)
	}
	b := &Backend{systemPrompt: "BASE", workDir: dir}
	first := b.effectiveSystemPrompt()
	if !strings.Contains(first, "V1") {
		t.Fatalf("expected V1 in first call, got %q", first)
	}
	if err := os.WriteFile(path, []byte("V2"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := b.effectiveSystemPrompt()
	if !strings.Contains(second, "V2") || strings.Contains(second, "V1") {
		t.Fatalf("expected V2 (not V1) in second call, got %q", second)
	}
}

func TestBuildParams_NoThinkingByDefault(t *testing.T) {
	// given
	// ... a backend with thinkingBudget unset
	b := &Backend{model: "kimi-for-coding"}

	// when
	// ... params are built
	params := b.buildParams()

	// then
	// ... no thinking config is attached
	a := assert.New(t)
	a.Nil(params.Thinking.OfEnabled)
	a.Nil(params.Thinking.OfDisabled)
}

func TestBuildParams_ThinkingEnabledWhenBudgetSet(t *testing.T) {
	// given
	// ... a backend with a positive thinking budget
	b := &Backend{model: "kimi-for-coding", thinkingBudget: 4096}

	// when
	// ... params are built
	params := b.buildParams()

	// then
	// ... params.Thinking carries the enabled config with that budget
	r := require.New(t)
	a := assert.New(t)
	r.NotNil(params.Thinking.OfEnabled)
	a.Equal(int64(4096), params.Thinking.OfEnabled.BudgetTokens)
}

func TestBackendFactory_Create_PropagatesThinkingBudget(t *testing.T) {
	// given
	// ... a factory configured with a thinking budget
	r := require.New(t)
	a := assert.New(t)
	factory := &BackendFactory{
		APIKey:               "test",
		DefaultWorkDir:       t.TempDir(),
		ThinkingBudgetTokens: 4096,
	}

	// when
	// ... a backend is created
	backend, err := factory.Create("")
	r.NoError(err)

	// then
	// ... the backend carries the same budget
	apiBackend, ok := backend.(*Backend)
	r.True(ok)
	a.Equal(4096, apiBackend.thinkingBudget)
}

func TestBackendFactory_Create_FallsBackToDefaultWorkDirWhenEmpty(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// given
	// ... a default work dir containing an AGENTS.md file
	defaultDir := t.TempDir()
	r.NoError(os.WriteFile(filepath.Join(defaultDir, "AGENTS.md"), []byte("RULES"), 0o644))
	factory := &BackendFactory{
		APIKey:         "test",
		DefaultWorkDir: defaultDir,
	}

	// when
	// ... a backend is created with an empty work dir
	backend, err := factory.Create("")
	r.NoError(err)

	// then
	// ... the backend uses DefaultWorkDir and AGENTS.md is loaded into the system prompt
	apiBackend, ok := backend.(*Backend)
	r.True(ok)
	a.Equal(defaultDir, apiBackend.workDir)
	a.Contains(apiBackend.effectiveSystemPrompt(), "RULES")
}

func TestBackend_Claim_FirstCallerOwnsLoop(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	b := &Backend{}

	owned := b.claim("hello")

	a.True(owned)
	a.True(b.running)
	r.Len(b.history, 1)
	a.Empty(b.mailbox)
}

func TestBackend_Claim_SecondCallerEnqueues(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	b := &Backend{}
	r.True(b.claim("first"))

	owned := b.claim("steered")

	a.False(owned)
	a.True(b.running)
	r.Len(b.history, 1)
	r.Len(b.mailbox, 1)
	a.Equal("steered", b.mailbox[0])
}

func TestBackend_Claim_AfterReleaseWithQueuedMessagesIncludesThemInNewTurn(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	b := &Backend{}
	r.True(b.claim("first"))
	r.False(b.claim("queued-during-error"))
	b.release()

	owned := b.claim("fresh")

	a.True(owned)
	a.True(b.running)
	a.Empty(b.mailbox)
	r.Len(b.history, 2)

	last := b.history[1]
	r.Len(last.Content, 2)
	a.Contains(last.Content[1].OfText.Text, "<user_steering>queued-during-error</user_steering>")
}

func TestBackend_DrainMailbox_ReturnsAndClears(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	b := &Backend{}
	r.True(b.claim("first"))
	r.False(b.claim("a"))
	r.False(b.claim("b"))

	got := b.drainMailbox()

	a.Equal([]string{"a", "b"}, got)
	a.Empty(b.mailbox)
	a.True(b.running)
}

func TestBackend_FinishOrContinue_EmptyMailboxFlipsRunningFalse(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	b := &Backend{}
	r.True(b.claim("first"))

	got := b.finishOrContinue()

	a.Nil(got)
	a.False(b.running)
}

func TestBackend_FinishOrContinue_NonEmptyMailboxKeepsRunning(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	b := &Backend{}
	r.True(b.claim("first"))
	r.False(b.claim("steered"))

	got := b.finishOrContinue()

	a.Equal([]string{"steered"}, got)
	a.True(b.running)
	a.Empty(b.mailbox)
}

func TestSteeringText_WrapsInTag(t *testing.T) {
	assert.Equal(t, "<user_steering>hello</user_steering>", steeringText("hello"))
}

func TestBackend_Claim_DropsWhenMailboxAtCap(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	b := &Backend{}
	r.True(b.claim("first"))
	for i := 0; i < maxMailbox; i++ {
		r.False(b.claim("queued"))
	}
	r.Len(b.mailbox, maxMailbox)

	owned := b.claim("dropped")

	a.False(owned)
	a.Len(b.mailbox, maxMailbox)
}

type stubResponder struct{}

func (stubResponder) SendTyping() error         { return nil }
func (stubResponder) PostResponse(string) error { return nil }
func (stubResponder) AddReaction(string) error  { return nil }
func (stubResponder) SendUpdate(string) error   { return nil }

type allowAllPerms struct{}

func (allowAllPerms) Check(string, core.ToolInput) (bool, string) { return true, "" }

func captureRequestBody(r *http.Request) string {
	body, _ := io.ReadAll(r.Body)
	r.Body.Close()
	return string(body)
}

func TestBackend_Converse_QueuedMessageContinuesLoopAtNaturalEnd(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	firstRequestStarted := make(chan struct{})
	releaseFirstRequest := make(chan struct{})
	var secondRequestBody string
	var bodyMu sync.Mutex

	var requestCount int
	var reqMu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqMu.Lock()
		requestCount++
		n := requestCount
		reqMu.Unlock()

		switch n {
		case 1:
			close(firstRequestStarted)
			<-releaseFirstRequest
			writeMessageJSON(w, "msg_1", "first reply", "end_turn")
		case 2:
			body := captureRequestBody(req)
			bodyMu.Lock()
			secondRequestBody = body
			bodyMu.Unlock()
			writeMessageJSON(w, "msg_2", "second reply", "end_turn")
		default:
			http.Error(w, "unexpected extra request", http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	client := anthropic.NewClient(
		option.WithAPIKey("test"),
		option.WithBaseURL(server.URL),
	)
	b := &Backend{
		client:    client,
		model:     "test-model",
		sessionID: "test",
		history:   []anthropic.MessageParam{},
	}

	type result struct {
		resp string
		err  error
	}
	first := make(chan result, 1)
	go func() {
		resp, err := b.Converse(context.Background(), "hello", stubResponder{}, allowAllPerms{})
		first <- result{resp, err}
	}()

	<-firstRequestStarted

	steerDone := make(chan result, 1)
	go func() {
		resp, err := b.Converse(context.Background(), "steer me", stubResponder{}, allowAllPerms{})
		steerDone <- result{resp, err}
	}()

	select {
	case got := <-steerDone:
		r.NoError(got.err)
		a.Equal("", got.resp)
	case <-time.After(2 * time.Second):
		t.Fatal("steered Converse blocked instead of enqueueing")
	}

	close(releaseFirstRequest)

	select {
	case got := <-first:
		r.NoError(got.err)
		a.Contains(got.resp, "first reply")
		a.Contains(got.resp, "second reply")
	case <-time.After(5 * time.Second):
		t.Fatal("loop did not finish")
	}

	bodyMu.Lock()
	body := secondRequestBody
	bodyMu.Unlock()

	// The SDK's JSON encoder HTML-escapes angle brackets, so substring-match
	// against decoded text rather than the raw body.
	var decoded struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	r.NoError(json.Unmarshal([]byte(body), &decoded))

	var found bool
	for _, msg := range decoded.Messages {
		if msg.Role != "user" {
			continue
		}
		for _, c := range msg.Content {
			if c.Type == "text" && strings.Contains(c.Text, "<user_steering>steer me</user_steering>") {
				found = true
			}
		}
	}
	a.True(found, "body=%s", body)
}

func writeMessageJSON(w http.ResponseWriter, id, text, stopReason string) {
	payload := map[string]any{
		"id":   id,
		"type": "message",
		"role": "assistant",
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
		"model":       "test-model",
		"stop_reason": stopReason,
		"usage": map[string]any{
			"input_tokens":  1,
			"output_tokens": 1,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
