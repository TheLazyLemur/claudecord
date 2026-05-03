package handler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.mau.fi/whatsmeow"
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

func (m *mockWAClient) Download(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error) {
	args := m.Called(ctx, msg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// testBurstDelay is short enough that tests stay snappy, long enough that the
// race detector's added overhead can't make sequential HandleEvent calls miss
// the same debounce window.
const testBurstDelay = 100 * time.Millisecond

func newTestWAHandler(t *testing.T, bot BotInterface, allowed []string, client WAClient) *WAHandler {
	t.Helper()
	h := NewWAHandler(bot, allowed, client, t.TempDir())
	h.SetBurstDelay(testBurstDelay)
	t.Cleanup(h.Stop)
	return h
}

// --- WAHandler tests ---

func TestWAHandler_AllowedSender_CallsHandleMessage(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond) // wait for debounced flush

	// then — message is wrapped in the prompt-shape XML
	r.Equal(1, bot.handledCount())
	a.Contains(bot.handledAt(0).message, "<text>hello</text>")
}

func TestWAHandler_DisallowedSender_Ignored(t *testing.T) {
	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"allowed@s.whatsapp.net"}, client)

	evt := makeMessageEvent("stranger@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)

	// then
	assert.Equal(t, 0, bot.handledCount())
}

func TestWAHandler_NewCommand_CallsNewSession(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "!new")

	// when
	h.HandleEvent(evt)

	// then
	a.Equal(1, bot.newSessionCount())
	a.Equal(0, bot.handledCount())
}

func TestWAHandler_EmptyText_Ignored(t *testing.T) {
	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "")

	// when
	h.HandleEvent(evt)

	// then
	assert.Equal(t, 0, bot.handledCount())
}

func TestWAHandler_SenderAltMatch_Allowed(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"alt-sender@s.whatsapp.net"}, client)

	evt := makeMessageEventWithAlt("sender-1@s.whatsapp.net", "alt-sender@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond) // wait for debounced flush

	// then — message is wrapped in the prompt-shape XML
	r.Equal(1, bot.handledCount())
	a.Contains(bot.handledAt(0).message, "<text>hello</text>")
}

func TestWAHandler_PhoneNumberOnly_MatchesSenderUser(t *testing.T) {
	r := require.New(t)

	// given — allowed list has phone number with @s.whatsapp.net, sender comes as LID
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"27123456789@s.whatsapp.net"}, client)

	// sender is LID, alt is the phone number JID
	evt := makeMessageEventWithAlt("12345@lid", "27123456789@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond) // wait for debounced flush

	// then
	r.Equal(1, bot.handledCount())
}

func TestWAHandler_BarePhoneNumber_MatchesSenderAltUser(t *testing.T) {
	r := require.New(t)

	// given — allowed list has bare phone number, sender comes as LID with phone in alt
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"27123456789"}, client)

	evt := makeMessageEventWithAlt("12345@lid", "27123456789@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	h.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond) // wait for debounced flush

	// then
	r.Equal(1, bot.handledCount())
}

func TestWAHandler_NonMessageEvent_Ignored(t *testing.T) {
	// given
	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	// when - pass a non-message event (string)
	h.HandleEvent("not a message event")

	// then
	assert.Equal(t, 0, bot.handledCount())
}

// --- Burst batching + attachment tests ---

func TestWAHandler_BurstBatch_DispatchesOnce(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	bot := &mockBot{}
	client := &mockWAClient{}
	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	for _, text := range []string{"one", "two", "three"} {
		h.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", text))
	}
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	r.Equal(1, bot.handledCount())
	a.Contains(bot.handledAt(0).message, "<text>one</text>")
	a.Contains(bot.handledAt(0).message, "<text>two</text>")
	a.Contains(bot.handledAt(0).message, "<text>three</text>")
}

func TestWAHandler_AttachmentOnly_FlushedWithAttachmentTag(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("Download", mock.Anything, mock.Anything).Return([]byte("PNGDATA"), nil)
	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeImageMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "look", "image/png")
	h.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	r.Equal(1, bot.handledCount())
	a.Contains(bot.handledAt(0).message, "<text>look</text>")
	a.Contains(bot.handledAt(0).message, `mime="image/png"`)
}

func TestWAHandler_OversizedAttachment_SkippedWithNotice(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	bot := &mockBot{}
	client := &mockWAClient{}
	huge := make([]byte, MaxImageBytes+1)
	client.On("Download", mock.Anything, mock.Anything).Return(huge, nil)
	client.On("SendText", "chat-1@g.us", mock.MatchedBy(func(s string) bool {
		return strings.HasPrefix(s, "skipped (too large):")
	})).Return(nil)

	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	evt := makeImageMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "", "image/png")
	h.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// Caption was empty, attachment was dropped → nothing flushed to bot.
	r.Equal(0, bot.handledCount())
	client.AssertCalled(t, "SendText", "chat-1@g.us", mock.AnythingOfType("string"))
	a.True(true) // smoke
}

func TestWAHandler_MixedTextAndAttachmentBatch_OrderPreserved(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	bot := &mockBot{}
	client := &mockWAClient{}
	client.On("Download", mock.Anything, mock.Anything).Return([]byte("DATA"), nil)
	h := newTestWAHandler(t, bot, []string{"sender-1@s.whatsapp.net"}, client)

	h.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "first"))
	h.HandleEvent(makeImageMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "", "image/png"))
	h.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "third"))
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	r.Equal(1, bot.handledCount())
	body := bot.handledAt(0).message
	idx1 := strings.Index(body, "first")
	idx2 := strings.Index(body, "image/png")
	idx3 := strings.Index(body, "third")
	a.True(idx1 >= 0 && idx2 > idx1 && idx3 > idx2, "out of order: %s", body)
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

func makeImageMessageEvent(senderJID, chatJID, caption, mime string) *events.Message {
	img := &waE2E.ImageMessage{
		Mimetype: proto.String(mime),
	}
	if caption != "" {
		img.Caption = proto.String(caption)
	}
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Sender: parseJID(senderJID),
				Chat:   parseJID(chatJID),
			},
		},
		Message: &waE2E.Message{ImageMessage: img},
	}
}

