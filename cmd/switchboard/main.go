package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	_ "modernc.org/sqlite"

	"github.com/TheLazyLemur/switchboard/internal/api"
	"github.com/TheLazyLemur/switchboard/internal/config"
	"github.com/TheLazyLemur/switchboard/internal/core"
	"github.com/TheLazyLemur/switchboard/internal/dashboard"
	"github.com/TheLazyLemur/switchboard/internal/permission"
	"github.com/TheLazyLemur/switchboard/internal/skills"
	"github.com/pkg/errors"
)

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
	if err := cfg.EnsureDirs(); err != nil {
		return err
	}

	// Export the resolved MEMORY_DIR so the memory skill scripts pick it up
	// when the model invokes them via the Bash tool.
	if err := os.Setenv("MEMORY_DIR", cfg.MemoryDir); err != nil {
		return errors.Wrap(err, "exporting MEMORY_DIR")
	}

	if err := core.BootstrapAgentsMd(cfg.ClaudeCWD, cfg.AgentsDefaultPath); err != nil {
		slog.Warn("bootstrap AGENTS.md", "error", err)
	}

	hub := dashboard.NewHub()
	go hub.Run()

	baseHandler := slog.NewTextHandler(os.Stdout, nil)
	slog.SetDefault(slog.New(dashboard.NewBroadcastHandler(hub, baseHandler)))

	slog.Info("starting")

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

	base := api.BackendFactory{
		APIKey:               cfg.APIKey,
		BaseURL:              cfg.BaseURL,
		Model:                cfg.Model,
		DefaultWorkDir:       cfg.ClaudeCWD,
		SkillStore:           skillStore,
		WebSearchAPIKey:      cfg.WebSearchAPIKey,
		ThinkingBudgetTokens: cfg.ThinkingBudgetTokens,
	}
	baseFactory := core.BackendFactory(&base)

	defaultPerms := core.PermissionChecker(permission.NewAutoApprovePermissionChecker(cfg.AllowedDirs))

	// Memory flush runs one final agent turn before each session reset (triggered
	// by a SessionKey change), so the model can persist durable facts. Disable
	// with MEMORY_FLUSH_DISABLED=1.
	var flushFn core.FlushFunc
	if os.Getenv("MEMORY_FLUSH_DISABLED") != "1" {
		flushFn = core.NewMemoryFlusher(defaultPerms)
	}

	baseSessionMgr := core.NewSessionManager(baseFactory, flushFn)
	defer baseSessionMgr.Close()
	bot := core.NewBot(baseSessionMgr, defaultPerms)

	if cfg.DiscordEnabled() {
		stop, err := startDiscord(cfg, bot)
		if err != nil {
			return err
		}
		defer stop()
	}

	if cfg.WhatsAppEnabled() {
		stop, err := startWhatsApp(cfg, hub, bot)
		if err != nil {
			return err
		}
		defer stop()
	}

	stopServer, err := startHTTPServer(cfg, hub, bot, baseSessionMgr, defaultPerms, skillStore, skillsDir)
	if err != nil {
		return errors.Wrap(err, "start HTTP server")
	}
	defer stopServer()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("shutting down")
	return nil
}

