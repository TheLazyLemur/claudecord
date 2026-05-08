package discord

import (
	"context"
	"errors"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/mock"
)

type sessionFull struct {
	discordSessionMock
}

func (s *sessionFull) MessageThreadStartComplex(channelID, messageID string, name string) (string, error) {
	args := s.Called(channelID, messageID, name)
	return args.String(0), args.Error(1)
}

// newTestPlugin constructs a Plugin with a mock session and pre-wired deliver,
// bypassing the discordgo Open path.
func newTestPlugin(s sessionForPlugin, botID string, allowed []string, deliver func(core.Inbound)) *Plugin {
	p := New(Config{BotID: botID, AllowedUsers: allowed}, s)
	_ = p.Start(context.Background(), deliver)
	return p
}

func TestPlugin_AtClaudeInPlainChannel_OpensNewThread(t *testing.T) {
	// given
	// ... a plugin bound to a fake discord session and an empty registry
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-1", mock.Anything).Return("thread-new", nil).Once()
	var got core.Inbound
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { got = in })

	// when
	// ... a MessageCreate event arrives in a plain channel mentioning the bot
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "channel-1",
		MessageID: "msg-1",
		Content:   "@claude do the thing",
		IsThread:  false,
	})

	// then
	// ... a new thread was opened and the SessionKey points at it
	if got.SessionKey != "discord:thread:thread-new" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
	if got.Text != "do the thing" {
		t.Fatalf("text: %q", got.Text)
	}
	s.AssertExpectations(t)
}

func TestPlugin_AtClaudeInOwnedThread_StaysInThread(t *testing.T) {
	// given
	// ... the plugin already owns thread-existing
	s := &sessionFull{}
	var got core.Inbound
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { got = in })
	p.threads.markOwned("thread-existing")

	// when
	// ... an @claude message lands inside that thread
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "thread-existing",
		ParentID:  "channel-1",
		MessageID: "msg-9",
		Content:   "@claude follow up",
		IsThread:  true,
	})

	// then
	// ... no new thread is created and SessionKey reuses the thread id
	s.AssertNotCalled(t, "MessageThreadStartComplex", mock.Anything, mock.Anything, mock.Anything)
	if got.SessionKey != "discord:thread:thread-existing" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
}

func TestPlugin_AtClaudeInForeignThread_OpensSiblingThread(t *testing.T) {
	// given
	// ... the plugin does NOT own this thread
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-7", mock.Anything).Return("thread-sibling", nil).Once()
	var got core.Inbound
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { got = in })

	// when
	// ... an @claude message arrives in a foreign thread
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "thread-foreign",
		ParentID:  "channel-1",
		MessageID: "msg-7",
		Content:   "@claude do it",
		IsThread:  true,
	})

	// then
	// ... a new sibling thread is opened on the parent channel
	if got.SessionKey != "discord:thread:thread-sibling" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
	s.AssertExpectations(t)
}

func TestPlugin_NoMention_Ignored(t *testing.T) {
	// given
	// ... a plugin started with a sink
	s := &sessionFull{}
	called := false
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { called = true })

	// when
	// ... a message without @claude arrives
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "channel-1",
		MessageID: "msg-2",
		Content:   "just chatting",
	})

	// then
	// ... no inbound was delivered
	if called {
		t.Fatalf("expected handler not to deliver for non-@claude message")
	}
}

func TestPlugin_DM_UsesDMSessionKey(t *testing.T) {
	// given
	// ... a plugin and a DM event
	s := &sessionFull{}
	var got core.Inbound
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { got = in })

	// when
	// ... a DM message arrives
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "dm-channel",
		MessageID: "msg-3",
		Content:   "@claude hello",
		IsDM:      true,
	})

	// then
	// ... the SessionKey is dm-prefixed and uses the user id
	if got.SessionKey != "discord:dm:user-1" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
}

func TestPlugin_DisallowedUser_Ignored(t *testing.T) {
	// given
	// ... a plugin where user-2 is not allowed
	s := &sessionFull{}
	called := false
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { called = true })

	// when
	// ... a message from user-2 arrives
	p.handleMessage(messageEvent{
		AuthorID:  "user-2",
		ChannelID: "channel-1",
		MessageID: "msg-4",
		Content:   "@claude hello",
	})

	// then
	// ... no inbound was delivered
	if called {
		t.Fatalf("expected message from disallowed user to be ignored")
	}
}

func TestPlugin_Inbound_CapabilitiesMatchPluginCapabilities(t *testing.T) {
	// given
	// ... a plugin and a captured inbound
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-1", mock.Anything).Return("thread-new", nil).Once()
	var got core.Inbound
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { got = in })

	// when
	// ... a message arrives and an inbound is dispatched
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "channel-1",
		MessageID: "msg-1",
		Content:   "@claude do the thing",
		IsThread:  false,
	})

	// then
	// ... the inbound's Capabilities exactly match p.Capabilities()
	if got.Capabilities != p.Capabilities() {
		t.Fatalf("capabilities mismatch: inbound=%+v plugin=%+v", got.Capabilities, p.Capabilities())
	}
}

func TestPlugin_ThreadCreateError_DropsMessage(t *testing.T) {
	// given
	// ... a session where thread creation fails
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-5", mock.Anything).Return("", errors.New("discord error")).Once()
	s.On("MessageReactionAdd", "channel-1", "msg-5", "❌").Return(nil).Once()
	delivered := false
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { delivered = true })

	// when
	// ... an @claude message arrives in a plain channel
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "channel-1",
		MessageID: "msg-5",
		Content:   "@claude hello",
		IsThread:  false,
	})

	// then
	// ... no inbound was delivered and a ❌ reaction was added to the original message
	if delivered {
		t.Fatalf("expected message to be dropped when thread creation fails")
	}
	s.AssertExpectations(t)
}
