package core

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockResponder struct {
	typingCalled       bool
	responses          []string
	reactions          []string
	updates            []string
	permissionPrompts  []string
	postErr            error
	reactionErr        error
	updateErr          error
	askPermissionAllow bool
	askPermissionErr   error
}

func (m *mockResponder) SendTyping() error {
	m.typingCalled = true
	return nil
}

func (m *mockResponder) PostResponse(content string) error {
	m.responses = append(m.responses, content)
	return m.postErr
}

func (m *mockResponder) AddReaction(emoji string) error {
	m.reactions = append(m.reactions, emoji)
	return m.reactionErr
}

func (m *mockResponder) SendUpdate(message string) error {
	m.updates = append(m.updates, message)
	return m.updateErr
}

func (m *mockResponder) AskPermission(prompt string) (bool, error) {
	m.permissionPrompts = append(m.permissionPrompts, prompt)
	return m.askPermissionAllow, m.askPermissionErr
}

type mockPermissionChecker struct {
	allowAll bool
	reason   string
	checks   []permCheck
}

type permCheck struct {
	toolName string
	input    map[string]any
}

func (m *mockPermissionChecker) Check(toolName string, input map[string]any) (bool, string) {
	m.checks = append(m.checks, permCheck{toolName, input})
	return m.allowAll, m.reason
}

type botMockBackend struct {
	sessionID       string
	closed          bool
	converseResp    string
	converseErr     error
	converseCalled  bool
	lastMsg         string
	lastResponder   Responder
	lastPerms       PermissionChecker
}

func (m *botMockBackend) Converse(ctx context.Context, msg string, responder Responder, perms PermissionChecker) (string, error) {
	m.converseCalled = true
	m.lastMsg = msg
	m.lastResponder = responder
	m.lastPerms = perms
	return m.converseResp, m.converseErr
}

func (m *botMockBackend) Close() error {
	m.closed = true
	return nil
}

func (m *botMockBackend) SessionID() string {
	return m.sessionID
}

type botMockFactory struct {
	backend *botMockBackend
	err     error
}

func (f *botMockFactory) Create(workDir string) (Backend, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.backend, nil
}

// --- Tests ---

func TestBot_HandleMessage_SendsTypingIndicator(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend := &botMockBackend{sessionID: "s1", converseResp: ""}
	factory := &botMockFactory{backend: backend}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// when
	err := bot.HandleMessage(responder, "hello")

	// then
	r.NoError(err)
	a.True(responder.typingCalled)
}

func TestBot_HandleMessage_CallsBackendConverse(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend := &botMockBackend{sessionID: "s1", converseResp: ""}
	factory := &botMockFactory{backend: backend}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// when
	err := bot.HandleMessage(responder, "test message")

	// then
	r.NoError(err)
	a.True(backend.converseCalled)
	a.Equal("test message", backend.lastMsg)
	a.Equal(responder, backend.lastResponder)
	a.Equal(perms, backend.lastPerms)
}

func TestBot_HandleMessage_PostsResponseFromBackend(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend := &botMockBackend{sessionID: "s1", converseResp: "Hello there!"}
	factory := &botMockFactory{backend: backend}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// when
	err := bot.HandleMessage(responder, "hi")

	// then
	r.NoError(err)
	r.Len(responder.responses, 1)
	a.Equal("Hello there!", responder.responses[0])
}

func TestBot_HandleMessage_NoResponseIfEmpty(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend := &botMockBackend{sessionID: "s1", converseResp: ""}
	factory := &botMockFactory{backend: backend}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// when
	err := bot.HandleMessage(responder, "hi")

	// then
	r.NoError(err)
	a.Len(responder.responses, 0)
}

func TestBot_HandleMessage_SessionError(t *testing.T) {
	// given
	factory := &botMockFactory{err: errors.New("spawn failed")}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// when
	err := bot.HandleMessage(responder, "hello")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spawn failed")
}

func TestBot_HandleMessage_ConverseError(t *testing.T) {
	// given
	backend := &botMockBackend{sessionID: "s1", converseErr: errors.New("converse failed")}
	factory := &botMockFactory{backend: backend}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// when
	err := bot.HandleMessage(responder, "hello")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "converse failed")
}

func TestBot_NewSession_StartsNewSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend1 := &botMockBackend{sessionID: "s1"}
	backend2 := &botMockBackend{sessionID: "s2"}
	factory := &botMockFactory{backend: backend1}
	perms := &mockPermissionChecker{}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// create initial session
	_ = bot.HandleMessage(responder, "init")
	factory.backend = backend2

	// when
	err := bot.NewSession("")

	// then
	r.NoError(err)
	a.True(backend1.closed)
}
