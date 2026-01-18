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

func TestLoad_Success(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN": "mytoken",
		"ALLOWED_DIRS":  "/home/user, /tmp",
		"ALLOWED_USERS": "123, 456",
	})
	require.NoError(t, err)

	assert.Equal(t, "mytoken", cfg.DiscordToken)
	assert.Equal(t, []string{"/home/user", "/tmp"}, cfg.AllowedDirs)
	assert.Equal(t, []string{"123", "456"}, cfg.AllowedUsers)
	assert.Equal(t, "/home/user", cfg.ClaudeCWD)
}

func TestLoad_ClaudeCWDOverride(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN": "mytoken",
		"ALLOWED_DIRS":  "/home/user",
		"ALLOWED_USERS": "123",
		"CLAUDE_CWD":    "/custom/path",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", cfg.ClaudeCWD)
}
