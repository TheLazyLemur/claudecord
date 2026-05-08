package discord

import (
	"context"
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

func TestPlugin_AtClaudeInPlainChannel_OpensNewThread(t *testing.T) {
	// given
	// ... a plugin bound to a fake discord session and an empty registry
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-1", mock.Anything).Return("thread-new", nil).Once()
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... a MessageCreate event arrives in a plain channel mentioning the bot
	p.handleMessageForTest(messageEvent{
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
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	p.threads.markOwned("thread-existing")
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... an @claude message lands inside that thread
	p.handleMessageForTest(messageEvent{
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
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... an @claude message arrives in a foreign thread
	p.handleMessageForTest(messageEvent{
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
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	called := false
	_ = p.Start(context.Background(), func(in core.Inbound) { called = true })

	// when
	// ... a message without @claude arrives
	p.handleMessageForTest(messageEvent{
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
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... a DM message arrives
	p.handleMessageForTest(messageEvent{
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
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	called := false
	_ = p.Start(context.Background(), func(in core.Inbound) { called = true })

	// when
	// ... a message from user-2 arrives
	p.handleMessageForTest(messageEvent{
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
