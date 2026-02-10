package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_RequiresDiscordToken(t *testing.T) {
	_, err := Load(map[string]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "DISCORD_TOKEN required")
}

func TestLoad_RequiresAllowedDirs(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN": "token",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALLOWED_DIRS required")
}

func TestLoad_RequiresAllowedUsers(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN": "token",
		"ALLOWED_DIRS":  "/tmp",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALLOWED_USERS required")
}

func TestLoad_ValidatesAllowedUsersNumeric(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN": "token",
		"ALLOWED_DIRS":  "/tmp",
		"ALLOWED_USERS": "notanumber",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be numeric")
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
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDE_CWD":         "/custom/path",
		"CLAUDECORD_MODE":    "cli",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", cfg.ClaudeCWD)
}

func TestLoad_DefaultHistoryDir(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":   "mytoken",
		"ALLOWED_DIRS":    "/home/user",
		"ALLOWED_USERS":   "123",
		"CLAUDECORD_MODE": "cli",
	})
	require.NoError(t, err)
	// Should contain .claudecord/history
	assert.Contains(t, cfg.HistoryBaseDir, ".claudecord")
	assert.Contains(t, cfg.HistoryBaseDir, "history")
}

func TestLoad_CustomHistoryDir(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":          "mytoken",
		"ALLOWED_DIRS":           "/home/user",
		"ALLOWED_USERS":          "123",
		"CLAUDECORD_MODE":        "cli",
		"CLAUDECORD_HISTORY_DIR": "/custom/history/path",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/history/path", cfg.HistoryBaseDir)
}
