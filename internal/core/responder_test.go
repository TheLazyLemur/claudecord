package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDiscordResponder_SendTyping(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	client.On("SendTyping", "chan-1").Return(nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	err := r.SendTyping()

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestDiscordResponder_PostResponse_Short(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	client.On("SendMessage", "chan-1", "hello").Return(nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	err := r.PostResponse("hello")

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestDiscordResponder_PostResponse_Long_CreatesThread(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	longMsg := string(make([]byte, 2001))
	client.On("CreateThread", "chan-1", longMsg).Return("thread-1", nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	err := r.PostResponse(longMsg)

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestDiscordResponder_AddReaction(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	client.On("AddReaction", "chan-1", "msg-1", "👍").Return(nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	err := r.AddReaction("👍")

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestDiscordResponder_SendUpdate_CreatesThread(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	client.On("StartThread", "chan-1", "msg-1", "Updates").Return("thread-1", nil)
	client.On("SendMessage", "thread-1", "working on it").Return(nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	err := r.SendUpdate("working on it")

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestDiscordResponder_SendUpdate_ReusesThread(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	client.On("StartThread", "chan-1", "msg-1", "Updates").Return("thread-1", nil).Once()
	client.On("SendMessage", "thread-1", "first").Return(nil)
	client.On("SendMessage", "thread-1", "second").Return(nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	r.SendUpdate("first")
	err := r.SendUpdate("second")

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestDiscordResponder_AskPermission_Approved(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	client.On("SendMessageReturningID", "chan-1", "Allow file edit? React ✅ or ❌").Return("perm-msg-1", nil)
	client.On("AddReaction", "chan-1", "perm-msg-1", "✅").Return(nil)
	client.On("AddReaction", "chan-1", "perm-msg-1", "❌").Return(nil)
	client.On("WaitForReaction", "chan-1", "perm-msg-1", []string{"✅", "❌"}, "user-1").Return("✅", nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	r.SetUserID("user-1")
	approved, err := r.AskPermission("Allow file edit?")

	a.NoError(err)
	a.True(approved)
	client.AssertExpectations(t)
}

func TestDiscordResponder_AskPermission_Denied(t *testing.T) {
	a := assert.New(t)
	client := &MockDiscordClient{}
	client.On("SendMessageReturningID", "chan-1", "Allow bash cmd? React ✅ or ❌").Return("perm-msg-2", nil)
	client.On("AddReaction", "chan-1", "perm-msg-2", "✅").Return(nil)
	client.On("AddReaction", "chan-1", "perm-msg-2", "❌").Return(nil)
	client.On("WaitForReaction", "chan-1", "perm-msg-2", []string{"✅", "❌"}, "user-1").Return("❌", nil)

	r := NewDiscordResponder(client, "chan-1", "msg-1")
	r.SetUserID("user-1")
	approved, err := r.AskPermission("Allow bash cmd?")

	a.NoError(err)
	a.False(approved)
	client.AssertExpectations(t)
}

func TestEmailResponder_SendTyping_Noop(t *testing.T) {
	a := assert.New(t)
	client := &MockEmailClient{}

	r := NewEmailResponder(client, "user@example.com", "Re: Hello")
	err := r.SendTyping()

	a.NoError(err)
}

func TestEmailResponder_PostResponse_SendsEmail(t *testing.T) {
	a := assert.New(t)
	client := &MockEmailClient{}
	client.On("Send", "user@example.com", "Re: Hello", "Here is my response").Return(nil)

	r := NewEmailResponder(client, "user@example.com", "Re: Hello")
	err := r.PostResponse("Here is my response")

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestEmailResponder_AddReaction_Noop(t *testing.T) {
	a := assert.New(t)
	client := &MockEmailClient{}

	r := NewEmailResponder(client, "user@example.com", "Re: Hello")
	err := r.AddReaction("👍")

	a.NoError(err)
}

func TestEmailResponder_SendUpdate_Noop(t *testing.T) {
	a := assert.New(t)
	client := &MockEmailClient{}

	r := NewEmailResponder(client, "user@example.com", "Re: Hello")
	err := r.SendUpdate("working on it")

	a.NoError(err)
}

func TestWhatsAppResponder_SendTyping(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}
	client.On("SendTyping", "chat-1@s.whatsapp.net").Return(nil)

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	err := r.SendTyping()

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestWhatsAppResponder_PostResponse_Short(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}
	client.On("SendText", "chat-1@s.whatsapp.net", "hello").Return(nil)

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	err := r.PostResponse("hello")

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestWhatsAppResponder_PostResponse_Chunked(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}
	long := strings.Repeat("x", MaxWhatsAppMessageLen+100)
	client.On("SendText", "chat-1@s.whatsapp.net", strings.Repeat("x", MaxWhatsAppMessageLen)).Return(nil)
	client.On("SendText", "chat-1@s.whatsapp.net", strings.Repeat("x", 100)).Return(nil)

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	err := r.PostResponse(long)

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestWhatsAppResponder_AddReaction_Noop(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	err := r.AddReaction("👍")

	a.NoError(err)
}

func TestWhatsAppResponder_SendUpdate(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}
	client.On("SendText", "chat-1@s.whatsapp.net", "working on it").Return(nil)

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	err := r.SendUpdate("working on it")

	a.NoError(err)
	client.AssertExpectations(t)
}

func TestWhatsAppResponder_AskPermission_ApprovedYes(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}
	client.On("SendText", "chat-1@s.whatsapp.net", "Allow file edit?\nReply yes/no").Return(nil)
	client.On("WaitForReply", "sender-1@s.whatsapp.net").Return("yes", nil)

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	approved, err := r.AskPermission("Allow file edit?")

	a.NoError(err)
	a.True(approved)
	client.AssertExpectations(t)
}

func TestWhatsAppResponder_AskPermission_ApprovedY(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}
	client.On("SendText", "chat-1@s.whatsapp.net", "Allow file edit?\nReply yes/no").Return(nil)
	client.On("WaitForReply", "sender-1@s.whatsapp.net").Return("Y", nil)

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	approved, err := r.AskPermission("Allow file edit?")

	a.NoError(err)
	a.True(approved)
	client.AssertExpectations(t)
}

func TestWhatsAppResponder_AskPermission_Denied(t *testing.T) {
	a := assert.New(t)
	client := &MockWhatsAppMessenger{}
	client.On("SendText", "chat-1@s.whatsapp.net", "Allow file edit?\nReply yes/no").Return(nil)
	client.On("WaitForReply", "sender-1@s.whatsapp.net").Return("no", nil)

	r := NewWhatsAppResponder(client, "chat-1@s.whatsapp.net", "sender-1@s.whatsapp.net")
	approved, err := r.AskPermission("Allow file edit?")

	a.NoError(err)
	a.False(approved)
	client.AssertExpectations(t)
}

func TestChunkMessage_Short(t *testing.T) {
	a := assert.New(t)
	a.Equal([]string{"hello"}, ChunkMessage("hello", 10))
}

func TestChunkMessage_ExactLimit(t *testing.T) {
	a := assert.New(t)
	a.Equal([]string{"12345"}, ChunkMessage("12345", 5))
}

func TestChunkMessage_MultipleChunks(t *testing.T) {
	a := assert.New(t)
	a.Equal([]string{"abc", "def", "ghi", "j"}, ChunkMessage("abcdefghij", 3))
}

func TestChunkMessage_Empty(t *testing.T) {
	a := assert.New(t)
	a.Empty(ChunkMessage("", 10))
}

func TestChunkMessage_DiscordLimit(t *testing.T) {
	a := assert.New(t)
	long := strings.Repeat("x", 4500)
	chunks := ChunkMessage(long, MaxDiscordMessageLen)
	a.Len(chunks, 3)
	a.Len(chunks[0], 2000)
	a.Len(chunks[1], 2000)
	a.Len(chunks[2], 500)
}

// MockEmailClient for tests
type MockEmailClient struct {
	mock.Mock
}

func (m *MockEmailClient) Send(to, subject, body string) error {
	args := m.Called(to, subject, body)
	return args.Error(0)
}

// MockDiscordClient for tests
type MockDiscordClient struct {
	mock.Mock
}

func (m *MockDiscordClient) SendMessage(channelID, content string) error {
	args := m.Called(channelID, content)
	return args.Error(0)
}

func (m *MockDiscordClient) CreateThread(channelID, content string) (string, error) {
	args := m.Called(channelID, content)
	return args.String(0), args.Error(1)
}

func (m *MockDiscordClient) StartThread(channelID, messageID, name string) (string, error) {
	args := m.Called(channelID, messageID, name)
	return args.String(0), args.Error(1)
}

func (m *MockDiscordClient) SendTyping(channelID string) error {
	args := m.Called(channelID)
	return args.Error(0)
}

func (m *MockDiscordClient) AddReaction(channelID, messageID, emoji string) error {
	args := m.Called(channelID, messageID, emoji)
	return args.Error(0)
}

func (m *MockDiscordClient) SendMessageReturningID(channelID, content string) (string, error) {
	args := m.Called(channelID, content)
	return args.String(0), args.Error(1)
}

func (m *MockDiscordClient) WaitForReaction(channelID, messageID string, emojis []string, userID string) (string, error) {
	args := m.Called(channelID, messageID, emojis, userID)
	return args.String(0), args.Error(1)
}

// MockWhatsAppMessenger for tests
type MockWhatsAppMessenger struct {
	mock.Mock
}

func (m *MockWhatsAppMessenger) SendText(chatJID, text string) error {
	args := m.Called(chatJID, text)
	return args.Error(0)
}

func (m *MockWhatsAppMessenger) SendTyping(chatJID string) error {
	args := m.Called(chatJID)
	return args.Error(0)
}

func (m *MockWhatsAppMessenger) WaitForReply(senderJID string) (string, error) {
	args := m.Called(senderJID)
	return args.String(0), args.Error(1)
}
