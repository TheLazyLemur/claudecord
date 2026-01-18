package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPassiveBot_HandleBufferedMessages_NoResponseIfEmpty(t *testing.T) {
	a := assert.New(t)

	// given
	recvChan := make(chan []byte, 2)
	proc := &botMockProcess{sessionID: "p1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// empty response from Claude
	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":""}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

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
	recvChan := make(chan []byte, 2)
	proc := &botMockProcess{sessionID: "p1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Here's how to do that in Go..."}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

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
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 2)
	proc := &botMockProcess{sessionID: "p1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	assistant := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Answer"}]}}`
	result := `{"type":"result","subtype":"success","result":"done"}`
	recvChan <- []byte(assistant)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	err := bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "first question", AuthorID: "u1"},
		{ChannelID: "chan-1", MessageID: "m2", Content: "more context", AuthorID: "u1"},
	})

	// then
	r.NoError(err)

	// verify combined message sent to CLI
	r.Len(proc.sentMessages, 1)
	var msg map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[0], &msg))
	message := msg["message"].(map[string]any)
	content := message["content"].(string)
	r.Contains(content, "first question")
	r.Contains(content, "more context")
}

func TestPassiveBot_HandleBufferedMessages_UsesReadOnlyPermissions(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	recvChan := make(chan []byte, 4)
	proc := &botMockProcess{sessionID: "p1", recvChan: recvChan}
	factory := &botMockFactory{process: proc}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// Claude tries to write
	permReq := `{"type":"control_request","request_id":"req-1","request":{"subtype":"can_use_tool","tool_name":"Write","tool_use_id":"toolu_123","input":{"file_path":"/tmp/test.txt"}}}`
	result := `{"type":"result","subtype":"success","result":"denied"}`
	recvChan <- []byte(permReq)
	recvChan <- []byte(result)
	close(recvChan)

	// when
	_ = bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "test", AuthorID: "u1"},
	})

	// then
	r.Len(perms.checks, 1)
	a.Equal("Write", perms.checks[0].toolName)

	// verify deny response sent
	r.Len(proc.sentMessages, 2) // user msg + deny
	var resp map[string]any
	r.NoError(json.Unmarshal(proc.sentMessages[1], &resp))
	response := resp["response"].(map[string]any)
	inner := response["response"].(map[string]any)
	a.Equal("deny", inner["behavior"])
}

func TestPassiveBot_NewSession_ResetsSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	proc1 := &botMockProcess{sessionID: "p1", recvChan: make(chan []byte)}
	proc2 := &botMockProcess{sessionID: "p2", recvChan: make(chan []byte)}
	factory := &botMockFactory{process: proc1}
	discord := &mockDiscordClient{threadID: "thread-1"}
	perms := &mockPassivePermissionChecker{}
	bot := NewPassiveBot(NewSessionManager(factory), discord, perms)

	// trigger session creation
	close(proc1.recvChan)
	_ = bot.HandleBufferedMessages("chan-1", []BufferedMessage{
		{ChannelID: "chan-1", MessageID: "m1", Content: "test", AuthorID: "u1"},
	})
	factory.process = proc2

	// when
	err := bot.NewSession()

	// then
	r.NoError(err)
	a.True(proc1.closed)
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
