package handler

import (
	"log/slog"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

const maxMessageLen = 2000

// DiscordSession abstracts the discordgo.Session methods we need
type DiscordSession interface {
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
	MessageThreadStartComplex(channelID, messageID string, data *discordgo.ThreadStart, options ...discordgo.RequestOption) (*discordgo.Channel, error)
	InteractionRespond(interaction *discordgo.Interaction, resp *discordgo.InteractionResponse, options ...discordgo.RequestOption) error
	MessageReactionAdd(channelID, messageID, emoji string, options ...discordgo.RequestOption) error
}

// DiscordClientWrapper implements core.DiscordClient using discordgo
type DiscordClientWrapper struct {
	session DiscordSession
}

// NewDiscordClientWrapper creates a wrapper around a discordgo session
func NewDiscordClientWrapper(session DiscordSession) *DiscordClientWrapper {
	return &DiscordClientWrapper{session: session}
}

func (c *DiscordClientWrapper) SendMessage(channelID, content string) error {
	_, err := c.session.ChannelMessageSend(channelID, content)
	return errors.Wrap(err, "sending message")
}

func (c *DiscordClientWrapper) SendTyping(channelID string) error {
	return c.session.ChannelTyping(channelID)
}

func (c *DiscordClientWrapper) AddReaction(channelID, messageID, emoji string) error {
	return errors.Wrap(c.session.MessageReactionAdd(channelID, messageID, emoji), "adding reaction")
}

func (c *DiscordClientWrapper) StartThread(channelID, messageID, name string) (string, error) {
	thread, err := c.session.MessageThreadStartComplex(channelID, messageID, &discordgo.ThreadStart{
		Name:                name,
		AutoArchiveDuration: 60,
	})
	if err != nil {
		return "", errors.Wrap(err, "starting thread")
	}
	return thread.ID, nil
}

func (c *DiscordClientWrapper) CreateThread(channelID, content string) (string, error) {
	// post initial message, then create thread from it
	msg, err := c.session.ChannelMessageSend(channelID, "Response (see thread)")
	if err != nil {
		return "", errors.Wrap(err, "sending thread anchor message")
	}

	thread, err := c.session.MessageThreadStartComplex(channelID, msg.ID, &discordgo.ThreadStart{
		Name:                "Response",
		AutoArchiveDuration: 60,
	})
	if err != nil {
		return "", errors.Wrap(err, "creating thread")
	}

	// post actual content in thread (chunked if needed)
	for len(content) > 0 {
		chunk := content
		if len(chunk) > maxMessageLen {
			chunk = content[:maxMessageLen]
			content = content[maxMessageLen:]
		} else {
			content = ""
		}
		_, err = c.session.ChannelMessageSend(thread.ID, chunk)
		if err != nil {
			return "", errors.Wrap(err, "sending thread content")
		}
	}

	return thread.ID, nil
}

// BotInterface defines what the Handler needs from Bot
type BotInterface interface {
	HandleMessage(channelID, messageID, message string) error
	NewSession(workDir string) error
}

// Handler handles Discord events
type Handler struct {
	bot         BotInterface
	botID       string
	allowedUsers []string
}

// NewHandler creates a new Handler
func NewHandler(bot BotInterface, botID string, allowedUsers []string) *Handler {
	return &Handler{
		bot:         bot,
		botID:       botID,
		allowedUsers: allowedUsers,
	}
}

// isUserAllowed checks if a user is in the allowed users list
func (h *Handler) isUserAllowed(userID string) bool {
	for _, allowedID := range h.allowedUsers {
		if allowedID == userID {
			return true
		}
	}
	return false
}

// getInteractionUser extracts user from interaction (guild or DM)
func getInteractionUser(i *discordgo.InteractionCreate) *discordgo.User {
	if i.Interaction.Member != nil {
		return i.Interaction.Member.User
	}
	return i.Interaction.User
}

func (h *Handler) respondError(s DiscordSession, i *discordgo.InteractionCreate, msg string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	}); err != nil {
		slog.Error("responding to interaction", "error", err)
	}
}

// OnMessageCreate handles incoming messages
func (h *Handler) OnMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	slog.Info("message received", "content", m.Content, "author", m.Author.Username, "mentions", len(m.Mentions), "botID", h.botID)

	if m.Author == nil || m.Author.Bot {
		return
	}

	if !h.isUserAllowed(m.Author.ID) {
		slog.Info("unauthorized user attempt", "user_id", m.Author.ID, "username", m.Author.Username)
		return
	}

	msg, ok := ExtractClaudeMention(m.Content, m.Mentions, h.botID)
	slog.Info("mention check", "extracted", msg, "ok", ok)
	if !ok {
		return
	}

	if err := h.bot.HandleMessage(m.ChannelID, m.Message.ID, msg); err != nil {
		slog.Error("handling message", "error", err)
	}
}

// OnInteractionCreate handles slash commands
func (h *Handler) OnInteractionCreate(s DiscordSession, i *discordgo.InteractionCreate) {
	slog.Debug("interaction received", "type", i.Type)

	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data, ok := i.Data.(discordgo.ApplicationCommandInteractionData)
	if !ok {
		return
	}

	slog.Info("slash command", "name", data.Name)

	switch data.Name {
	case "ping":
		// Unrestricted: ping is a health check that doesn't perform any actions
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "pong!",
			},
		}); err != nil {
			slog.Error("responding to ping", "error", err)
		}

	case "new-session":
		user := getInteractionUser(i)
		if user == nil {
			slog.Error("interaction has no member or user", "type", i.Type)
			h.respondError(s, i, "Error: Unable to identify user.")
			return
		}

		if !h.isUserAllowed(user.ID) {
			slog.Info("unauthorized user attempt", "user_id", user.ID, "username", user.Username)
			h.respondError(s, i, "You are not authorized to use this command.")
			return
		}

		var dir string
		for _, opt := range data.Options {
			if opt.Name == "directory" {
				dir = opt.StringValue()
				break
			}
		}

		// respond to interaction first (Discord requires response within 3s)
		msg := "Starting new session..."
		if dir != "" {
			msg = "Starting new session in " + dir + "..."
		}
		if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: msg,
			},
		}); err != nil {
			slog.Error("responding to interaction", "error", err)
			return
		}

		if err := h.bot.NewSession(dir); err != nil {
			slog.Error("creating new session", "error", err)
		}
	}
}

// ExtractClaudeMention checks if message starts with @bot mention and returns the rest
func ExtractClaudeMention(content string, mentions []*discordgo.User, botID string) (string, bool) {
	// check if bot is mentioned
	botMentioned := false
	for _, u := range mentions {
		if u.ID == botID {
			botMentioned = true
			break
		}
	}
	if !botMentioned {
		return "", false
	}

	// check if mention is at start (either <@ID> or <@!ID> format)
	mentionPrefix := "<@" + botID + ">"
	mentionPrefixNick := "<@!" + botID + ">"

	var rest string
	if strings.HasPrefix(content, mentionPrefix) {
		rest = strings.TrimPrefix(content, mentionPrefix)
	} else if strings.HasPrefix(content, mentionPrefixNick) {
		rest = strings.TrimPrefix(content, mentionPrefixNick)
	} else {
		return "", false
	}

	return strings.TrimSpace(rest), true
}

// SlashCommands returns the slash commands to register
func SlashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "new-session",
			Description: "Start a fresh Claude session",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "directory",
					Description: "Working directory (must be under allowed dirs)",
					Required:    false,
				},
			},
		},
		{
			Name:        "ping",
			Description: "Check if bot is responding",
		},
	}
}
