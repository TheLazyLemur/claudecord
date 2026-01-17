package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strconv"
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
	processFactory := &cliProcessFactory{defaultWorkDir: claudeCwd, allowedDirs: allowedDirs}
	sessionMgr := core.NewSessionManager(processFactory)

	// create discord session
	dg, err := discordgo.New("Bot " + discordToken)
	if err != nil {
		return errors.Wrap(err, "creating discord session")
	}

	discordClient := handler.NewDiscordClientWrapper(dg)
	bot := core.NewBot(sessionMgr, discordClient, permChecker)

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

	// parse allowed users (required)
	allowedUsersStr := os.Getenv("ALLOWED_USERS")
	if allowedUsersStr == "" {
		return errors.New("ALLOWED_USERS required")
	}
	allowedUsers := strings.Split(allowedUsersStr, ",")
	for i := range allowedUsers {
		allowedUsers[i] = strings.TrimSpace(allowedUsers[i])
		if _, err := strconv.ParseUint(allowedUsers[i], 10, 64); err != nil {
			return errors.Errorf("invalid user ID %q: must be numeric", allowedUsers[i])
		}
	}

	// now create handler with botID and allowed users
	h := handler.NewHandler(bot, dg.State.User.ID, allowedUsers)
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

	// wait for interrupt
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	slog.Info("shutting down")
	sessionMgr.Close()

	return nil
}
