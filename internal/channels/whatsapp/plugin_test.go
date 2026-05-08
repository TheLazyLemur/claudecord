package whatsapp

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	waow "go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// --- mocks ---

type messengerMock struct{ mock.Mock }

func (m *messengerMock) SendText(jid, text string) error {
	args := m.Called(jid, text)
	return args.Error(0)
}
func (m *messengerMock) SendTyping(jid string) error {
	args := m.Called(jid)
	return args.Error(0)
}

type downloaderMock struct{ mock.Mock }

func (d *downloaderMock) Download(ctx context.Context, msg waow.DownloadableMessage) ([]byte, error) {
	args := d.Called(ctx, msg)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

// testBurstDelay is short enough that tests stay snappy, long enough that the
// race detector's added overhead can't make sequential HandleEvent calls miss
// the same debounce window.
const testBurstDelay = 100 * time.Millisecond

// deliverSink collects inbounds delivered by the plugin.
type deliverSink struct {
	mu       sync.Mutex
	inbounds []core.Inbound
}

func (s *deliverSink) fn() func(core.Inbound) {
	return func(in core.Inbound) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.inbounds = append(s.inbounds, in)
	}
}

func (s *deliverSink) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.inbounds)
}

func (s *deliverSink) at(i int) core.Inbound {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inbounds[i]
}

func newTestPlugin(t *testing.T, msgr *messengerMock, dl *downloaderMock, allowed []string) (*Plugin, *deliverSink) {
	t.Helper()
	p := New(Config{
		Messenger:      msgr,
		Downloader:     dl,
		AllowedSenders: allowed,
		MediaDir:       t.TempDir(),
	})
	p.setBurstDelay(testBurstDelay)
	t.Cleanup(func() { _ = p.Stop() })
	sink := &deliverSink{}
	_ = p.Start(context.Background(), sink.fn())
	return p, sink
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

// --- plugin shape tests ---

func TestPlugin_DeliversInboundWithChatJIDAsSessionKey(t *testing.T) {
	// given
	// ... a plugin holding a fake messenger and a delivery sink
	msgr := &messengerMock{}
	p := New(Config{Messenger: msgr, AllowedSenders: []string{"15551234567@s.whatsapp.net"}})
	var received core.Inbound

	// when
	// ... an inbound is simulated through the plugin's deliver path
	_ = p.Start(context.Background(), func(in core.Inbound) { received = in })
	p.deliverForTest(core.Inbound{
		SessionKey: "whatsapp:15551234567@s.whatsapp.net",
		Text:       "hello",
	})

	// then
	// ... the SessionKey is the chat JID prefixed with the channel id
	if received.SessionKey != "whatsapp:15551234567@s.whatsapp.net" {
		t.Fatalf("session key: %q", received.SessionKey)
	}
	if received.Text != "hello" {
		t.Fatalf("text: %q", received.Text)
	}
}

func TestPlugin_Capabilities(t *testing.T) {
	// given
	// ... a default plugin
	p := New(Config{Messenger: &messengerMock{}})

	// when
	// ... capabilities are queried
	caps := p.Capabilities()

	// then
	// ... typing yes, reactions no
	if !caps.Typing || caps.Reactions {
		t.Fatalf("unexpected caps: %+v", caps)
	}
}

func TestPlugin_ID(t *testing.T) {
	// given
	// ... a default plugin
	p := New(Config{Messenger: &messengerMock{}})

	// when
	// ... id is queried
	id := p.ID()

	// then
	// ... it is "whatsapp"
	if id != "whatsapp" {
		t.Fatalf("id: %q", id)
	}
}

// --- HandleEvent tests ---

func TestPlugin_AllowedSender_DeliversInbound(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a plugin with one allowed sender
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"sender-1@s.whatsapp.net"})
	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	// ... an allowed sender sends a message
	p.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... the inbound is delivered with the rendered prompt
	r.Equal(1, sink.count())
	a.Contains(sink.at(0).Text, "<text>hello</text>")
	a.Equal(SessionKey("chat-1@g.us"), sink.at(0).SessionKey)
}

func TestPlugin_DisallowedSender_Ignored(t *testing.T) {
	// given
	// ... a plugin with a different allowed sender
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"allowed@s.whatsapp.net"})
	evt := makeMessageEvent("stranger@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	// ... an unauthorized sender sends a message
	p.HandleEvent(evt)

	// then
	// ... no inbound is delivered
	assert.Equal(t, 0, sink.count())
}

func TestPlugin_EmptyText_Ignored(t *testing.T) {
	// given
	// ... a plugin with an allowed sender
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"sender-1@s.whatsapp.net"})
	evt := makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "")

	// when
	// ... an empty message arrives
	p.HandleEvent(evt)

	// then
	// ... nothing is delivered
	assert.Equal(t, 0, sink.count())
}

func TestPlugin_SenderAltMatch_Allowed(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a plugin whose allowed list contains the alt JID
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"alt-sender@s.whatsapp.net"})
	evt := makeMessageEventWithAlt("sender-1@s.whatsapp.net", "alt-sender@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	// ... the message arrives with the primary JID not in the allowed list
	p.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... it is still delivered because alt JID matches
	r.Equal(1, sink.count())
	a.Contains(sink.at(0).Text, "<text>hello</text>")
}

func TestPlugin_PhoneNumberOnly_MatchesSenderUser(t *testing.T) {
	r := require.New(t)

	// given
	// ... allowed list has phone number with @s.whatsapp.net, sender comes as LID
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"27123456789@s.whatsapp.net"})
	evt := makeMessageEventWithAlt("12345@lid", "27123456789@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	// ... the message arrives with LID as sender and phone JID as alt
	p.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... it is delivered
	r.Equal(1, sink.count())
}

func TestPlugin_BarePhoneNumber_MatchesSenderAltUser(t *testing.T) {
	r := require.New(t)

	// given
	// ... allowed list has bare phone number
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"27123456789"})
	evt := makeMessageEventWithAlt("12345@lid", "27123456789@s.whatsapp.net", "chat-1@g.us", "hello")

	// when
	// ... the message arrives with LID sender and phone JID as alt
	p.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... it is delivered via user-part match
	r.Equal(1, sink.count())
}

func TestPlugin_NonMessageEvent_Ignored(t *testing.T) {
	// given
	// ... a plugin with an allowed sender
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"sender-1@s.whatsapp.net"})

	// when
	// ... a non-message event is passed
	p.HandleEvent("not a message event")

	// then
	// ... nothing is delivered
	assert.Equal(t, 0, sink.count())
}

func TestPlugin_BurstBatch_DispatchesOnce(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a plugin receiving three rapid messages
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	p, sink := newTestPlugin(t, msgr, dl, []string{"sender-1@s.whatsapp.net"})

	// when
	// ... three messages arrive within the burst window
	p.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "one"))
	p.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "two"))
	p.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "three"))
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... a single inbound is delivered containing all three texts
	r.Equal(1, sink.count())
	a.Contains(sink.at(0).Text, "<text>one</text>")
	a.Contains(sink.at(0).Text, "<text>two</text>")
	a.Contains(sink.at(0).Text, "<text>three</text>")
}

func TestPlugin_AttachmentOnly_FlushedWithAttachmentTag(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a plugin and a downloader that returns image bytes
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	dl.On("Download", mock.Anything, mock.Anything).Return([]byte("PNGDATA"), nil)
	p, sink := newTestPlugin(t, msgr, dl, []string{"sender-1@s.whatsapp.net"})
	evt := makeImageMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "look", "image/png")

	// when
	// ... an image message arrives
	p.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... the inbound text contains both caption and attachment tag
	r.Equal(1, sink.count())
	a.Contains(sink.at(0).Text, "<text>look</text>")
	a.Contains(sink.at(0).Text, `mime="image/png"`)
}

func TestPlugin_OversizedAttachment_SkippedWithNotice(t *testing.T) {
	r := require.New(t)

	// given
	// ... a plugin and a downloader that returns an oversized image
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	huge := make([]byte, MaxImageBytes+1)
	dl.On("Download", mock.Anything, mock.Anything).Return(huge, nil)
	msgr.On("SendText", "chat-1@g.us", mock.MatchedBy(func(s string) bool {
		return strings.HasPrefix(s, "skipped (too large):")
	})).Return(nil)
	p, sink := newTestPlugin(t, msgr, dl, []string{"sender-1@s.whatsapp.net"})
	evt := makeImageMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "", "image/png")

	// when
	// ... an oversized image arrives
	p.HandleEvent(evt)
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... nothing is delivered and a skip notice is sent
	r.Equal(0, sink.count())
	msgr.AssertCalled(t, "SendText", "chat-1@g.us", mock.AnythingOfType("string"))
}

func TestPlugin_MixedTextAndAttachmentBatch_OrderPreserved(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a plugin receiving text, image, then text in the same burst
	msgr := &messengerMock{}
	dl := &downloaderMock{}
	dl.On("Download", mock.Anything, mock.Anything).Return([]byte("DATA"), nil)
	p, sink := newTestPlugin(t, msgr, dl, []string{"sender-1@s.whatsapp.net"})

	// when
	// ... three events arrive within the burst window
	p.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "first"))
	p.HandleEvent(makeImageMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "", "image/png"))
	p.HandleEvent(makeMessageEvent("sender-1@s.whatsapp.net", "chat-1@g.us", "third"))
	time.Sleep(testBurstDelay + 200*time.Millisecond)

	// then
	// ... order is preserved in the rendered prompt
	r.Equal(1, sink.count())
	body := sink.at(0).Text
	idx1 := strings.Index(body, "first")
	idx2 := strings.Index(body, "image/png")
	idx3 := strings.Index(body, "third")
	a.True(idx1 >= 0 && idx2 > idx1 && idx3 > idx2, "out of order: %s", body)
}

// --- ExtractText tests (migrated from handler package) ---

func TestExtractText_Conversation(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a plain conversation message
	msg := makeWAMessage("hello world")

	// when
	// ... text is extracted
	text := ExtractText(msg)

	// then
	// ... the conversation text is returned
	a.Equal("hello world", text)
}

func TestExtractText_ExtendedTextMessage(t *testing.T) {
	a := assert.New(t)

	// given
	// ... an extended text message (link preview)
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{
			Text: proto.String("link message"),
		},
	}

	// when
	// ... text is extracted
	text := ExtractText(msg)

	// then
	// ... the extended text is returned
	a.Equal("link message", text)
}

func TestExtractText_Empty(t *testing.T) {
	a := assert.New(t)

	// given
	// ... an empty message
	msg := makeWAMessage("")

	// when
	// ... text is extracted
	text := ExtractText(msg)

	// then
	// ... empty string is returned
	a.Equal("", text)
}
