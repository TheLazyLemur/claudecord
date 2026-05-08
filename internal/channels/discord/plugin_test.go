package discord

import (
	"context"
	"errors"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
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

// newTestPluginWithMedia constructs a Plugin that includes MediaDir + Downloader.
func newTestPluginWithMedia(s sessionForPlugin, botID string, allowed []string, mediaDir string, dl Downloader, deliver func(core.Inbound)) *Plugin {
	p := New(Config{BotID: botID, AllowedUsers: allowed, MediaDir: mediaDir, Downloader: dl}, s)
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
		Content:   "<@bot-id> do the thing",
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
		Content:   "<@bot-id> follow up",
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
		Content:   "<@bot-id> do it",
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
		Content:   "<@bot-id> hello",
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
		Content:   "<@bot-id> hello",
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
		Content:   "<@bot-id> do the thing",
		IsThread:  false,
	})

	// then
	// ... the inbound's Capabilities exactly match p.Capabilities()
	if got.Capabilities != p.Capabilities() {
		t.Fatalf("capabilities mismatch: inbound=%+v plugin=%+v", got.Capabilities, p.Capabilities())
	}
}

func TestPlugin_Capabilities_UpdatesTrue(t *testing.T) {
	// given
	// ... a plugin with no media dir
	p := New(Config{}, nil)

	// when
	// ... capabilities are queried
	caps := p.Capabilities()

	// then
	// ... reactions yes, updates yes
	if !caps.Reactions {
		t.Fatalf("expected Reactions=true, got %+v", caps)
	}
	if !caps.Updates {
		t.Fatalf("expected Updates=true, got %+v", caps)
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
		Content:   "<@bot-id> hello",
		IsThread:  false,
	})

	// then
	// ... no inbound was delivered and a ❌ reaction was added to the original message
	if delivered {
		t.Fatalf("expected message to be dropped when thread creation fails")
	}
	s.AssertExpectations(t)
}

func TestPlugin_HandleMessage_PopulatesAttachments(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a plugin with a media dir and a stub downloader returning image bytes
	dir := t.TempDir()
	s := &sessionFull{}
	payload := []byte("PNGDATA")
	dl := &stubDownloader{body: payload}
	var got core.Inbound
	p := newTestPluginWithMedia(s, "bot-id", []string{"user-1"}, dir, dl, func(in core.Inbound) { got = in })
	p.threads.markOwned("thread-1")

	// when
	// ... an @claude message with an attachment arrives in an owned thread
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "thread-1",
		MessageID: "msg-1",
		Content:   "<@bot-id> look at this",
		IsThread:  true,
		Attachments: []*discordgo.MessageAttachment{
			{
				ID:          "att-1",
				Filename:    "photo.png",
				ContentType: "image/png",
				URL:         "https://cdn.discordapp.com/photo.png",
			},
		},
	})

	// then
	// ... the inbound has one AttachmentRef with correct metadata
	r.Len(got.Attachments, 1)
	a.Equal("image/png", got.Attachments[0].MIME)
	a.Equal("photo.png", got.Attachments[0].OriginalName)
	a.NotEmpty(got.Attachments[0].Path)
}

func TestPlugin_HandleMessage_SkippedAttachmentPrependedToText(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a plugin with a stub downloader returning an oversized image
	dir := t.TempDir()
	s := &sessionFull{}
	oversized := make([]byte, MaxImageBytes+1)
	dl := &stubDownloader{body: oversized}
	var got core.Inbound
	p := newTestPluginWithMedia(s, "bot-id", []string{"user-1"}, dir, dl, func(in core.Inbound) { got = in })
	p.threads.markOwned("thread-1")

	// when
	// ... an @claude message with an oversized attachment arrives
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "thread-1",
		MessageID: "msg-2",
		Content:   "<@bot-id> check this",
		IsThread:  true,
		Attachments: []*discordgo.MessageAttachment{
			{
				ID:          "att-2",
				Filename:    "huge.png",
				ContentType: "image/png",
				URL:         "https://cdn.discordapp.com/huge.png",
			},
		},
	})

	// then
	// ... the inbound text includes the skipped notice and no attachments
	a.Empty(got.Attachments)
	a.Contains(got.Text, "skipped (too large): huge.png")
}

func TestPlugin_HandleMessage_NoMediaDir_NoAttachmentsProcessed(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a plugin with no MediaDir configured (attachment processing disabled)
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-3", mock.Anything).Return("thread-new", nil).Once()
	var got core.Inbound
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { got = in })

	// when
	// ... an @claude message with an attachment arrives (MediaDir is empty)
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "channel-1",
		MessageID: "msg-3",
		Content:   "<@bot-id> hi",
		IsThread:  false,
		Attachments: []*discordgo.MessageAttachment{
			{ID: "att-3", Filename: "file.png", ContentType: "image/png", URL: "https://cdn/file.png"},
		},
	})

	// then
	// ... attachments are silently skipped and the inbound has no refs
	a.Empty(got.Attachments)
	s.AssertExpectations(t)
}

func TestStripMention_AcceptsBotMention(t *testing.T) {
	// given
	// ... a bot ID and a canonical Discord mention prefix
	botID := "123456789"
	content := "<@123456789> do the thing"

	// when
	// ... stripMention is called
	result, ok := stripMention(content, botID)

	// then
	// ... the mention is stripped and the remainder returned
	if !ok {
		t.Fatalf("expected ok=true for canonical mention")
	}
	if result != "do the thing" {
		t.Fatalf("text: %q", result)
	}
}

func TestStripMention_AcceptsBangMention(t *testing.T) {
	// given
	// ... a bot ID and a bang-style Discord mention prefix
	botID := "123456789"
	content := "<@!123456789> follow up"

	// when
	// ... stripMention is called
	result, ok := stripMention(content, botID)

	// then
	// ... the mention is stripped and the remainder returned
	if !ok {
		t.Fatalf("expected ok=true for bang mention")
	}
	if result != "follow up" {
		t.Fatalf("text: %q", result)
	}
}

func TestStripMention_RejectsLiteralAtClaude(t *testing.T) {
	// given
	// ... a bot ID and a literal @claude string (not a real Discord mention)
	botID := "123456789"
	content := "@claude do the thing"

	// when
	// ... stripMention is called
	_, ok := stripMention(content, botID)

	// then
	// ... the check fails because the format is wrong
	if ok {
		t.Fatalf("expected ok=false for literal @claude")
	}
}

func TestStripMention_RejectsNoMention(t *testing.T) {
	// given
	// ... a bot ID and a plain message with no mention
	botID := "123456789"
	content := "just chatting"

	// when
	// ... stripMention is called
	_, ok := stripMention(content, botID)

	// then
	// ... the check fails
	if ok {
		t.Fatalf("expected ok=false for message with no mention")
	}
}

func TestPlugin_OwnedThread_StateCacheRace_StillStaysInThread(t *testing.T) {
	// given
	// ... the plugin already owns thread-existing, but IsThread is false (State cache stale)
	s := &sessionFull{}
	var got core.Inbound
	p := newTestPlugin(s, "bot-id", []string{"user-1"}, func(in core.Inbound) { got = in })
	p.threads.markOwned("thread-existing")

	// when
	// ... an @claude message arrives in that thread with IsThread=false due to State cache race
	p.handleMessage(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "thread-existing",
		MessageID: "msg-10",
		Content:   "<@bot-id> follow up",
		IsThread:  false,
	})

	// then
	// ... no new thread is created and SessionKey reuses the thread id
	s.AssertNotCalled(t, "MessageThreadStartComplex", mock.Anything, mock.Anything, mock.Anything)
	if got.SessionKey != "discord:thread:thread-existing" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
}

func TestTranslate_AttachmentsPopulated(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a MessageCreate with one attachment
	att := &discordgo.MessageAttachment{
		ID:          "att-1",
		Filename:    "image.png",
		ContentType: "image/png",
		URL:         "https://cdn.discordapp.com/image.png",
	}
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:      &discordgo.User{ID: "user-1"},
			ChannelID:   "ch-1",
			ID:          "msg-1",
			Content:     "<@bot-id> hello",
			GuildID:     "guild-1",
			Attachments: []*discordgo.MessageAttachment{att},
		},
	}

	// when
	// ... translateMessageCreate is called
	ev, ok := translateMessageCreate(m, "bot-id", nil)

	// then
	// ... the event carries the attachment
	a.True(ok)
	a.Len(ev.Attachments, 1)
	a.Equal("image.png", ev.Attachments[0].Filename)
}
