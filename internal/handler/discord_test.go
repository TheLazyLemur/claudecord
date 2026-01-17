package handler

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock discordgo session ---

type mockSession struct {
	sentMessages          []sentMsg
	typingChannels        []string
	createdThreads        []threadCreate
	threadMessages        []sentMsg
	interactionResponses  []*discordgo.InteractionResponse
	addedReactions        []reactionAdd
	sendErr               error
	threadErr             error
	threadMsgErr          error
	interactionErr        error
	reactionErr           error
	createdThreadID       string
}

type reactionAdd struct {
	channelID string
	messageID string
	emoji     string
}

type sentMsg struct {
	channelID string
	content   string
}

type threadCreate struct {
	channelID string
	messageID string
	name      string
}

func (m *mockSession) ChannelMessageSend(channelID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.sentMessages = append(m.sentMessages, sentMsg{channelID, content})
	return &discordgo.Message{ID: "msg-123", ChannelID: channelID}, m.sendErr
}

func (m *mockSession) ChannelTyping(channelID string, _ ...discordgo.RequestOption) error {
	m.typingChannels = append(m.typingChannels, channelID)
	return nil
}

func (m *mockSession) MessageThreadStartComplex(channelID, messageID string, data *discordgo.ThreadStart, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	m.createdThreads = append(m.createdThreads, threadCreate{channelID, messageID, data.Name})
	return &discordgo.Channel{ID: m.createdThreadID}, m.threadErr
}

func (m *mockSession) InteractionRespond(_ *discordgo.Interaction, resp *discordgo.InteractionResponse, _ ...discordgo.RequestOption) error {
	m.interactionResponses = append(m.interactionResponses, resp)
	return m.interactionErr
}

func (m *mockSession) MessageReactionAdd(channelID, messageID, emoji string, _ ...discordgo.RequestOption) error {
	m.addedReactions = append(m.addedReactions, reactionAdd{channelID, messageID, emoji})
	return m.reactionErr
}

// --- Mock Bot ---

type mockBot struct {
	handledMessages   []handledMsg
	newSessionCalls   int
	lastNewSessionDir string
	handleErr         error
	newSessionErr     error
}

type handledMsg struct {
	channelID string
	messageID string
	message   string
}

func (m *mockBot) HandleMessage(channelID, messageID, message string) error {
	m.handledMessages = append(m.handledMessages, handledMsg{channelID, messageID, message})
	return m.handleErr
}

func (m *mockBot) NewSession(workDir string) error {
	m.newSessionCalls++
	m.lastNewSessionDir = workDir
	return m.newSessionErr
}

// --- Tests: DiscordClientWrapper ---

func TestClientWrapper_SendMessage(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	session := &mockSession{}
	client := NewDiscordClientWrapper(session)

	// when
	err := client.SendMessage("chan-1", "hello world")

	// then
	r.NoError(err)
	r.Len(session.sentMessages, 1)
	a.Equal("chan-1", session.sentMessages[0].channelID)
	a.Equal("hello world", session.sentMessages[0].content)
}

func TestClientWrapper_SendTyping(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	session := &mockSession{}
	client := NewDiscordClientWrapper(session)

	// when
	err := client.SendTyping("chan-2")

	// then
	r.NoError(err)
	a.Contains(session.typingChannels, "chan-2")
}

func TestClientWrapper_CreateThread(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	session := &mockSession{createdThreadID: "thread-123"}
	client := NewDiscordClientWrapper(session)

	// when
	threadID, err := client.CreateThread("chan-3", "long response content here")

	// then
	r.NoError(err)
	a.Equal("thread-123", threadID)
	// should send anchor message then thread content
	r.Len(session.sentMessages, 2)
	a.Equal("chan-3", session.sentMessages[0].channelID)
	a.Equal("thread-123", session.sentMessages[1].channelID)
	a.Equal("long response content here", session.sentMessages[1].content)
	// thread created from anchor message
	r.Len(session.createdThreads, 1)
	a.Equal("chan-3", session.createdThreads[0].channelID)
	a.Equal("msg-123", session.createdThreads[0].messageID)
}

func TestClientWrapper_CreateThread_ChunksLongContent(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	session := &mockSession{createdThreadID: "thread-123"}
	client := NewDiscordClientWrapper(session)
	// content > 2000 chars
	longContent := make([]byte, 2500)
	for i := range longContent {
		longContent[i] = 'x'
	}

	// when
	threadID, err := client.CreateThread("chan-3", string(longContent))

	// then
	r.NoError(err)
	a.Equal("thread-123", threadID)
	// should send anchor + 2 chunks
	r.Len(session.sentMessages, 3)
	a.Equal(2000, len(session.sentMessages[1].content))
	a.Equal(500, len(session.sentMessages[2].content))
}

func TestClientWrapper_AddReaction(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	session := &mockSession{}
	client := NewDiscordClientWrapper(session)

	// when
	err := client.AddReaction("chan-1", "msg-123", "👍")

	// then
	r.NoError(err)
	r.Len(session.addedReactions, 1)
	a.Equal("chan-1", session.addedReactions[0].channelID)
	a.Equal("msg-123", session.addedReactions[0].messageID)
	a.Equal("👍", session.addedReactions[0].emoji)
}

func TestClientWrapper_AddReaction_Error(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{reactionErr: assert.AnError}
	client := NewDiscordClientWrapper(session)

	// when
	err := client.AddReaction("chan-1", "msg-123", "👍")

	// then
	a.Error(err)
	a.Contains(err.Error(), "adding reaction")
}

// --- Tests: Message detection ---

func TestExtractClaudeMention_ValidMention(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		mentions []*discordgo.User
		botID    string
		wantMsg  string
		wantOK   bool
	}{
		{
			name:     "mention at start",
			content:  "<@123456> hello world",
			mentions: []*discordgo.User{{ID: "123456"}},
			botID:    "123456",
			wantMsg:  "hello world",
			wantOK:   true,
		},
		{
			name:     "mention with nickname format",
			content:  "<@!123456> test message",
			mentions: []*discordgo.User{{ID: "123456"}},
			botID:    "123456",
			wantMsg:  "test message",
			wantOK:   true,
		},
		{
			name:     "mention not at start",
			content:  "hey <@123456> do something",
			mentions: []*discordgo.User{{ID: "123456"}},
			botID:    "123456",
			wantMsg:  "",
			wantOK:   false,
		},
		{
			name:     "no mention",
			content:  "hello world",
			mentions: nil,
			botID:    "123456",
			wantMsg:  "",
			wantOK:   false,
		},
		{
			name:     "mention wrong user",
			content:  "<@999999> hello",
			mentions: []*discordgo.User{{ID: "999999"}},
			botID:    "123456",
			wantMsg:  "",
			wantOK:   false,
		},
		{
			name:     "multiple mentions, bot first",
			content:  "<@123456> <@999999> hi",
			mentions: []*discordgo.User{{ID: "123456"}, {ID: "999999"}},
			botID:    "123456",
			wantMsg:  "<@999999> hi",
			wantOK:   true,
		},
		{
			name:     "empty message after mention",
			content:  "<@123456>",
			mentions: []*discordgo.User{{ID: "123456"}},
			botID:    "123456",
			wantMsg:  "",
			wantOK:   true,
		},
		{
			name:     "whitespace after mention",
			content:  "<@123456>   ",
			mentions: []*discordgo.User{{ID: "123456"}},
			botID:    "123456",
			wantMsg:  "",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := assert.New(t)
			msg, ok := ExtractClaudeMention(tt.content, tt.mentions, tt.botID)
			a.Equal(tt.wantOK, ok)
			a.Equal(tt.wantMsg, msg)
		})
	}
}

// --- Tests: Handler authorization ---

func TestHandler_isUserAllowed_AllowedUser(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}
	allowedUsers := []string{"user-123", "user-456"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	// when
	result := h.isUserAllowed("user-123")

	// then
	a.True(result)
}

func TestHandler_isUserAllowed_DisallowedUser(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}
	allowedUsers := []string{"user-123", "user-456"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	// when
	result := h.isUserAllowed("user-789")

	// then
	a.False(result)
}

func TestHandler_isUserAllowed_EmptyAllowList(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", []string{})

	// when
	result := h.isUserAllowed("user-123")

	// then
	a.False(result) // empty list denies all
}

func TestHandler_isUserAllowed_NilAllowList(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", nil)

	// when
	result := h.isUserAllowed("user-123")

	// then
	a.False(result) // nil list denies all
}

// --- Tests: Handler event handling ---

func TestHandler_OnMessageCreate_TriggersBot(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", []string{"user-1"})

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   "<@bot-123> do something",
			Author:    &discordgo.User{ID: "user-1"},
			Mentions:  []*discordgo.User{{ID: "bot-123"}},
		},
	}

	// when
	h.OnMessageCreate(nil, msg)

	// then
	r.Len(bot.handledMessages, 1)
	a.Equal("chan-1", bot.handledMessages[0].channelID)
	a.Equal("msg-1", bot.handledMessages[0].messageID)
	a.Equal("do something", bot.handledMessages[0].message)
}

func TestHandler_OnMessageCreate_IgnoresBotMessages(t *testing.T) {
	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", []string{"any-user"})

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   "<@bot-123> hello",
			Author:    &discordgo.User{ID: "bot-123", Bot: true},
			Mentions:  []*discordgo.User{{ID: "bot-123"}},
		},
	}

	// when
	h.OnMessageCreate(nil, msg)

	// then
	assert.Len(t, bot.handledMessages, 0)
}

func TestHandler_OnMessageCreate_IgnoresNoMention(t *testing.T) {
	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", []string{"user-1"})

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   "hello world",
			Author:    &discordgo.User{ID: "user-1"},
		},
	}

	// when
	h.OnMessageCreate(nil, msg)

	// then
	assert.Len(t, bot.handledMessages, 0)
}

func TestHandler_OnMessageCreate_AllowedUser_Processes(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	bot := &mockBot{}
	allowedUsers := []string{"user-123", "user-456"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   "<@bot-123> do something",
			Author:    &discordgo.User{ID: "user-123", Username: "allowed_user"},
			Mentions:  []*discordgo.User{{ID: "bot-123"}},
		},
	}

	// when
	h.OnMessageCreate(nil, msg)

	// then
	r.Len(bot.handledMessages, 1)
	a.Equal("chan-1", bot.handledMessages[0].channelID)
	a.Equal("msg-1", bot.handledMessages[0].messageID)
	a.Equal("do something", bot.handledMessages[0].message)
}

func TestHandler_OnMessageCreate_DisallowedUser_Ignored(t *testing.T) {
	// given
	bot := &mockBot{}
	allowedUsers := []string{"user-123", "user-456"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   "<@bot-123> do something",
			Author:    &discordgo.User{ID: "user-789", Username: "disallowed_user"},
			Mentions:  []*discordgo.User{{ID: "bot-123"}},
		},
	}

	// when
	h.OnMessageCreate(nil, msg)

	// then
	assert.Len(t, bot.handledMessages, 0)
}

func TestHandler_OnMessageCreate_EmptyAllowList_DeniesAll(t *testing.T) {
	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", []string{})

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   "<@bot-123> do something",
			Author:    &discordgo.User{ID: "user-999", Username: "any_user"},
			Mentions:  []*discordgo.User{{ID: "bot-123"}},
		},
	}

	// when
	h.OnMessageCreate(nil, msg)

	// then
	assert.Len(t, bot.handledMessages, 0) // empty list denies all
}

func TestHandler_OnMessageCreate_AllowedUserButNotMentioned_Ignored(t *testing.T) {
	r := require.New(t)

	// given
	bot := &mockBot{}
	allowedUsers := []string{"user-123", "user-456"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	msg := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-1",
			ChannelID: "chan-1",
			Content:   "do something",
			Author:    &discordgo.User{ID: "user-123", Username: "allowed_user"},
		},
	}

	// when
	h.OnMessageCreate(nil, msg)

	// then
	r.Len(bot.handledMessages, 0)
}

func TestHandler_NewHandler_WithAllowedUsers(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}
	allowedUsers := []string{"user-123", "user-456"}

	// when
	h := NewHandler(bot, "bot-123", allowedUsers)

	// then
	a.Equal(bot, h.bot)
	a.Equal("bot-123", h.botID)
	a.Equal(allowedUsers, h.allowedUsers)
}

func TestHandler_NewHandler_WithoutAllowedUsers(t *testing.T) {
	a := assert.New(t)

	// given
	bot := &mockBot{}

	// when
	h := NewHandler(bot, "bot-123", []string{})

	// then
	a.Equal(bot, h.bot)
	a.Equal("bot-123", h.botID)
	a.Empty(h.allowedUsers)
}

func TestHandler_OnNewSession_CallsBot(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", []string{"user-123"})

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
			},
			Member: &discordgo.Member{
				User: &discordgo.User{ID: "user-123", Username: "test_user"},
			},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(1, bot.newSessionCalls)
	a.Len(session.interactionResponses, 1)
}

func TestHandler_OnInteractionCreate_NewSession_AllowedUser(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	allowedUsers := []string{"user-123"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
			},
			Member: &discordgo.Member{
				User: &discordgo.User{ID: "user-123", Username: "allowed_user"},
			},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(1, bot.newSessionCalls)
	a.Equal("", bot.lastNewSessionDir)
	a.Len(session.interactionResponses, 1)
	a.Equal("Starting new session...", session.interactionResponses[0].Data.Content)
}

func TestHandler_OnInteractionCreate_NewSession_DisallowedUser(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	allowedUsers := []string{"user-456"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
			},
			Member: &discordgo.Member{
				User: &discordgo.User{ID: "user-123", Username: "disallowed_user"},
			},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(0, bot.newSessionCalls)
	a.Len(session.interactionResponses, 1)
	a.Equal("You are not authorized to use this command.", session.interactionResponses[0].Data.Content)
}

func TestHandler_OnInteractionCreate_NewSession_EmptyAllowList_DeniesAll(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123", []string{})

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
			},
			Member: &discordgo.Member{
				User: &discordgo.User{ID: "user-999", Username: "any_user"},
			},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(0, bot.newSessionCalls) // empty list denies all
	a.Len(session.interactionResponses, 1)
	a.Equal("You are not authorized to use this command.", session.interactionResponses[0].Data.Content)
}

func TestHandler_OnInteractionCreate_NewSession_DM_AllowedUser(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	allowedUsers := []string{"user-123"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
			},
			User: &discordgo.User{ID: "user-123", Username: "allowed_user_dm"},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(1, bot.newSessionCalls)
	a.Equal("", bot.lastNewSessionDir)
	a.Len(session.interactionResponses, 1)
	a.Equal("Starting new session...", session.interactionResponses[0].Data.Content)
}

func TestHandler_OnInteractionCreate_NewSession_DM_DisallowedUser(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	allowedUsers := []string{"user-456"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
			},
			User: &discordgo.User{ID: "user-123", Username: "disallowed_user_dm"},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(0, bot.newSessionCalls)
	a.Len(session.interactionResponses, 1)
	a.Equal("You are not authorized to use this command.", session.interactionResponses[0].Data.Content)
}

func TestHandler_OnInteractionCreate_NewSession_WithDirectory_AllowedUser(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	allowedUsers := []string{"user-123"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
				Options: []*discordgo.ApplicationCommandInteractionDataOption{
					{
						Name:  "directory",
						Type:  discordgo.ApplicationCommandOptionString,
						Value: "/some/directory",
					},
				},
			},
			Member: &discordgo.Member{
				User: &discordgo.User{ID: "user-123", Username: "allowed_user"},
			},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(1, bot.newSessionCalls)
	a.Equal("/some/directory", bot.lastNewSessionDir)
	a.Len(session.interactionResponses, 1)
	a.Equal("Starting new session in /some/directory...", session.interactionResponses[0].Data.Content)
}

func TestHandler_OnInteractionCreate_Ping_Unrestricted(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	allowedUsers := []string{"user-123"}
	h := NewHandler(bot, "bot-123", allowedUsers)

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "ping",
			},
			Member: &discordgo.Member{
				User: &discordgo.User{ID: "user-999", Username: "disallowed_user"},
			},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(0, bot.newSessionCalls)
	a.Len(session.interactionResponses, 1)
	a.Equal("pong!", session.interactionResponses[0].Data.Content)
}
