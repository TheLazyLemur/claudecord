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
	sendErr               error
	threadErr             error
	threadMsgErr          error
	interactionErr        error
	createdThreadID       string
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
	message   string
}

func (m *mockBot) HandleMessage(channelID, message string) error {
	m.handledMessages = append(m.handledMessages, handledMsg{channelID, message})
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

// --- Tests: Handler event handling ---

func TestHandler_OnMessageCreate_TriggersBot(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123")

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
	a.Equal("do something", bot.handledMessages[0].message)
}

func TestHandler_OnMessageCreate_IgnoresBotMessages(t *testing.T) {
	// given
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123")

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
	h := NewHandler(bot, "bot-123")

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

func TestHandler_OnNewSession_CallsBot(t *testing.T) {
	a := assert.New(t)

	// given
	session := &mockSession{}
	bot := &mockBot{}
	h := NewHandler(bot, "bot-123")

	interaction := &discordgo.InteractionCreate{
		Interaction: &discordgo.Interaction{
			Type: discordgo.InteractionApplicationCommand,
			Data: discordgo.ApplicationCommandInteractionData{
				Name: "new-session",
			},
		},
	}

	// when
	h.OnInteractionCreate(session, interaction)

	// then
	a.Equal(1, bot.newSessionCalls)
	a.Len(session.interactionResponses, 1)
}
