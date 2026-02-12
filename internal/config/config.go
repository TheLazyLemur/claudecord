package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

type Mode string

const (
	ModeCLI Mode = "cli"
	ModeAPI Mode = "api"
)

type Config struct {
	DiscordToken string
	AllowedDirs  []string
	AllowedUsers []string
	ClaudeCWD    string
	WebhookPort  string

	// Backend mode: "cli" or "api"
	Mode Mode
	// API key for API mode (CLAUDECORD_API_KEY)
	APIKey string
	// Optional base URL for API mode (CLAUDECORD_BASE_URL)
	BaseURL string
	// Resend API key for email skills
	ResendAPIKey string
	// Optional password for dashboard auth
	DashboardPassword string
	// Minimax API key for WebSearch tool
	MinimaxAPIKey string
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

	mode := Mode(strings.ToLower(env["CLAUDECORD_MODE"]))
	if mode == "" {
		mode = ModeAPI // default to API
	}
	if mode != ModeCLI && mode != ModeAPI {
		return nil, errors.Errorf("invalid CLAUDECORD_MODE %q: must be 'cli' or 'api'", mode)
	}

	apiKey := env["CLAUDECORD_API_KEY"]
	if mode == ModeAPI && apiKey == "" {
		return nil, errors.New("CLAUDECORD_API_KEY required when CLAUDECORD_MODE=api")
	}

	baseURL := env["CLAUDECORD_BASE_URL"]
	resendAPIKey := env["RESEND_API_KEY"]
	dashboardPassword := env["DASHBOARD_PASSWORD"]
	minimaxAPIKey := env["MINIMAX_API_KEY"]

	return &Config{
		DiscordToken: discordToken,
		AllowedDirs:  allowedDirs,
		AllowedUsers: allowedUsers,
		ClaudeCWD:    claudeCwd,
		WebhookPort:  webhookPort,
		Mode:         mode,
		APIKey:       apiKey,
		BaseURL:           baseURL,
		ResendAPIKey:      resendAPIKey,
		DashboardPassword: dashboardPassword,
		MinimaxAPIKey:     minimaxAPIKey,
	}, nil
}

// LoadFromEnv loads config from os environment variables.
func LoadFromEnv() (*Config, error) {
	env := map[string]string{
		"DISCORD_TOKEN":       os.Getenv("DISCORD_TOKEN"),
		"ALLOWED_DIRS":        os.Getenv("ALLOWED_DIRS"),
		"ALLOWED_USERS":       os.Getenv("ALLOWED_USERS"),
		"CLAUDE_CWD":          os.Getenv("CLAUDE_CWD"),
		"WEBHOOK_PORT":        os.Getenv("WEBHOOK_PORT"),
		"CLAUDECORD_MODE":     os.Getenv("CLAUDECORD_MODE"),
		"CLAUDECORD_API_KEY":  os.Getenv("CLAUDECORD_API_KEY"),
		"CLAUDECORD_BASE_URL": os.Getenv("CLAUDECORD_BASE_URL"),
		"RESEND_API_KEY":      os.Getenv("RESEND_API_KEY"),
		"DASHBOARD_PASSWORD":  os.Getenv("DASHBOARD_PASSWORD"),
		"MINIMAX_API_KEY":     os.Getenv("MINIMAX_API_KEY"),
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
