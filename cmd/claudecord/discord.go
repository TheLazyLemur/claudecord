package main

import (
	"log/slog"

	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/handler"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

// startDiscord wires up Discord using the provided factories and permission
// checkers. Returns a cleanup func that closes the session manager and the
// Discord connection. flushFn may be nil.
func startDiscord(
	cfg *config.Config,
	discordFactory core.BackendFactory,
	passiveFactory core.BackendFactory,
	defaultPerms core.PermissionChecker,
	roPerms core.PermissionChecker,
	flushFn core.FlushFunc,
) (func(), error) {
	discordSessionMgr := core.NewSessionManager(discordFactory, flushFn)
	discordBot := core.NewBot(discordSessionMgr, defaultPerms)

	passiveSessionMgr := core.NewSessionManager(passiveFactory, flushFn)

	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		discordSessionMgr.Close()
		passiveSessionMgr.Close()
		return nil, errors.Wrap(err, "creating discord session")
	}

	discordClient := handler.NewDiscordClientWrapper(dg)
	passiveBot := core.NewPassiveBot(passiveSessionMgr, discordClient, roPerms)

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	if err := dg.Open(); err != nil {
		discordSessionMgr.Close()
		passiveSessionMgr.Close()
		return nil, errors.Wrap(err, "opening discord connection")
	}

	dg.UpdateGameStatus(0, "Ready")
	slog.Info("discord connected", "botID", dg.State.User.ID, "username", dg.State.User.Username)

	h := handler.NewHandler(discordBot, dg.State.User.ID, cfg.AllowedUsers, discordClient, passiveBot)
	dg.AddHandler(h.OnMessageCreate)
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		h.OnInteractionCreate(s, i)
	})

	for _, cmd := range handler.SlashCommands() {
		if _, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd); err != nil {
			slog.Warn("registering slash command", "name", cmd.Name, "error", err)
		}
	}

	slog.Info("discord bot started", "user", dg.State.User.Username)

	cleanup := func() {
		dg.Close()
		discordSessionMgr.Close()
		passiveSessionMgr.Close()
	}
	return cleanup, nil
}
