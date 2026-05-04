package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "modernc.org/sqlite"

	"github.com/TheLazyLemur/claudecord/internal/api"
	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/dashboard"
	"github.com/TheLazyLemur/claudecord/internal/handler"
	"github.com/TheLazyLemur/claudecord/internal/permission"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/bwmarrin/discordgo"
	"github.com/mdp/qrterminal/v3"
	"github.com/pkg/errors"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
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
	// when the model invokes them via the Bash tool. Set unconditionally so
	// scripts see the resolved default even when the user didn't set it.
	if err := os.Setenv("MEMORY_DIR", cfg.MemoryDir); err != nil {
		return errors.Wrap(err, "exporting MEMORY_DIR")
	}

	if err := core.BootstrapAgentsMd(cfg.ClaudeCWD, cfg.AgentsDefaultPath); err != nil {
		slog.Warn("bootstrap AGENTS.md", "error", err)
	}

	// Create dashboard hub and wrap slog handler
	hub := dashboard.NewHub()
	go hub.Run()

	baseHandler := slog.NewTextHandler(os.Stdout, nil)
	broadcastHandler := dashboard.NewBroadcastHandler(hub, baseHandler)
	slog.SetDefault(slog.New(broadcastHandler))

	slog.Info("starting")

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

	// Create backend factories.
	// discordFactory includes react_emoji tool; backendFactory (WA/dashboard) does not.
	base := api.BackendFactory{
		APIKey:               cfg.APIKey,
		BaseURL:              cfg.BaseURL,
		Model:                cfg.Model,
		AllowedDirs:          cfg.AllowedDirs,
		DefaultWorkDir:       cfg.ClaudeCWD,
		SkillStore:           skillStore,
		MinimaxAPIKey:        cfg.MinimaxAPIKey,
		WhatsAppEnabled:      cfg.WhatsAppEnabled(),
		ThinkingBudgetTokens: cfg.ThinkingBudgetTokens,
	}
	discord := base
	discord.Discord = true
	passive := base
	passive.Passive = true
	backendFactory := core.BackendFactory(&base)
	discordFactory := core.BackendFactory(&discord)
	passiveFactory := core.BackendFactory(&passive)

	// Auto-approve everything inside ALLOWED_DIRS for both platforms.
	// Path containment is the only safety check; no interactive prompts.
	defaultPermChecker := permission.NewAutoApprovePermissionChecker(cfg.AllowedDirs)
	roPermChecker := permission.NewReadOnlyPermissionChecker(cfg.AllowedDirs)

	discordPermChecker := core.PermissionChecker(defaultPermChecker)
	waPermChecker := core.PermissionChecker(defaultPermChecker)

	// Memory flush runs one final agent turn before each /new-session, so
	// the model can persist durable facts via remember.sh / note.sh. Disable
	// with MEMORY_FLUSH_DISABLED=1.
	var flushFn core.FlushFunc
	if os.Getenv("MEMORY_FLUSH_DISABLED") != "1" {
		flushFn = core.NewMemoryFlusher(waPermChecker)
	}

	// Create session manager + bot for WA/dashboard (no react_emoji)
	sessionMgr := core.NewSessionManager(backendFactory, flushFn)
	bot := core.NewBot(sessionMgr, waPermChecker)
	defer sessionMgr.Close()

	// Discord (optional) — separate session manager with react_emoji
	if cfg.DiscordToken != "" {
		discordSessionMgr := core.NewSessionManager(discordFactory, flushFn)
		defer discordSessionMgr.Close()
		discordBot := core.NewBot(discordSessionMgr, discordPermChecker)

		passiveSessionMgr := core.NewSessionManager(passiveFactory, flushFn)
		defer passiveSessionMgr.Close()

		dg, err := discordgo.New("Bot " + cfg.DiscordToken)
		if err != nil {
			return errors.Wrap(err, "creating discord session")
		}

		discordClient := handler.NewDiscordClientWrapper(dg)
		passiveBot := core.NewPassiveBot(passiveSessionMgr, discordClient, roPermChecker)

		dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

		dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
			slog.Info("READY event", "user", r.User.Username, "guilds", len(r.Guilds))
		})

		dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
			slog.Info("RAW message", "content", m.Content, "author", m.Author.Username)
		})

		if err := dg.Open(); err != nil {
			return errors.Wrap(err, "opening discord connection")
		}
		defer dg.Close()

		dg.UpdateGameStatus(0, "Ready")

		slog.Info("discord connected", "botID", dg.State.User.ID, "username", dg.State.User.Username)

		h := handler.NewHandler(discordBot, dg.State.User.ID, cfg.AllowedUsers, discordClient, passiveBot)
		dg.AddHandler(h.OnMessageCreate)
		dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
			h.OnInteractionCreate(s, i)
		})

		cmds := handler.SlashCommands()
		for _, cmd := range cmds {
			_, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd)
			if err != nil {
				slog.Warn("registering slash command", "name", cmd.Name, "error", err)
			}
		}

		slog.Info("discord bot started", "user", dg.State.User.Username)
	}

	// WhatsApp (optional)
	if cfg.WhatsAppEnabled() {
		container, err := sqlstore.New(context.Background(), "sqlite", "file:"+cfg.WhatsAppDBPath+"?_pragma=foreign_keys(1)", nil)
		if err != nil {
			return errors.Wrap(err, "creating whatsapp store")
		}
		device, err := container.GetFirstDevice(context.Background())
		if err != nil {
			return errors.Wrap(err, "getting whatsapp device")
		}
		waClient := whatsmeow.NewClient(device, nil)
		waWrapper := handler.NewWhatsAppClientWrapper(waClient)
		waHandler := handler.NewWAHandler(bot, cfg.WhatsAppAllowedSenders, waWrapper, cfg.WhatsAppMediaDir)
		defer waHandler.Stop()
		waClient.AddEventHandler(waHandler.HandleEvent)

		if waClient.Store.ID == nil {
			qrChan, err := waClient.GetQRChannel(context.Background())
			if err != nil {
				return errors.Wrap(err, "getting whatsapp QR channel")
			}
			if err := waClient.Connect(); err != nil {
				return errors.Wrap(err, "connecting whatsapp")
			}
			go func() {
				for evt := range qrChan {
					if evt.Event == "code" {
						fmt.Println("Scan this QR code in WhatsApp > Linked Devices:")
						qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
						hub.BroadcastSticky(dashboard.Message{Type: "whatsapp_qr", Content: evt.Code})
					} else {
						slog.Info("whatsapp qr event", "event", evt.Event)
						hub.ClearSticky()
						hub.Broadcast(dashboard.Message{Type: "whatsapp_qr", Content: evt.Event})
					}
				}
			}()
		} else {
			if err := waClient.Connect(); err != nil {
				return errors.Wrap(err, "connecting whatsapp")
			}
		}
		slog.Info("whatsapp connected")
		defer waClient.Disconnect()
	}

	// Dashboard server (platform-independent)
	dashboardServer := dashboard.NewServer(hub, sessionMgr, waPermChecker, skillStore, skillsDir, cfg.ClaudeCWD, cfg.AgentsDefaultPath, cfg.MemoryDir, cfg.DashboardPassword)

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

	return nil
}
