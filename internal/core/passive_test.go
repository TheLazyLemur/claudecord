package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDiscordClient struct {
	sentMessages   []sentMessage
	createdThreads []createdThread
	startedThreads []startedThread
	typingChannels []string
	addedReactions []addedReaction
	sendErr        error
	threadErr      error
	reactionErr    error
	threadID       string
}

type startedThread struct {
	channelID string
	messageID string
	name      string
}

type sentMessage struct {
	channelID string
	content   string
}

type createdThread struct {
	channelID string
	content   string
}

type addedReaction struct {
	channelID string
	messageID string
	emoji     string
}

func (m *mockDiscordClient) SendMessage(channelID, content string) error {
	m.sentMessages = append(m.sentMessages, sentMessage{channelID, content})
	return m.sendErr
}

func (m *mockDiscordClient) CreateThread(channelID, content string) (string, error) {
	m.createdThreads = append(m.createdThreads, createdThread{channelID, content})
	return m.threadID, m.threadErr
}

func (m *mockDiscordClient) SendTyping(channelID string) error {
	m.typingChannels = append(m.typingChannels, channelID)
	return nil
}

func (m *mockDiscordClient) AddReaction(channelID, messageID, emoji string) error {
	m.addedReactions = append(m.addedReactions, addedReaction{channelID, messageID, emoji})
	return m.reactionErr
}

func (m *mockDiscordClient) StartThread(channelID, messageID, name string) (string, error) {
	m.startedThreads = append(m.startedThreads, startedThread{channelID, messageID, name})
	return m.threadID, m.threadErr
}

func (m *mockDiscordClient) SendMessageReturningID(channelID, content string) (string, error) {
	m.sentMessages = append(m.sentMessages, sentMessage{channelID, content})
	return "mock-msg-id", m.sendErr
}

func (m *mockDiscordClient) WaitForReaction(channelID, messageID string, emojis []string, userID string) (string, error) {
	return "", nil
}

type passiveMockBackend struct {
	sessionID    string
	closed       bool
	converseResp string
	converseErr  error
	converseFunc func(ctx context.Context) (string, error)
	lastMsg      string
}

func (m *passiveMockBackend) Converse(ctx context.Context, msg string, responder Responder, perms PermissionChecker) (string, error) {
	m.lastMsg = msg
	if m.converseFunc != nil {
		return m.converseFunc(ctx)
	}
	return m.converseResp, m.converseErr
}

func (m *passiveMockBackend) Close() error {
	m.closed = true
	return nil
}

func (m *passiveMockBackend) SessionID() string {
	return m.sessionID
}

type passiveMockFactory struct {
	backend *passiveMockBackend
	err     error
}

func (f *passiveMockFactory) Create(workDir string) (Backend, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.backend, nil
}

func TestPassiveBot_HandleBufferedMessages_NoResponseIfEmpty(t *testing.T) {
	a := assert.New(t)

	// given
	backend := &passiveMockBackend{sessionID: "p1", converseResp: ""}
	factory := &passiveMockFactory{backend: backend}
	discord := &mockDiscordClient{}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// when
	err := bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "random chat", AuthorID: "u1"},
	})

	// then
	require.NoError(t, err)
	a.Len(discord.sentMessages, 0)
	a.Len(discord.startedThreads, 0)
}

func TestPassiveBot_HandleBufferedMessages_NoResponseMarker(t *testing.T) {
	a := assert.New(t)

	// given
	backend := &passiveMockBackend{sessionID: "p1", converseResp: "[NO_RESPONSE]"}
	factory := &passiveMockFactory{backend: backend}
	discord := &mockDiscordClient{}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// when
	err := bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "random chat", AuthorID: "u1"},
	})

	// then
	require.NoError(t, err)
	a.Len(discord.sentMessages, 0)
	a.Len(discord.startedThreads, 0)
}

func TestPassiveBot_HandleBufferedMessages_RespondsInThread(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend := &passiveMockBackend{sessionID: "p1", converseResp: "Here's how to do that in Go..."}
	factory := &passiveMockFactory{backend: backend}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// when
	err := bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "how do I parse JSON in Go?", AuthorID: "u1"},
	})

	// then
	r.NoError(err)
	r.Len(discord.startedThreads, 1)
	a.Equal("chan-1", discord.startedThreads[0].channelID)
	a.Equal("m1", discord.startedThreads[0].messageID)
	r.Len(discord.sentMessages, 1)
	a.Equal("thread-1", discord.sentMessages[0].channelID)
	a.Equal("Here's how to do that in Go...", discord.sentMessages[0].content)
}

func TestPassiveBot_HandleBufferedMessages_CombinesMultipleMessages(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend := &passiveMockBackend{sessionID: "p1", converseResp: "Answer"}
	factory := &passiveMockFactory{backend: backend}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// when
	err := bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "first question", AuthorID: "u1"},
		{ChannelID: "chan-1", MessageID: "m2", Content: "more context", AuthorID: "u1"},
	})

	// then
	r.NoError(err)
	a.Contains(backend.lastMsg, "first question")
	a.Contains(backend.lastMsg, "more context")
}

func TestPassiveBot_NewSession_ResetsSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend1 := &passiveMockBackend{sessionID: "p1", converseResp: ""}
	backend2 := &passiveMockBackend{sessionID: "p2", converseResp: ""}
	factory := &passiveMockFactory{backend: backend1}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// trigger session creation
	_ = bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "test", AuthorID: "u1"},
	})
	factory.backend = backend2

	// when
	err := bot.NewSession()

	// then
	r.NoError(err)
	a.True(backend1.closed)
}

type mockPassivePermissionChecker struct {
	checks []permCheck
}

func (m *mockPassivePermissionChecker) Check(toolName string, input map[string]any) (bool, string) {
	m.checks = append(m.checks, permCheck{toolName, input})
	// passive bot denies all writes
	switch toolName {
	case "Read", "Glob", "Grep", "WebFetch", "WebSearch":
		return true, ""
	default:
		return false, "passive mode: read-only"
	}
}

func TestPassiveBot_HandleBufferedMessages_ConverseTimeout(t *testing.T) {
	a := assert.New(t)

	// given
	backend := &passiveMockBackend{sessionID: "p1"}
	backend.converseFunc = func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	factory := &passiveMockFactory{backend: backend}
	discord := &mockDiscordClient{}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)
	bot.converseTimeout = 50 * time.Millisecond

	// when
	err := bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "test", AuthorID: "u1"},
	})

	// then
	a.Error(err)
	a.Contains(err.Error(), "context deadline exceeded")
}
