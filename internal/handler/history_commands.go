package handler

import (
	"fmt"
	"strings"

	"log/slog"

	"github.com/TheLazyLemur/claudecord/internal/history"
	"github.com/bwmarrin/discordgo"
)

// HistoryBotInterface extends BotInterface with history operations
type HistoryBotInterface interface {
	BotInterface
	ListSessions() ([]*history.Session, error)
	ResumeSession(sessionID string) error
	DeleteSession(sessionID string) error
	GetCurrentSessionID() string
}

// HistorySlashCommands returns additional slash commands for session history
func HistorySlashCommands() []*discordgo.ApplicationCommand {
	return []*discordgo.ApplicationCommand{
		{
			Name:        "list-sessions",
			Description: "List all saved conversation sessions",
		},
		{
			Name:        "resume-session",
			Description: "Resume a previous conversation session",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "session_id",
					Description: "The session ID to resume (first 8+ characters)",
					Required:    true,
				},
			},
		},
		{
			Name:        "delete-session",
			Description: "Delete a saved conversation session",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "session_id",
					Description: "The session ID to delete (first 8+ characters)",
					Required:    true,
				},
			},
		},
		{
			Name:        "current-session",
			Description: "Show the current session ID",
		},
	}
}

// HandleHistoryCommand processes history-related slash commands
// Returns true if the command was handled
func (h *Handler) HandleHistoryCommand(s DiscordSession, i *discordgo.InteractionCreate, data discordgo.ApplicationCommandInteractionData) bool {
	historyBot, ok := h.bot.(HistoryBotInterface)
	if !ok {
		// History not enabled
		return false
	}

	switch data.Name {
	case "list-sessions":
		h.handleListSessions(s, i, historyBot)
		return true
	case "resume-session":
		h.handleResumeSession(s, i, historyBot, data.Options)
		return true
	case "delete-session":
		h.handleDeleteSession(s, i, historyBot, data.Options)
		return true
	case "current-session":
		h.handleCurrentSession(s, i, historyBot)
		return true
	}

	return false
}

func (h *Handler) handleListSessions(s DiscordSession, i *discordgo.InteractionCreate, bot HistoryBotInterface) {
	user := getInteractionUser(i)
	if user == nil {
		h.respondError(s, i, "Error: Unable to identify user.")
		return
	}

	if !h.isUserAllowed(user.ID) {
		h.respondError(s, i, "You are not authorized to use this command.")
		return
	}

	sessions, err := bot.ListSessions()
	if err != nil {
		h.respondError(s, i, fmt.Sprintf("Error listing sessions: %v", err))
		return
	}

	if len(sessions) == 0 {
		h.respondSuccess(s, i, "No saved sessions found.")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("**Saved Sessions** (%d total):\n\n", len(sessions)))
	for _, sess := range sessions {
		sb.WriteString("• " + sess.Summary() + "\n")
	}

	h.respondSuccess(s, i, sb.String())
}

func (h *Handler) handleResumeSession(s DiscordSession, i *discordgo.InteractionCreate, bot HistoryBotInterface, options []*discordgo.ApplicationCommandInteractionDataOption) {
	user := getInteractionUser(i)
	if user == nil {
		h.respondError(s, i, "Error: Unable to identify user.")
		return
	}

	if !h.isUserAllowed(user.ID) {
		h.respondError(s, i, "You are not authorized to use this command.")
		return
	}

	var sessionID string
	for _, opt := range options {
		if opt.Name == "session_id" {
			sessionID = opt.StringValue()
			break
		}
	}

	if sessionID == "" {
		h.respondError(s, i, "Session ID is required.")
		return
	}

	// Respond first
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("Resuming session %s...", sessionID),
		},
	}); err != nil {
		slog.Error("responding to interaction", "error", err)
		return
	}

	// Then resume
	if err := bot.ResumeSession(sessionID); err != nil {
		// Send error as follow-up
		if _, err := s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("Error resuming session: %v", err)); err != nil {
			slog.Error("sending error message", "error", err)
		}
		return
	}

	// Success follow-up
	if _, err := s.ChannelMessageSend(i.ChannelID, fmt.Sprintf("✅ Resumed session %s", sessionID)); err != nil {
		slog.Error("sending success message", "error", err)
	}
}

func (h *Handler) handleDeleteSession(s DiscordSession, i *discordgo.InteractionCreate, bot HistoryBotInterface, options []*discordgo.ApplicationCommandInteractionDataOption) {
	user := getInteractionUser(i)
	if user == nil {
		h.respondError(s, i, "Error: Unable to identify user.")
		return
	}

	if !h.isUserAllowed(user.ID) {
		h.respondError(s, i, "You are not authorized to use this command.")
		return
	}

	var sessionID string
	for _, opt := range options {
		if opt.Name == "session_id" {
			sessionID = opt.StringValue()
			break
		}
	}

	if sessionID == "" {
		h.respondError(s, i, "Session ID is required.")
		return
	}

	if err := bot.DeleteSession(sessionID); err != nil {
		h.respondError(s, i, fmt.Sprintf("Error deleting session: %v", err))
		return
	}

	h.respondSuccess(s, i, fmt.Sprintf("✅ Deleted session %s", sessionID))
}

func (h *Handler) handleCurrentSession(s DiscordSession, i *discordgo.InteractionCreate, bot HistoryBotInterface) {
	user := getInteractionUser(i)
	if user == nil {
		h.respondError(s, i, "Error: Unable to identify user.")
		return
	}

	if !h.isUserAllowed(user.ID) {
		h.respondError(s, i, "You are not authorized to use this command.")
		return
	}

	sessionID := bot.GetCurrentSessionID()
	if sessionID == "" {
		h.respondSuccess(s, i, "No active session.")
		return
	}

	h.respondSuccess(s, i, fmt.Sprintf("Current session: `%s`", sessionID))
}

func (h *Handler) respondSuccess(s DiscordSession, i *discordgo.InteractionCreate, msg string) {
	if err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: msg},
	}); err != nil {
		slog.Error("responding to interaction", "error", err)
	}
}
