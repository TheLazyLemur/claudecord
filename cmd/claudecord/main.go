package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/cli"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/handler"
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

const initTimeout = 30 * time.Second

type cliProcessFactory struct {
	workingDir string
}

func (f *cliProcessFactory) Create(resumeSessionID string) (core.CLIProcess, error) {
	return cli.NewProcess(f.workingDir, resumeSessionID, initTimeout)
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "error", err)
		os.Exit(1)
	}
}

func run() error {
	// read required env vars
	discordToken := os.Getenv("DISCORD_TOKEN")
	if discordToken == "" {
		return errors.New("DISCORD_TOKEN required")
	}

	allowedDirsStr := os.Getenv("ALLOWED_DIRS")
	if allowedDirsStr == "" {
		return errors.New("ALLOWED_DIRS required")
	}

	allowedDirs := strings.Split(allowedDirsStr, ",")
	for i := range allowedDirs {
		allowedDirs[i] = strings.TrimSpace(allowedDirs[i])
	}

	claudeCwd := os.Getenv("CLAUDE_CWD")
	if claudeCwd == "" {
		claudeCwd = allowedDirs[0]
	}

	// create dependencies
	permChecker := cli.NewPermissionChecker(allowedDirs)
	processFactory := &cliProcessFactory{workingDir: claudeCwd}
	sessionMgr := core.NewSessionManager(processFactory)

	// create discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		return errors.Wrap(err, "creating discord session")
	}

	discordClient := handler.NewDiscordClientWrapper(dg)
	bot := core.NewBot(sessionMgr, discordClient, permChecker)

	// need intents for message content
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	// open discord connection to get bot ID
	if err := dg.Open(); err != nil {
		return errors.Wrap(err, "opening discord connection")
	}
	defer dg.Close()

	botID := dg.State.User.ID
	h := handler.NewHandler(bot, botID)

	// register event handlers (after Open is fine, discordgo queues internally)
	dg.AddHandler(h.OnMessageCreate)
	dg.AddHandler(h.OnInteractionCreate)

	// register slash commands
	cmds := handler.SlashCommands()
	for _, cmd := range cmds {
		_, err := dg.ApplicationCommandCreate(dg.State.User.ID, "", cmd)
		if err != nil {
			slog.Warn("registering slash command", "name", cmd.Name, "error", err)
		}
	}

	slog.Info("bot started", "user", dg.State.User.Username)

	// wait for interrupt
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("shutting down")
	sessionMgr.Close()

	return nil
}
