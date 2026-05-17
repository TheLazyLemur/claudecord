package discord

import (
	"errors"
	"testing"

	"github.com/bwmarrin/discordgo"
)

func TestTranslate_BotsOwnMessage_Skipped(t *testing.T) {
	// given
	// ... a MessageCreate from the bot itself
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:    &discordgo.User{ID: "bot-id"},
			ChannelID: "ch-1",
			ID:        "msg-1",
			Content:   "@switchboard hello",
		},
	}

	// when
	// ... translateMessageCreate is called with matching botID
	_, ok := translateMessageCreate(m, "bot-id", nil)

	// then
	// ... the event is skipped
	if ok {
		t.Fatal("expected bot's own message to be skipped")
	}
}

func TestTranslate_NoAuthor_Skipped(t *testing.T) {
	// given
	// ... a MessageCreate with a nil Author
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:    nil,
			ChannelID: "ch-1",
			ID:        "msg-2",
			Content:   "@switchboard hello",
		},
	}

	// when
	// ... translateMessageCreate is called
	_, ok := translateMessageCreate(m, "bot-id", nil)

	// then
	// ... the event is skipped
	if ok {
		t.Fatal("expected nil-author message to be skipped")
	}
}

func TestTranslate_DMHasIsDMTrue(t *testing.T) {
	// given
	// ... a MessageCreate with GuildID == "" (DM)
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:    &discordgo.User{ID: "user-1"},
			ChannelID: "dm-ch",
			ID:        "msg-3",
			Content:   "@switchboard hi",
			GuildID:   "",
		},
	}

	// when
	// ... translateMessageCreate is called with no lookupChannel
	ev, ok := translateMessageCreate(m, "bot-id", nil)

	// then
	// ... IsDM is true and fields are populated correctly
	if !ok {
		t.Fatal("expected DM message to produce an event")
	}
	if !ev.IsDM {
		t.Fatalf("expected IsDM == true, got false")
	}
	if ev.AuthorID != "user-1" {
		t.Fatalf("AuthorID: got %q, want %q", ev.AuthorID, "user-1")
	}
	if ev.ChannelID != "dm-ch" {
		t.Fatalf("ChannelID: got %q, want %q", ev.ChannelID, "dm-ch")
	}
}

func TestTranslate_ThreadLookupSetsParentID(t *testing.T) {
	// given
	// ... a MessageCreate in a guild channel where lookupChannel returns a thread
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:    &discordgo.User{ID: "user-1"},
			ChannelID: "thread-ch",
			ID:        "msg-4",
			Content:   "@switchboard go",
			GuildID:   "guild-1",
		},
	}
	lookup := func(id string) (*discordgo.Channel, error) {
		return &discordgo.Channel{
			ID:       id,
			ParentID: "parent-ch",
			Type:     discordgo.ChannelTypeGuildPublicThread,
		}, nil
	}

	// when
	// ... translateMessageCreate is called with the thread-returning lookup
	ev, ok := translateMessageCreate(m, "bot-id", lookup)

	// then
	// ... IsThread is true and ParentID is set
	if !ok {
		t.Fatal("expected thread message to produce an event")
	}
	if !ev.IsThread {
		t.Fatalf("expected IsThread == true, got false")
	}
	if ev.ParentID != "parent-ch" {
		t.Fatalf("ParentID: got %q, want %q", ev.ParentID, "parent-ch")
	}
}

func TestTranslate_NonThreadChannel_NoParent(t *testing.T) {
	// given
	// ... a MessageCreate in a plain guild channel where lookup returns a non-thread channel
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			Author:    &discordgo.User{ID: "user-1"},
			ChannelID: "plain-ch",
			ID:        "msg-5",
			Content:   "@switchboard hey",
			GuildID:   "guild-1",
		},
	}
	lookup := func(id string) (*discordgo.Channel, error) {
		return nil, errors.New("not in cache")
	}

	// when
	// ... translateMessageCreate is called with a lookup that fails
	ev, ok := translateMessageCreate(m, "bot-id", lookup)

	// then
	// ... IsThread is false and ParentID is empty
	if !ok {
		t.Fatal("expected plain channel message to produce an event")
	}
	if ev.IsThread {
		t.Fatalf("expected IsThread == false, got true")
	}
	if ev.ParentID != "" {
		t.Fatalf("expected empty ParentID, got %q", ev.ParentID)
	}
}
