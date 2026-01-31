package core

import (
	"encoding/json"
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
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// send result to complete
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "hello")

	// then
	r.NoError(err)
	a.True(responder.typingCalled)
}

func TestBot_HandleMessage_SendsUserMessageToCLI(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 2)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "test message")

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

func TestBot_HandleMessage_PostsAssistantTextViaResponder(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 3)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello there!"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "hi")

	// then
	r.NoError(err)
	r.Len(responder.responses, 1)
	a.Equal("Hello there!", responder.responses[0])
}

func TestBot_HandleMessage_HandlesPermissionRequest_Allow(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	permReq := `{"type":"control_request","request_id":"req-1","request":{"subtype":"can_use_tool","tool_name":"Write","tool_use_id":"toolu_123","input":{"file_path":"/tmp/test.txt"}}}`
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done!"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(permReq)
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "write file")

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
	perms := &mockPermissionChecker{allowAll: false, reason: "path not allowed"}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	permReq := `{"type":"control_request","request_id":"req-2","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"toolu_456","input":{"command":"rm -rf /"}}}`
	result := `{"type":"result","subtype":"success","result":"denied"}`
	recvChan <- []byte(permReq)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "delete everything")

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

func TestBot_HandleMessage_PermissionDeniedByChecker_UserApproves(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... checker denies but user approves via reaction
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: false, reason: "needs approval"}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{askPermissionAllow: true}

	permReq := `{"type":"control_request","request_id":"req-3","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"toolu_789","input":{"command":"echo hello"}}}`
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Done!"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(permReq)
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "run echo")

	// then
	r.NoError(err)

	// should have asked user for permission
	r.Len(responder.permissionPrompts, 1)
	a.Contains(responder.permissionPrompts[0], "Bash")

	// should have sent allow response
	r.Len(proc.sentMessages, 2)
	var resp map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[1], &resp))
	response := resp["response"].(map[string]any)
	inner := response["response"].(map[string]any)
	a.Equal("allow", inner["behavior"])
	a.Equal("toolu_789", inner["toolUseID"])
}

func TestBot_HandleMessage_PermissionDeniedByChecker_UserDenies(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... checker denies and user also denies via reaction
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: false, reason: "needs approval"}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{askPermissionAllow: false}

	permReq := `{"type":"control_request","request_id":"req-4","request":{"subtype":"can_use_tool","tool_name":"Bash","tool_use_id":"toolu_999","input":{"command":"rm -rf /"}}}`
	result := `{"type":"result","subtype":"success","result":"denied"}`
	recvChan <- []byte(permReq)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "delete all")

	// then
	r.NoError(err)

	// should have asked user for permission
	r.Len(responder.permissionPrompts, 1)

	// should have sent deny response
	r.Len(proc.sentMessages, 2)
	var resp map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[1], &resp))
	response := resp["response"].(map[string]any)
	inner := response["response"].(map[string]any)
	a.Equal("deny", inner["behavior"])
	a.Equal("toolu_999", inner["toolUseID"])
}

func TestBot_HandleMessage_ConcatenatesMultipleTextBlocks(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 3)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello "},{"type":"tool_use","name":"test"},{"type":"text","text":"world!"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "hi")

	// then
	r.NoError(err)
	r.Len(responder.responses, 1)
	a.Equal("Hello world!", responder.responses[0])
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

func TestBot_HandleMessage_SendError(t *testing.T) {
	// given
	recvChan := make(chan []byte)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan, sendErr: errors.New("send failed")}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "hello")

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
	perms := &mockPermissionChecker{}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// create initial session
	close(proc1.recvChan)
	_ = bot.HandleMessage(responder, "init")
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
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// only tool_use, no text
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"test"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "hi")

	// then
	r.NoError(err)
	a.Len(responder.responses, 0)
}

func TestBot_HandleMessage_IgnoresReplayMessages(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	// replay message should be ignored
	replay := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Old response"}]},"isReplay":true}`
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"New response"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(replay)
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "hi")

	// then
	r.NoError(err)
	r.Len(responder.responses, 1)
	a.Equal("New response", responder.responses[0])
}

// --- MCP Message Tests ---

func TestBot_HandleMessage_MCPReactEmoji_Success(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 3)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	mcpReq := `{"type":"control_request","request_id":"mcp-123","request":{"subtype":"mcp_message","server_name":"discord-tools","message":{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"react_emoji","arguments":{"emoji":"eyes"}}}}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(mcpReq)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "react to this")

	// then
	r.NoError(err)
	r.Len(responder.reactions, 1)
	a.Equal("eyes", responder.reactions[0])

	// check MCP success response sent
	r.Len(proc.sentMessages, 2) // user msg + mcp response
	var resp map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[1], &resp))
	a.Equal("control_response", resp["type"])
	response := resp["response"].(map[string]any)
	a.Equal("success", response["subtype"])
	a.Equal("mcp-123", response["request_id"])
	innerResp := response["response"].(map[string]any)
	mcpResp := innerResp["mcp_response"].(map[string]any)
	a.Equal("2.0", mcpResp["jsonrpc"])
	a.Equal(float64(1), mcpResp["id"])
	mcpResult := mcpResp["result"].(map[string]any)
	content := mcpResult["content"].([]any)
	r.Len(content, 1)
	block := content[0].(map[string]any)
	a.Equal("text", block["type"])
	a.Equal("reaction added", block["text"])
}

func TestBot_HandleMessage_MCPSendUpdate_Success(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 3)
	proc := &botMockProcess{sessionID: "s1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	perms := &mockPermissionChecker{allowAll: true}
	bot := NewBot(NewSessionManager(factory), perms)
	responder := &mockResponder{}

	mcpReq := `{"type":"control_request","request_id":"mcp-456","request":{"subtype":"mcp_message","server_name":"discord-tools","message":{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"send_update","arguments":{"message":"Working on it..."}}}}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(mcpReq)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleMessage(responder, "do something")

	// then
	r.NoError(err)
	r.Len(responder.updates, 1)
	a.Equal("Working on it...", responder.updates[0])
}
