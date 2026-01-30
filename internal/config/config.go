package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Config struct {
	DiscordToken string
	AllowedDirs  []string
	AllowedUsers []string
	ClaudeCWD    string
	WebhookPort  string
}

// Load reads config from env map. For production use LoadFromEnv.
func Load(env map[string]string) (*Config, error) {
	discordToken := env["DISCORD_TOKEN"]
	if discordToken == "" {
		return nil, errors.New("DISCORD_TOKEN required")
	}

	allowedDirsStr := env["ALLOWED_DIRS"]
	if allowedDirsStr == "" {
		return nil, errors.New("ALLOWED_DIRS required")
	}

	allowedDirs := splitAndTrim(allowedDirsStr)

	allowedUsersStr := env["ALLOWED_USERS"]
	if allowedUsersStr == "" {
		return nil, errors.New("ALLOWED_USERS required")
	}

	allowedUsers := splitAndTrim(allowedUsersStr)
	for _, u := range allowedUsers {
		if _, err := strconv.ParseUint(u, 10, 64); err != nil {
			return nil, errors.Errorf("invalid user ID %q: must be numeric", u)
		}
	}

	claudeCwd := env["CLAUDE_CWD"]
	if claudeCwd == "" {
		claudeCwd = allowedDirs[0]
	}

	webhookPort := env["WEBHOOK_PORT"]
	if webhookPort == "" {
		webhookPort = "5005"
	}

	return &Config{
		DiscordToken: discordToken,
		AllowedDirs:  allowedDirs,
		AllowedUsers: allowedUsers,
		ClaudeCWD:    claudeCwd,
		WebhookPort:  webhookPort,
	}, nil
}

// LoadFromEnv loads config from os environment variables.
func LoadFromEnv() (*Config, error) {
	env := map[string]string{
		"DISCORD_TOKEN": os.Getenv("DISCORD_TOKEN"),
		"ALLOWED_DIRS":  os.Getenv("ALLOWED_DIRS"),
		"ALLOWED_USERS": os.Getenv("ALLOWED_USERS"),
		"CLAUDE_CWD":    os.Getenv("CLAUDE_CWD"),
		"WEBHOOK_PORT":  os.Getenv("WEBHOOK_PORT"),
	}
	return Load(env)
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}
