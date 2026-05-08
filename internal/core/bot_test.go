package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockResponder struct {
	typingCalled bool
	responses    []string
	reactions    []string
	updates      []string
	postErr      error
	reactionErr  error
	updateErr    error
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

type mockPermissionChecker struct {
	allowAll bool
	reason   string
	checks   []permCheck
}

type permCheck struct {
	toolName string
	input    ToolInput
}

func (m *mockPermissionChecker) Check(toolName string, input ToolInput) (bool, string) {
	m.checks = append(m.checks, permCheck{toolName, input})
	return m.allowAll, m.reason
}

type botMockBackend struct {
	sessionID      string
	closed         bool
	converseResp   string
	converseErr    error
	converseCalled bool
	converseFunc   func(ctx context.Context) (string, error)
	lastMsg        string
	lastResponder  Outbound
	lastPerms      PermissionChecker
}

func (m *botMockBackend) Converse(ctx context.Context, msg string, responder Outbound, perms PermissionChecker) (string, error) {
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

func TestBot_NewSession_StartsNewSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend1 := &botMockBackend{sessionID: "s1"}
	backend2 := &botMockBackend{sessionID: "s2"}
	factory := &botMockFactory{backend: backend1}
	perms := &mockPermissionChecker{}
	bot := NewBot(NewSessionManager(factory, nil), perms)
	responder := &mockResponder{}

	// create initial session
	_ = bot.HandleInbound(Inbound{SessionKey: "k1", Text: "init", Reply: responder})
	factory.backend = backend2

	// when
	err := bot.NewSession("")

	// then
	r.NoError(err)
	a.True(backend1.closed)
}

func TestBot_NewSession_WaitsForInflightHandleInbound(t *testing.T) {
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
	bot := NewBot(NewSessionManager(factory, nil), perms)
	responder := &mockResponder{}

	// when
	// ... HandleInbound is in-flight (blocked in Converse)
	handleDone := make(chan error, 1)
	go func() {
		handleDone <- bot.HandleInbound(Inbound{SessionKey: "k1", Text: "hello", Reply: responder})
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
		t.Fatal("NewSession returned before HandleInbound finished")
	case <-time.After(50 * time.Millisecond):
		// expected: NewSession is blocked waiting for HandleInbound
	}
	a.False(backend1.closed, "backend closed while Converse in-flight")

	// ... unblock Converse, both should complete
	close(converseUnblock)
	r.NoError(<-handleDone)
	r.NoError(<-newSessionDone)
	a.True(backend1.closed)
}
