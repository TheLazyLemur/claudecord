package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockResponder struct {
	reactions []string
	updates   []string

	reactionErr error
	updateErr   error
}

func (m *mockResponder) SendTyping() error              { return nil }
func (m *mockResponder) PostResponse(string) error      { return nil }
func (m *mockResponder) AddReaction(emoji string) error { m.reactions = append(m.reactions, emoji); return m.reactionErr }
func (m *mockResponder) SendUpdate(msg string) error    { m.updates = append(m.updates, msg); return m.updateErr }

type mockSkillStore struct {
	skills     map[string]string
	supporting map[string][]byte
}

func (m *mockSkillStore) List() ([]skills.SkillMetadata, error) { return nil, nil }

func (m *mockSkillStore) Load(name string) (*skills.Skill, error) {
	if inst, ok := m.skills[name]; ok {
		return &skills.Skill{Instructions: inst}, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockSkillStore) LoadSupporting(name, path string) ([]byte, error) {
	key := name + "/" + path
	if data, ok := m.supporting[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("not found")
}

func TestExecute_ReactEmoji(t *testing.T) {
	a := assert.New(t)
	r := &mockResponder{}

	result, isErr := Execute("react_emoji", core.ToolInput{Emoji: "👀"}, Deps{Responder: r})

	a.Equal("reaction added", result)
	a.False(isErr)
	a.Equal([]string{"👀"}, r.reactions)
}

func TestExecute_ReactEmoji_MissingArg(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("react_emoji", core.ToolInput{}, Deps{Responder: &mockResponder{}})

	a.Equal("missing emoji argument", result)
	a.True(isErr)
}

func TestExecute_SendUpdate(t *testing.T) {
	a := assert.New(t)
	r := &mockResponder{}

	result, isErr := Execute("send_update", core.ToolInput{Message: "working on it"}, Deps{Responder: r})

	a.Equal("update sent", result)
	a.False(isErr)
	a.Equal([]string{"working on it"}, r.updates)
}

func TestExecute_SendUpdate_MissingArg(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("send_update", core.ToolInput{}, Deps{Responder: &mockResponder{}})

	a.Equal("missing message argument", result)
	a.True(isErr)
}

func TestExecute_Skill(t *testing.T) {
	a := assert.New(t)
	store := &mockSkillStore{skills: map[string]string{"greet": "say hello"}}

	result, isErr := Execute("Skill", core.ToolInput{Name: "greet"}, Deps{SkillStore: store})

	a.Equal("say hello", result)
	a.False(isErr)
}

func TestExecute_Skill_NotFound(t *testing.T) {
	a := assert.New(t)
	store := &mockSkillStore{skills: map[string]string{}}

	result, isErr := Execute("Skill", core.ToolInput{Name: "missing"}, Deps{SkillStore: store})

	a.Equal("skill not found: missing", result)
	a.True(isErr)
}

func TestExecute_Skill_NilStore(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("Skill", core.ToolInput{Name: "x"}, Deps{})

	a.Equal("skill store not configured", result)
	a.True(isErr)
}

func TestExecute_LoadSkillSupporting(t *testing.T) {
	a := assert.New(t)
	store := &mockSkillStore{supporting: map[string][]byte{"greet/refs.md": []byte("ref content")}}

	result, isErr := Execute("LoadSkillSupporting", core.ToolInput{Name: "greet", Path: "refs.md"}, Deps{SkillStore: store})

	a.Equal("ref content", result)
	a.False(isErr)
}

func TestExecute_UnknownTool(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("bogus", core.ToolInput{}, Deps{})

	a.Equal("unknown tool: bogus", result)
	a.True(isErr)
}

func TestTruncateOutput(t *testing.T) {
	a := assert.New(t)

	long := strings.Repeat("x", 100)
	a.Equal(strings.Repeat("x", 10)+"\n... (truncated)", truncateOutput(long, 10))
}

func TestTruncateOutput_NoTruncation(t *testing.T) {
	a := assert.New(t)

	a.Equal("short", truncateOutput("short", 100))
	a.Equal("exact", truncateOutput("exact", 5))
}

// --- Read with image sentinel ---

// PNG signature (8 bytes) + minimal IHDR — enough for http.DetectContentType
// to recognize as image/png.
var pngBytes = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00,
}

func TestExecuteRead_ImageByExtension(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	dir := t.TempDir()
	for _, name := range []string{"a.jpg", "a.jpeg", "a.png", "a.webp", "a.gif"} {
		path := filepath.Join(dir, name)
		r.NoError(os.WriteFile(path, []byte("anything"), 0o644))

		out, isErr := executeRead(core.ToolInput{FilePath: path})
		a.False(isErr, name)
		a.True(strings.HasPrefix(out, ImageSentinel+"\t"), "name=%s out=%q", name, out)
	}
}

func TestExecuteRead_ImageBySniff(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "noext")
	r.NoError(os.WriteFile(path, pngBytes, 0o644))

	out, isErr := executeRead(core.ToolInput{FilePath: path})
	a.False(isErr)
	a.True(strings.HasPrefix(out, ImageSentinel+"\timage/png\t"))
}

func TestExecuteRead_TextUnchanged(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	r.NoError(os.WriteFile(path, []byte("hello"), 0o644))

	out, isErr := executeRead(core.ToolInput{FilePath: path})
	a.False(isErr)
	a.Equal("hello", out)
}

func TestExecuteRead_MissingFile(t *testing.T) {
	a := assert.New(t)
	out, isErr := executeRead(core.ToolInput{FilePath: "/nonexistent/missing"})
	a.True(isErr)
	a.Contains(out, "error reading file")
}

func TestExecuteBash_Timeout(t *testing.T) {
	a := assert.New(t)

	// given
	old := bashTimeout
	bashTimeout = 100 * time.Millisecond
	defer func() { bashTimeout = old }()

	// when
	result, isErr := executeBash(core.ToolInput{Command: "sleep 10"})

	// then
	a.True(isErr)
	a.Contains(result, "signal: killed")
}
