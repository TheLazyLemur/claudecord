package core

import (
	"encoding/json"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mocks ---

type mockDiscordClient struct {
	sentMessages   []sentMessage
	createdThreads []createdThread
	typingChannels []string
	sendErr        error
	threadErr      error
	threadID       string
}

type sentMessage struct {
	channelID string
	content   string
}

type createdThread struct {
	channelID string
	content   string
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
	return nil
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

type botMockProcess struct {
	sessionID    string
	sentMessages [][]byte
	recvChan     chan []byte
	closed       bool
	sendErr      error
}

func (m *botMockProcess) Send(msg []byte) error {
	if m.closed {
		return errors.New("process closed")
	}
	m.sentMessages = append(m.sentMessages, msg)
	return m.sendErr
}

func (m *botMockProcess) Receive() (<-chan []byte, error) {
	return m.recvChan, nil
}

func (m *botMockProcess) Close() error {
	m.closed = true
	return nil
}

func (m *botMockProcess) SessionID() string {
	return m.sessionID
}

type botMockFactory struct {
	process *botMockProcess
	err     error
}

func (f *botMockFactory) Create(resumeSessionID, workDir string) (CLIProcess, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.process, nil
}

// --- Tests ---

func TestBot_HandleMessage_SendsTypingIndicator(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 2)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	// send result to complete
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hello")

	// then
	r.NoError(err)
	a.Contains(discord.typingChannels, "chan-1")
}

func TestBot_HandleMessage_SendsUserMessageToCLI(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 2)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "test message")

	// then
	r.NoError(err)
	r.Len(proc.sentMessages, 1)

	var msg map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[0], &msg))
	a.Equal("user", msg["type"])
	a.Equal("s1", msg["session_id"])
	message := msg["message"].(map[string]any)
	a.Equal("user", message["role"])
	a.Equal("test message", message["content"])
}

func TestBot_HandleMessage_PostsAssistantTextToDiscord(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 3)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello there!"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hi")

	// then
	r.NoError(err)
	r.Len(discord.sentMessages, 1)
	a.Equal("chan-1", discord.sentMessages[0].channelID)
	a.Equal("Hello there!", discord.sentMessages[0].content)
}

func TestBot_HandleMessage_CreatesThreadForLongResponse(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 3)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	// create >2000 char response
	longText := make([]byte, 2100)
	for i := range longText {
		longText[i] = 'a'
	}
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"` + string(longText) + `"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hi")

	// then
	r.NoError(err)
	a.Len(discord.sentMessages, 0)
	r.Len(discord.createdThreads, 1)
	a.Equal("chan-1", discord.createdThreads[0].channelID)
	a.Equal(string(longText), discord.createdThreads[0].content)
}

func TestBot_HandleMessage_HandlesPermissionRequest_Allow(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	permReq := `{"type":"control_request","request_id":"req-1","request":{"subtype":"can_use_tool","tool_name":"Write","tool_use_id":"toolu_123","input":{"file_path":"/tmp/test.txt"}}}`
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done!"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(permReq)
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "write file")

	// then
	r.NoError(err)
	r.Len(perms.checks, 1)
	a.Equal("Write", perms.checks[0].toolName)
	a.Equal("/tmp/test.txt", perms.checks[0].input["file_path"])

	// check response sent to CLI
	r.Len(proc.sentMessages, 2) // user msg + control response
	var resp map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[1], &resp))
	a.Equal("control_response", resp["type"])
	response := resp["response"].(map[string]any)
	a.Equal("success", response["subtype"])
	a.Equal("req-1", response["request_id"])
	inner := response["response"].(map[string]any)
	a.Equal("allow", inner["behavior"])
	a.Equal("toolu_123", inner["toolUseID"])
	// updatedInput is required per protocol spec
	updatedInput := inner["updatedInput"].(map[string]any)
	a.Equal("/tmp/test.txt", updatedInput["file_path"])
}

func TestBot_HandleMessage_HandlesPermissionRequest_Deny(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: false, reason: "path not allowed"}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	permReq := `{"type":"control_request","request_id":"req-2","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"toolu_456","input":{"command":"rm -rf /"}}}`
	result := `{"type":"result","subtype":"success","result":"denied"}`
	recvChan <- []byte(permReq)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "delete everything")

	// then
	r.NoError(err)

	// check deny response sent
	r.Len(proc.sentMessages, 2)
	var resp map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[1], &resp))
	response := resp["response"].(map[string]any)
	inner := response["response"].(map[string]any)
	a.Equal("deny", inner["behavior"])
	a.Equal("toolu_456", inner["toolUseID"])
	a.Equal("path not allowed", inner["message"])
}

func TestBot_HandleMessage_ConcatenatesMultipleTextBlocks(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 3)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello "},{"type":"tool_use","name":"test"},{"type":"text","text":"world!"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hi")

	// then
	r.NoError(err)
	r.Len(discord.sentMessages, 1)
	a.Equal("Hello world!", discord.sentMessages[0].content)
}

func TestBot_HandleMessage_SessionError(t *testing.T) {
	// given
	factory := &botMockFactory{err: errors.New("spawn failed")}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hello")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "spawn failed")
}

func TestBot_HandleMessage_SendError(t *testing.T) {
	// given
	recvChan := make(chan []byte)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan, sendErr: errors.New("send failed")}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hello")

	// then
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "send failed")
}

func TestBot_NewSession_StartsNewSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	proc1 := &botMockProcess{sessionID: "s1", recvChan: make(chan []byte)}
	proc2 := &botMockProcess{sessionID: "s2", recvChan: make(chan []byte)}
	factory := &botMockFactory{process: proc1}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	// create initial session
	close(proc1.recvChan)
	_ = bot.HandleMessage("chan-1", "msg-1", "init")
	factory.process = proc2

	// when
	err := bot.NewSession("")

	// then
	r.NoError(err)
	a.True(proc1.closed)
}

func TestBot_HandleMessage_NoResponseIfNoText(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 2)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	// only tool_use, no text
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"test"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hi")

	// then
	r.NoError(err)
	a.Len(discord.sentMessages, 0)
	a.Len(discord.createdThreads, 0)
}

func TestBot_HandleMessage_IgnoresReplayMessages(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), discord, perms)

	// replay message should be ignored
	replay := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Old response"}]},"isReplay":true}`
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"New response"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(replay)
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage("chan-1", "msg-1", "hi")

	// then
	r.NoError(err)
	r.Len(discord.sentMessages, 1)
	a.Equal("New response", discord.sentMessages[0].content)
}
