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
	if err != nil {
		return errors.Wrap(err, "sending message")
	}
	return nil
}

func (c *DiscordClientWrapper) SendTyping(channelID string) error {
	return c.session.ChannelTyping(channelID)
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
	HandleMessage(channelID, message string) error
	NewSession() error
}

// Handler handles Discord events
type Handler struct {
	bot   BotInterface
	botID string
}

// NewHandler creates a new Handler
func NewHandler(bot BotInterface, botID string) *Handler {
	return &Handler{bot: bot, botID: botID}
}

// OnMessageCreate handles incoming messages
func (h *Handler) OnMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}

	msg, ok := ExtractClaudeMention(m.Content, m.Mentions, h.botID)
	if !ok {
		return
	}

	if err := h.bot.HandleMessage(m.ChannelID, msg); err != nil {
		slog.Error("handling message", "error", err)
	}
}

// OnInteractionCreate handles slash commands
func (h *Handler) OnInteractionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data, ok := i.Data.(discordgo.ApplicationCommandInteractionData)
	if !ok {
		return
	}

	if data.Name == "new-session" {
		if err := h.bot.NewSession(); err != nil {
			slog.Error("creating new session", "error", err)
		}
		// respond to interaction
		if s != nil {
			s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "New session started",
				},
			})
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
		},
	}
}
