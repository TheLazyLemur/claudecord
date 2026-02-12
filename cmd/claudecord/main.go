package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/api"
	"github.com/TheLazyLemur/claudecord/internal/cli"
	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/dashboard"
	"github.com/TheLazyLemur/claudecord/internal/handler"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

const initTimeout = 30 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadFromEnv()
	if err != nil {
		return err
	}

	// Create dashboard hub and wrap slog handler
	hub := dashboard.NewHub()
	go hub.Run()

	baseHandler := slog.NewTextHandler(os.Stdout, nil)
	broadcastHandler := dashboard.NewBroadcastHandler(hub, baseHandler)
	slog.SetDefault(slog.New(broadcastHandler))

	slog.Info("starting", "mode", cfg.Mode)

	// Initialize skills
	skillsDir, err := skills.DefaultSkillsDir()
	if err != nil {
		return errors.Wrap(err, "getting skills dir")
	}
	if err := skills.DumpBuiltinSkills(skillsDir); err != nil {
		slog.Warn("dumping builtin skills", "error", err)
	}
	skillStore := skills.NewFSSkillStore(skillsDir)
	skillList, _ := skillStore.List()
	slog.Info("skills loaded", "count", len(skillList))

	// Create backend factory based on mode
	var backendFactory core.BackendFactory
	var passiveFactory core.BackendFactory

	switch cfg.Mode {
	case config.ModeCLI:
		backendFactory = &cli.BackendFactory{
			DefaultWorkDir: cfg.ClaudeCWD,
			AllowedDirs:    cfg.AllowedDirs,
			InitTimeout:    initTimeout,
			SkillStore:     skillStore,
		}
		passiveFactory = &cli.BackendFactory{
			DefaultWorkDir: cfg.ClaudeCWD,
			AllowedDirs:    cfg.AllowedDirs,
			InitTimeout:    initTimeout,
			SkillStore:     skillStore,
			Passive:        true,
		}
	case config.ModeAPI:
		backendFactory = &api.BackendFactory{
			APIKey:         cfg.APIKey,
			BaseURL:        cfg.BaseURL,
			AllowedDirs:    cfg.AllowedDirs,
			DefaultWorkDir: cfg.ClaudeCWD,
			SkillStore:     skillStore,
			MinimaxAPIKey:  cfg.MinimaxAPIKey,
		}
		passiveFactory = &api.BackendFactory{
			APIKey:         cfg.APIKey,
			BaseURL:        cfg.BaseURL,
			AllowedDirs:    cfg.AllowedDirs,
			DefaultWorkDir: cfg.ClaudeCWD,
			SkillStore:     skillStore,
			MinimaxAPIKey:  cfg.MinimaxAPIKey,
			Passive:        true,
		}
	}

	// Create permission checkers
	permChecker := cli.NewPermissionChecker(cfg.AllowedDirs)
	roPermChecker := cli.NewReadOnlyPermissionChecker(cfg.AllowedDirs)

	// Create session managers
	sessionMgr := core.NewSessionManager(backendFactory)
	passiveSessionMgr := core.NewSessionManager(passiveFactory)

	// Create discord session
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return errors.Wrap(err, "creating discord session")
	}

	discordClient := handler.NewDiscordClientWrapper(dg)
	bot := core.NewBot(sessionMgr, permChecker)
	passiveBot := core.NewPassiveBot(passiveSessionMgr, discordClient, roPermChecker)

	// Need intents for message content and reactions
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent | discordgo.IntentsGuildMessageReactions

	// Register ready handler
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("READY event", "user", r.User.Username, "guilds", len(r.Guilds))
	})

	// Register raw message handler to debug
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		slog.Info("RAW message", "content", m.Content, "author", m.Author.Username)
	})

	// Open discord connection to get bot ID
	if err := dg.Open(); err != nil {
		return errors.Wrap(err, "opening discord connection")
	}
	defer dg.Close()

	// Set bot status to online
	dg.UpdateGameStatus(0, "Ready")

	slog.Info("connected", "botID", dg.State.User.ID, "username", dg.State.User.Username)

	// Create handler with botID, allowed users, and passive bot
	h := handler.NewHandler(bot, dg.State.User.ID, cfg.AllowedUsers, discordClient, passiveBot)
	dg.AddHandler(h.OnMessageCreate)
	dg.AddHandler(h.OnReactionAdd)
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		h.OnInteractionCreate(s, i)
	})

	// Register slash commands
	cmds := handler.SlashCommands()
	for _, cmd := range cmds {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd)
		if err != nil {
			slog.Warn("registering slash command", "name", cmd.Name, "error", err)
		}
	}

	slog.Info("bot started", "user", dg.State.User.Username)

	// Create dashboard server (shares session with Discord bot)
	dashboardServer := dashboard.NewServer(hub, sessionMgr, permChecker, skillStore, skillsDir, cfg.DashboardPassword)

	// Start webhook + dashboard server
	mux := http.NewServeMux()
	mux.Handle("/webhook", handler.NewWebhookHandler())
	mux.Handle("/", dashboardServer.Handler())
	srv := &http.Server{Addr: ":" + cfg.WebhookPort, Handler: mux}
	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	// Wait for interrupt
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	sessionMgr.Close()

	return nil
}
