package main

import (
	"context"
	"log/slog"

	"github.com/TheLazyLemur/claudecord/internal/channels/discord"
	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/pkg/errors"
)

// startDiscord constructs the Discord plugin, starts it, and returns a cleanup func.
func startDiscord(cfg *config.Config, bot *core.Bot) (func(), error) {
	plugin := discord.New(discord.Config{
		Token:        cfg.DiscordToken,
		AllowedUsers: cfg.AllowedUsers,
	})

	if err := plugin.Start(context.Background(), func(in core.Inbound) {
		if err := bot.HandleInbound(in); err != nil {
			slog.Error("handling discord inbound", "error", err)
		}
	}); err != nil {
		return nil, errors.Wrap(err, "starting discord plugin")
	}

	slog.Info("discord plugin started")

	cleanup := func() {
		if err := plugin.Stop(); err != nil {
			slog.Warn("discord plugin stop", "error", err)
		}
	}
	return cleanup, nil
}
