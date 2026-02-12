package tools

import (
	"fmt"
	"strings"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/stretchr/testify/assert"
)

type mockResponder struct {
	reactions []string
	updates   []string

	reactionErr error
	updateErr   error
}

func (m *mockResponder) SendTyping() error                   { return nil }
func (m *mockResponder) PostResponse(string) error           { return nil }
func (m *mockResponder) AskPermission(string) (bool, error)  { return false, nil }
func (m *mockResponder) AddReaction(emoji string) error      { m.reactions = append(m.reactions, emoji); return m.reactionErr }
func (m *mockResponder) SendUpdate(msg string) error         { m.updates = append(m.updates, msg); return m.updateErr }

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

	result, isErr := Execute("react_emoji", map[string]any{"emoji": "👀"}, Deps{Responder: r})

	a.Equal("reaction added", result)
	a.False(isErr)
	a.Equal([]string{"👀"}, r.reactions)
}

func TestExecute_ReactEmoji_MissingArg(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("react_emoji", map[string]any{}, Deps{Responder: &mockResponder{}})

	a.Equal("missing emoji argument", result)
	a.True(isErr)
}

func TestExecute_SendUpdate(t *testing.T) {
	a := assert.New(t)
	r := &mockResponder{}

	result, isErr := Execute("send_update", map[string]any{"message": "working on it"}, Deps{Responder: r})

	a.Equal("update sent", result)
	a.False(isErr)
	a.Equal([]string{"working on it"}, r.updates)
}

func TestExecute_SendUpdate_MissingArg(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("send_update", map[string]any{}, Deps{Responder: &mockResponder{}})

	a.Equal("missing message argument", result)
	a.True(isErr)
}

func TestExecute_Skill(t *testing.T) {
	a := assert.New(t)
	store := &mockSkillStore{skills: map[string]string{"greet": "say hello"}}

	result, isErr := Execute("Skill", map[string]any{"name": "greet"}, Deps{SkillStore: store})

	a.Equal("say hello", result)
	a.False(isErr)
}

func TestExecute_Skill_NotFound(t *testing.T) {
	a := assert.New(t)
	store := &mockSkillStore{skills: map[string]string{}}

	result, isErr := Execute("Skill", map[string]any{"name": "missing"}, Deps{SkillStore: store})

	a.Equal("skill not found: missing", result)
	a.True(isErr)
}

func TestExecute_Skill_NilStore(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("Skill", map[string]any{"name": "x"}, Deps{})

	a.Equal("skill store not configured", result)
	a.True(isErr)
}

func TestExecute_LoadSkillSupporting(t *testing.T) {
	a := assert.New(t)
	store := &mockSkillStore{supporting: map[string][]byte{"greet/refs.md": []byte("ref content")}}

	result, isErr := Execute("LoadSkillSupporting", map[string]any{"name": "greet", "path": "refs.md"}, Deps{SkillStore: store})

	a.Equal("ref content", result)
	a.False(isErr)
}

func TestExecute_UnknownTool(t *testing.T) {
	a := assert.New(t)

	result, isErr := Execute("bogus", map[string]any{}, Deps{})

	a.Equal("unknown tool: bogus", result)
	a.True(isErr)
}

func TestRequireString(t *testing.T) {
	a := assert.New(t)

	val, errMsg, isErr := requireString(map[string]any{"key": "hello"}, "key")
	a.Equal("hello", val)
	a.Empty(errMsg)
	a.False(isErr)
}

func TestRequireString_Missing(t *testing.T) {
	a := assert.New(t)

	// missing key
	val, errMsg, isErr := requireString(map[string]any{}, "key")
	a.Empty(val)
	a.Equal("missing key argument", errMsg)
	a.True(isErr)

	// empty string
	val, errMsg, isErr = requireString(map[string]any{"key": ""}, "key")
	a.Empty(val)
	a.Equal("missing key argument", errMsg)
	a.True(isErr)

	// wrong type
	val, errMsg, isErr = requireString(map[string]any{"key": 123}, "key")
	a.Empty(val)
	a.Equal("missing key argument", errMsg)
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

