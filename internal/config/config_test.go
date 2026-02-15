package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDiscordEnv() map[string]string {
	return map[string]string{
		"DISCORD_TOKEN":   "mytoken",
		"ALLOWED_DIRS":    "/home/user",
		"ALLOWED_USERS":   "123",
		"CLAUDECORD_MODE": "cli",
	}
}

func validWhatsAppEnv() map[string]string {
	return map[string]string{
		"WHATSAPP_ALLOWED_SENDERS": "123456@lid",
		"ALLOWED_DIRS":             "/home/user",
		"CLAUDECORD_MODE":          "cli",
	}
}

// --- Platform requirement tests ---

func TestLoad_RequiresAtLeastOnePlatform(t *testing.T) {
	_, err := Load(map[string]string{
		"ALLOWED_DIRS":    "/tmp",
		"CLAUDECORD_MODE": "cli",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one platform")
}

func TestLoad_DiscordOnlySuccess(t *testing.T) {
	cfg, err := Load(validDiscordEnv())
	require.NoError(t, err)
	assert.True(t, cfg.DiscordEnabled())
	assert.False(t, cfg.WhatsAppEnabled())
}

func TestLoad_WhatsAppOnlySuccess(t *testing.T) {
	cfg, err := Load(validWhatsAppEnv())
	require.NoError(t, err)
	assert.False(t, cfg.DiscordEnabled())
	assert.True(t, cfg.WhatsAppEnabled())
}

func TestLoad_BothPlatformsSuccess(t *testing.T) {
	env := validWhatsAppEnv()
	env["DISCORD_TOKEN"] = "mytoken"
	env["ALLOWED_USERS"] = "123"
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.True(t, cfg.DiscordEnabled())
	assert.True(t, cfg.WhatsAppEnabled())
}

// --- Discord-specific validation ---

func TestLoad_DiscordRequiresAllowedUsers(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN":   "token",
		"ALLOWED_DIRS":    "/tmp",
		"CLAUDECORD_MODE": "cli",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALLOWED_USERS required")
}

func TestLoad_DiscordValidatesAllowedUsersNumeric(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN":   "token",
		"ALLOWED_DIRS":    "/tmp",
		"ALLOWED_USERS":   "notanumber",
		"CLAUDECORD_MODE": "cli",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be numeric")
}

// --- WhatsApp-specific tests ---

func TestLoad_WhatsAppDoesNotRequireAllowedUsers(t *testing.T) {
	cfg, err := Load(validWhatsAppEnv())
	require.NoError(t, err)
	assert.Empty(t, cfg.AllowedUsers)
}

func TestLoad_WhatsAppAllowedSendersParsed(t *testing.T) {
	env := validWhatsAppEnv()
	env["WHATSAPP_ALLOWED_SENDERS"] = "abc@lid, def@s.whatsapp.net"
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, []string{"abc@lid", "def@s.whatsapp.net"}, cfg.WhatsAppAllowedSenders)
}

func TestLoad_WhatsAppDBPathDefault(t *testing.T) {
	cfg, err := Load(validWhatsAppEnv())
	require.NoError(t, err)
	assert.Equal(t, "whatsapp.db", cfg.WhatsAppDBPath)
}

func TestLoad_WhatsAppDBPathOverride(t *testing.T) {
	env := validWhatsAppEnv()
	env["WHATSAPP_DB_PATH"] = "/data/wa.db"
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, "/data/wa.db", cfg.WhatsAppDBPath)
}

func TestLoad_WhatsAppNoNumericValidationOnAllowedUsers(t *testing.T) {
	// WhatsApp-only: ALLOWED_USERS not set, no numeric validation triggered
	cfg, err := Load(validWhatsAppEnv())
	require.NoError(t, err)
	assert.Empty(t, cfg.AllowedUsers)
}

// --- Existing tests (updated for optional Discord) ---

func TestLoad_RequiresAllowedDirs(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN": "token",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALLOWED_DIRS required")
}

func TestLoad_DefaultsToAPIMode(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-test",
	})
	require.NoError(t, err)
	assert.Equal(t, ModeAPI, cfg.Mode)
}

func TestLoad_CLIModeSuccess(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":   "mytoken",
		"ALLOWED_DIRS":    "/home/user, /tmp",
		"ALLOWED_USERS":   "123, 456",
		"CLAUDECORD_MODE": "cli",
	})
	require.NoError(t, err)

	assert.Equal(t, "mytoken", cfg.DiscordToken)
	assert.Equal(t, []string{"/home/user", "/tmp"}, cfg.AllowedDirs)
	assert.Equal(t, []string{"123", "456"}, cfg.AllowedUsers)
	assert.Equal(t, "/home/user", cfg.ClaudeCWD)
	assert.Equal(t, ModeCLI, cfg.Mode)
}

func TestLoad_APIModeRequiresAPIKey(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN":   "token",
		"ALLOWED_DIRS":    "/tmp",
		"ALLOWED_USERS":   "123",
		"CLAUDECORD_MODE": "api",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDECORD_API_KEY required")
}

func TestLoad_APIModeSuccess(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_MODE":    "api",
		"CLAUDECORD_API_KEY": "sk-test-key",
	})
	require.NoError(t, err)
	assert.Equal(t, ModeAPI, cfg.Mode)
	assert.Equal(t, "sk-test-key", cfg.APIKey)
}

func TestLoad_APIModeWithBaseURL(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"CLAUDECORD_MODE":     "api",
		"CLAUDECORD_API_KEY":  "sk-test-key",
		"CLAUDECORD_BASE_URL": "https://proxy.example.com",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://proxy.example.com", cfg.BaseURL)
}

func TestLoad_InvalidMode(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN":   "token",
		"ALLOWED_DIRS":    "/tmp",
		"ALLOWED_USERS":   "123",
		"CLAUDECORD_MODE": "invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid CLAUDECORD_MODE")
}

func TestLoad_ClaudeCWDOverride(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":   "mytoken",
		"ALLOWED_DIRS":    "/home/user",
		"ALLOWED_USERS":   "123",
		"CLAUDE_CWD":      "/custom/path",
		"CLAUDECORD_MODE": "cli",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", cfg.ClaudeCWD)
}

func TestLoad_DashboardPassword(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_MODE":    "cli",
		"DASHBOARD_PASSWORD": "secret123",
	})
	require.NoError(t, err)
	assert.Equal(t, "secret123", cfg.DashboardPassword)
}

func TestLoad_DashboardPasswordOptional(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":   "mytoken",
		"ALLOWED_DIRS":    "/home/user",
		"ALLOWED_USERS":   "123",
		"CLAUDECORD_MODE": "cli",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.DashboardPassword)
}

func TestLoad_MinimaxAPIKey(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":   "mytoken",
		"ALLOWED_DIRS":    "/home/user",
		"ALLOWED_USERS":   "123",
		"CLAUDECORD_MODE": "cli",
		"MINIMAX_API_KEY": "minimax-test-key",
	})
	require.NoError(t, err)
	assert.Equal(t, "minimax-test-key", cfg.MinimaxAPIKey)
}

func TestLoad_MinimaxAPIKeyOptional(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":   "mytoken",
		"ALLOWED_DIRS":    "/home/user",
		"ALLOWED_USERS":   "123",
		"CLAUDECORD_MODE": "cli",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.MinimaxAPIKey)
}
