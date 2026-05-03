package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

const (
	DefaultModel     = "claude-sonnet-4-20250514"
	DefaultKimiModel = "Kimi-for-Coding"
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

	// Model id passed to the API. Required when BaseURL is set, else defaults
	// to the Sonnet id baked into DefaultModel.
	Model string

	// WhatsApp
	WhatsAppAllowedSenders []string
	WhatsAppDBPath         string
	// Directory media attachments are decrypted into. Required when WhatsApp
	// is enabled. Must live under one of AllowedDirs.
	WhatsAppMediaDir string

	// Directory the memory skill stores MEMORY.md and daily logs in. Defaults
	// to <first AllowedDirs>/claudecord-memory. Must live under AllowedDirs.
	MemoryDir string
}

func (c *Config) DiscordEnabled() bool {
	return c.DiscordToken != ""
}

func (c *Config) WhatsAppEnabled() bool {
	return len(c.WhatsAppAllowedSenders) > 0
}

// Load reads config from env map. For production use LoadFromEnv.
func Load(env map[string]string) (*Config, error) {
	discordToken := env["DISCORD_TOKEN"]

	// WhatsApp config
	var whatsAppSenders []string
	if s := env["WHATSAPP_ALLOWED_SENDERS"]; s != "" {
		whatsAppSenders = splitAndTrim(s)
	}

	whatsAppDBPath := env["WHATSAPP_DB_PATH"]
	if whatsAppDBPath == "" {
		whatsAppDBPath = "whatsapp.db"
	}

	// At least one platform required
	if discordToken == "" && len(whatsAppSenders) == 0 {
		return nil, errors.New("at least one platform required: set DISCORD_TOKEN or WHATSAPP_ALLOWED_SENDERS")
	}

	allowedDirsStr := env["ALLOWED_DIRS"]
	if allowedDirsStr == "" {
		return nil, errors.New("ALLOWED_DIRS required")
	}
	allowedDirs := splitAndTrim(allowedDirsStr)

	// Discord requires ALLOWED_USERS with numeric IDs
	var allowedUsers []string
	if discordToken != "" {
		allowedUsersStr := env["ALLOWED_USERS"]
		if allowedUsersStr == "" {
			return nil, errors.New("ALLOWED_USERS required when DISCORD_TOKEN is set")
		}
		allowedUsers = splitAndTrim(allowedUsersStr)
		for _, u := range allowedUsers {
			if _, err := strconv.ParseUint(u, 10, 64); err != nil {
				return nil, errors.Errorf("invalid user ID %q: must be numeric", u)
			}
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

	model := env["MODEL"]
	if model == "" {
		if baseURL != "" {
			model = DefaultKimiModel
		} else {
			model = DefaultModel
		}
	}

	var mediaDir string
	if len(whatsAppSenders) > 0 {
		mediaDir = env["WHATSAPP_MEDIA_DIR"]
		if mediaDir == "" {
			mediaDir = filepath.Join(allowedDirs[0], "wa-media")
		}
		if !pathInsideAllowedDirs(mediaDir, allowedDirs) {
			return nil, errors.Errorf("WHATSAPP_MEDIA_DIR %q must live under ALLOWED_DIRS", mediaDir)
		}
		if err := os.MkdirAll(mediaDir, 0o700); err != nil {
			return nil, errors.Wrap(err, "creating WHATSAPP_MEDIA_DIR")
		}
	}

	memoryDir := env["MEMORY_DIR"]
	if memoryDir == "" {
		memoryDir = filepath.Join(allowedDirs[0], "claudecord-memory")
	}
	if !pathInsideAllowedDirs(memoryDir, allowedDirs) {
		return nil, errors.Errorf("MEMORY_DIR %q must live under ALLOWED_DIRS", memoryDir)
	}
	if err := os.MkdirAll(memoryDir, 0o700); err != nil {
		return nil, errors.Wrap(err, "creating MEMORY_DIR")
	}

	return &Config{
		DiscordToken:           discordToken,
		AllowedDirs:            allowedDirs,
		AllowedUsers:           allowedUsers,
		ClaudeCWD:              claudeCwd,
		WebhookPort:            webhookPort,
		Mode:                   mode,
		APIKey:                 apiKey,
		BaseURL:                baseURL,
		ResendAPIKey:           resendAPIKey,
		DashboardPassword:      dashboardPassword,
		MinimaxAPIKey:          minimaxAPIKey,
		Model:                  model,
		WhatsAppAllowedSenders: whatsAppSenders,
		WhatsAppDBPath:         whatsAppDBPath,
		WhatsAppMediaDir:       mediaDir,
		MemoryDir:              memoryDir,
	}, nil
}

func pathInsideAllowedDirs(path string, allowedDirs []string) bool {
	clean := filepath.Clean(path)
	for _, dir := range allowedDirs {
		d := filepath.Clean(dir)
		if clean == d || strings.HasPrefix(clean, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// LoadFromEnv loads config from os environment variables.
func LoadFromEnv() (*Config, error) {
	env := map[string]string{
		"DISCORD_TOKEN":            os.Getenv("DISCORD_TOKEN"),
		"ALLOWED_DIRS":             os.Getenv("ALLOWED_DIRS"),
		"ALLOWED_USERS":            os.Getenv("ALLOWED_USERS"),
		"CLAUDE_CWD":               os.Getenv("CLAUDE_CWD"),
		"WEBHOOK_PORT":             os.Getenv("WEBHOOK_PORT"),
		"CLAUDECORD_MODE":          os.Getenv("CLAUDECORD_MODE"),
		"CLAUDECORD_API_KEY":       os.Getenv("CLAUDECORD_API_KEY"),
		"CLAUDECORD_BASE_URL":      os.Getenv("CLAUDECORD_BASE_URL"),
		"RESEND_API_KEY":           os.Getenv("RESEND_API_KEY"),
		"DASHBOARD_PASSWORD":       os.Getenv("DASHBOARD_PASSWORD"),
		"MINIMAX_API_KEY":          os.Getenv("MINIMAX_API_KEY"),
		"WHATSAPP_ALLOWED_SENDERS": os.Getenv("WHATSAPP_ALLOWED_SENDERS"),
		"WHATSAPP_DB_PATH":         os.Getenv("WHATSAPP_DB_PATH"),
		"WHATSAPP_MEDIA_DIR":       os.Getenv("WHATSAPP_MEDIA_DIR"),
		"MODEL":                    os.Getenv("MODEL"),
		"MEMORY_DIR":               os.Getenv("MEMORY_DIR"),
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
