package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// --- Mock WAClient ---

type mockWAClient struct {
	mock.Mock
}

func (m *mockWAClient) SendText(chatJID, text string) error {
	args := m.Called(chatJID, text)
	return args.Error(0)
}

func (m *mockWAClient) SendTyping(chatJID string) error {
	args := m.Called(chatJID)
	return args.Error(0)
}

func (m *mockWAClient) WaitForReply(senderJID string) (string, error) {
	args := m.Called(senderJID)
	return args.String(0), args.Error(1)
}

func (m *mockWAClient) HandleIncomingReply(senderJID, text string) bool {
	args := m.Called(senderJID, text)
	return args.Bool(0)
}

// --- WAHandler tests ---

func TestWAHandler_AllowedSender_CallsHandleMessage(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("HandleIncomingReply", "sender-1@s.whatsapp.net", "hello").Return(false)
	client.On("HandleIncomingReply", "", "hello").Return(false) // SenderAlt fallback
	h := NewWAHandler(bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)

	// then
	r.Len(bot.handledMessages, 1)
	a.Equal("hello", bot.handledMessages[0].message)
}

func TestWAHandler_DisallowedSender_Ignored(t *testing.T) {
	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := NewWAHandler(bot, []string{"allowed@s.whatsapp.net"}, client)

	evt := makeMessageEvent("stranger@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)

	// then
	assert.Len(t, bot.handledMessages, 0)
}

func TestWAHandler_NewCommand_CallsNewSession(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("HandleIncomingReply", "sender-1@s.whatsapp.net", "!new").Return(false)
	client.On("HandleIncomingReply", "", "!new").Return(false) // SenderAlt fallback
	h := NewWAHandler(bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "!new")

	// when
	h.HandleEvent(evt)

	// then
	a.Equal(1, bot.newSessionCalls)
	a.Len(bot.handledMessages, 0)
}

func TestWAHandler_PermissionReply_Consumed(t *testing.T) {
	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("HandleIncomingReply", "sender-1@s.whatsapp.net", "yes").Return(true)
	h := NewWAHandler(bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "yes")

	// when
	h.HandleEvent(evt)

	// then - consumed by reply waiter, bot not called
	assert.Len(t, bot.handledMessages, 0)
	assert.Equal(t, 0, bot.newSessionCalls)
}

func TestWAHandler_EmptyText_Ignored(t *testing.T) {
	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := NewWAHandler(bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "")

	// when
	h.HandleEvent(evt)

	// then
	assert.Len(t, bot.handledMessages, 0)
}

func TestWAHandler_SenderAltMatch_Allowed(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("HandleIncomingReply", "sender-1@s.whatsapp.net", "hello").Return(false)
	client.On("HandleIncomingReply", "alt-sender@s.whatsapp.net", "hello").Return(false) // SenderAlt fallback
	h := NewWAHandler(bot, []string{"alt-sender@s.whatsapp.net"}, client)

	evt := makeMessageEventWithAlt("sender-1@s.whatsapp.net", "alt-sender@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)

	// then
	r.Len(bot.handledMessages, 1)
	a.Equal("hello", bot.handledMessages[0].message)
}

func TestWAHandler_PhoneNumberOnly_MatchesSenderUser(t *testing.T) {
	r := require.New(t)

	// given — allowed list has phone number with @s.whatsapp.net, sender comes as LID
	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("HandleIncomingReply", "12345@lid", "hello").Return(false)
	client.On("HandleIncomingReply", "27123456789@s.whatsapp.net", "hello").Return(false) // SenderAlt fallback
	h := NewWAHandler(bot, []string{"27123456789@s.whatsapp.net"}, client)

	// sender is LID, alt is the phone number JID
	evt := makeMessageEventWithAlt("12345@lid", "27123456789@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)

	// then
	r.Len(bot.handledMessages, 1)
}

func TestWAHandler_BarePhoneNumber_MatchesSenderAltUser(t *testing.T) {
	r := require.New(t)

	// given — allowed list has bare phone number, sender comes as LID with phone in alt
	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("HandleIncomingReply", "12345@lid", "hello").Return(false)
	client.On("HandleIncomingReply", "27123456789@s.whatsapp.net", "hello").Return(false) // SenderAlt fallback
	h := NewWAHandler(bot, []string{"27123456789"}, client)

	evt := makeMessageEventWithAlt("12345@lid", "27123456789@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)

	// then
	r.Len(bot.handledMessages, 1)
}

func TestWAHandler_NonMessageEvent_Ignored(t *testing.T) {
	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := NewWAHandler(bot, []string{"sender-1@s.whatsapp.net"}, client)

	// when - pass a non-message event (string)
	h.HandleEvent("not a message event")

	// then
	assert.Len(t, bot.handledMessages, 0)
}

// --- WhatsAppClientWrapper WaitForReply/HandleIncomingReply tests ---

func TestWhatsAppClientWrapper_WaitForReply_Success(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given - nil whatsmeow client is fine, WaitForReply doesn't touch it
	wrapper := NewWhatsAppClientWrapper(nil)

	// when
	go func() {
		// small delay to ensure WaitForReply is blocking
		time.Sleep(10 * time.Millisecond)
		wrapper.HandleIncomingReply("sender-1@s.whatsapp.net", "yes")
	}()

	reply, err := wrapper.WaitForReply("sender-1@s.whatsapp.net")

	// then
	r.NoError(err)
	a.Equal("yes", reply)
}

func TestWhatsAppClientWrapper_WaitForReply_Timeout(t *testing.T) {
	a := assert.New(t)

	// given
	wrapper := NewWhatsAppClientWrapper(nil)
	wrapper.timeout = 50 * time.Millisecond

	// when - no reply arrives
	_, err := wrapper.WaitForReply("sender-1@s.whatsapp.net")

	// then
	a.Error(err)
	a.Contains(err.Error(), "timeout")
}

func TestWhatsAppClientWrapper_HandleIncomingReply_WrongSender(t *testing.T) {
	a := assert.New(t)

	// given
	wrapper := NewWhatsAppClientWrapper(nil)

	// set up a waiter for sender-1
	go func() {
		wrapper.WaitForReply("sender-1@s.whatsapp.net")
	}()
	time.Sleep(10 * time.Millisecond) // let waiter register

	// when - wrong sender
	consumed := wrapper.HandleIncomingReply("wrong-sender@s.whatsapp.net", "yes")

	// then
	a.False(consumed)
}

func TestWhatsAppClientWrapper_HandleIncomingReply_NoActiveWaiter(t *testing.T) {
	a := assert.New(t)

	// given
	wrapper := NewWhatsAppClientWrapper(nil)

	// when - no waiter registered
	consumed := wrapper.HandleIncomingReply("sender-1@s.whatsapp.net", "yes")

	// then
	a.False(consumed)
}

// --- extractText tests ---

func TestExtractText_Conversation(t *testing.T) {
	a := assert.New(t)

	// given
	msg := makeWAMessage("hello world")

	// when
	text := extractText(msg)

	// then
	a.Equal("hello world", text)
}

func TestExtractText_ExtendedTextMessage(t *testing.T) {
	a := assert.New(t)

	// given
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("link message"),
		},
	}

	// when
	text := extractText(msg)

	// then
	a.Equal("link message", text)
}

func TestExtractText_Empty(t *testing.T) {
	a := assert.New(t)

	// given
	msg := makeWAMessage("")

	// when
	text := extractText(msg)

	// then
	a.Equal("", text)
}

// --- test helpers ---

func parseJID(s string) types.JID {
	jid, _ := types.ParseJID(s)
	return jid
}

func makeWAMessage(text string) *waE2E.Message {
	if text == "" {
		return &waE2E.Message{}
	}
	return &waE2E.Message{Conversation: proto.String(text)}
}

func makeMessageEvent(senderJID, chatJID, text string) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: parseJID(senderJID),
				Chat:   parseJID(chatJID),
			},
		},
		Message: makeWAMessage(text),
	}
}

func makeMessageEventWithAlt(senderJID, altJID, chatJID, text string) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender:    parseJID(senderJID),
				SenderAlt: parseJID(altJID),
				Chat:      parseJID(chatJID),
			},
		},
		Message: makeWAMessage(text),
	}
}

