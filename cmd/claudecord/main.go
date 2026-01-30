package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/cli"
	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/handler"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

const initTimeout = 30 * time.Second

type cliProcessFactory struct {
	defaultWorkDir string
	allowedDirs    []string
}

func (f *cliProcessFactory) Create(resumeSessionID, workDir string) (core.CLIProcess, error) {
	if workDir == "" {
		workDir = f.defaultWorkDir
	} else if !f.isAllowed(workDir) {
		return nil, errors.Errorf("directory %q not under allowed dirs", workDir)
	}
	return cli.NewProcess(workDir, resumeSessionID, initTimeout)
}

func (f *cliProcessFactory) isAllowed(path string) bool {
	for _, allowed := range f.allowedDirs {
		if path == allowed || strings.HasPrefix(path, allowed+"/") {
			return true
		}
	}
	return false
}

type passiveCLIProcessFactory struct {
	defaultWorkDir string
}

func (f *passiveCLIProcessFactory) Create(resumeSessionID, workDir string) (core.CLIProcess, error) {
	if workDir == "" {
		workDir = f.defaultWorkDir
	}
	return cli.NewProcessWithSystemPrompt(workDir, resumeSessionID, initTimeout, core.PassiveSystemPrompt())
}

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

	// create dependencies
	permChecker := cli.NewPermissionChecker(cfg.AllowedDirs)
	processFactory := &cliProcessFactory{defaultWorkDir: cfg.ClaudeCWD, allowedDirs: cfg.AllowedDirs}
	sessionMgr := core.NewSessionManager(processFactory)

	// create discord session
	dg, err := discordgo.New("Bot " + cfg.DiscordToken)
	if err != nil {
		return errors.Wrap(err, "creating discord session")
	}

	discordClient := handler.NewDiscordClientWrapper(dg)
	bot := core.NewBot(sessionMgr, discordClient, permChecker)

	// create passive bot for auto-help feature
	roPermChecker := cli.NewReadOnlyPermissionChecker(cfg.AllowedDirs)
	passiveSessionMgr := core.NewSessionManager(&passiveCLIProcessFactory{
		defaultWorkDir: cfg.ClaudeCWD,
	})
	passiveBot := core.NewPassiveBot(passiveSessionMgr, discordClient, roPermChecker)

	// need intents for message content
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	// register ready handler
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		slog.Info("READY event", "user", r.User.Username, "guilds", len(r.Guilds))
	})

	// register raw message handler to debug
	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		slog.Info("RAW message", "content", m.Content, "author", m.Author.Username)
	})

	// open discord connection to get bot ID
	if err := dg.Open(); err != nil {
		return errors.Wrap(err, "opening discord connection")
	}
	defer dg.Close()

	// set bot status to online
	dg.UpdateGameStatus(0, "Ready")

	slog.Info("connected", "botID", dg.State.User.ID, "username", dg.State.User.Username)

	// create handler with botID, allowed users, and passive bot
	h := handler.NewHandler(bot, dg.State.User.ID, cfg.AllowedUsers, passiveBot)
	dg.AddHandler(h.OnMessageCreate)
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		h.OnInteractionCreate(s, i)
	})

	// register slash commands
	cmds := handler.SlashCommands()
	for _, cmd := range cmds {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd)
		if err != nil {
			slog.Warn("registering slash command", "name", cmd.Name, "error", err)
		}
	}

	slog.Info("bot started", "user", dg.State.User.Username)

	// start webhook server
	mux := http.NewServeMux()
	mux.Handle("/webhook", handler.NewWebhookHandler())
	srv := &http.Server{Addr: ":" + cfg.WebhookPort, Handler: mux}
	go func() {
		slog.Info("webhook server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("webhook server error", "error", err)
		}
	}()

	// wait for interrupt
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
