package core

import (
	"context"
	"testing"
	"time"

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
	converseFunc    func(ctx context.Context) (string, error)
	lastMsg         string
	lastResponder   Responder
	lastPerms       PermissionChecker
}

func (m *botMockBackend) Converse(ctx context.Context, msg string, responder Responder, perms PermissionChecker) (string, error) {
	m.converseCalled = true
	m.lastMsg = msg
	m.lastResponder = responder
	m.lastPerms = perms
	if m.converseFunc != nil {
		return m.converseFunc(ctx)
	}
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

func TestBot_NewSession_WaitsForInflightHandleMessage(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a backend that blocks in Converse until we signal
	converseStarted := make(chan struct{})
	converseUnblock := make(chan struct{})
	backend1 := &botMockBackend{sessionID: "s1"}
	backend1.converseFunc = func(ctx context.Context) (string, error) {
		close(converseStarted)
		<-converseUnblock
		return "done", nil
	}
	backend2 := &botMockBackend{sessionID: "s2"}
	factory := &botMockFactory{backend: backend1}
	perms := &mockPermissionChecker{}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// when
	// ... HandleMessage is in-flight (blocked in Converse)
	handleDone := make(chan error, 1)
	go func() {
		handleDone <- bot.HandleMessage(responder, "hello")
	}()
	<-converseStarted

	// ... NewSession is called concurrently
	factory.backend = backend2
	newSessionDone := make(chan error, 1)
	go func() {
		newSessionDone <- bot.NewSession("")
	}()

	// then
	// ... backend must NOT be closed while Converse is running
	select {
	case <-newSessionDone:
		t.Fatal("NewSession returned before HandleMessage finished")
	case <-time.After(50 * time.Millisecond):
		// expected: NewSession is blocked waiting for HandleMessage
	}
	a.False(backend1.closed, "backend closed while Converse in-flight")

	// ... unblock Converse, both should complete
	close(converseUnblock)
	r.NoError(<-handleDone)
	r.NoError(<-newSessionDone)
	a.True(backend1.closed)
}

func TestBot_HandleMessage_ConverseTimeout(t *testing.T) {
	a := assert.New(t)

	// given
	backend := &botMockBackend{sessionID: "s1"}
	backend.converseFunc = func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	factory := &botMockFactory{backend: backend}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	bot.converseTimeout = 50 * time.Millisecond
	responder := &mockResponder{}

	// when
	err := bot.HandleMessage(responder, "hello")

	// then
	a.Error(err)
	a.Contains(err.Error(), "context deadline exceeded")
}
