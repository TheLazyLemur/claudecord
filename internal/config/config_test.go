package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDiscordEnv() map[string]string {
	return map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-test",
	}
}

func validWhatsAppEnv() map[string]string {
	dir := mustTempMediaDir()
	return map[string]string{
		"WHATSAPP_ALLOWED_SENDERS": "123456@lid",
		"ALLOWED_DIRS":             dir,
		"CLAUDECORD_API_KEY":       "sk-test",
		"WHATSAPP_MEDIA_DIR":       dir + "/media",
	}
}

func mustTempMediaDir() string {
	dir, err := os.MkdirTemp("", "claudecord-cfg-test-")
	if err != nil {
		panic(err)
	}
	return dir
}

// --- Platform requirement tests ---

func TestLoad_RequiresAtLeastOnePlatform(t *testing.T) {
	_, err := Load(map[string]string{
		"ALLOWED_DIRS":       "/tmp",
		"CLAUDECORD_API_KEY": "sk-test",
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
		"DISCORD_TOKEN":      "token",
		"ALLOWED_DIRS":       "/tmp",
		"CLAUDECORD_API_KEY": "sk-test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALLOWED_USERS required")
}

func TestLoad_DiscordValidatesAllowedUsersNumeric(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN":      "token",
		"ALLOWED_DIRS":       "/tmp",
		"ALLOWED_USERS":      "notanumber",
		"CLAUDECORD_API_KEY": "sk-test",
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

func TestLoad_RequiresAPIKey(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN": "token",
		"ALLOWED_DIRS":  "/tmp",
		"ALLOWED_USERS": "123",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "CLAUDECORD_API_KEY required")
}

func TestLoad_Success(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user, /tmp",
		"ALLOWED_USERS":      "123, 456",
		"CLAUDECORD_API_KEY": "sk-test-key",
	})
	require.NoError(t, err)

	assert.Equal(t, "mytoken", cfg.DiscordToken)
	assert.Equal(t, []string{"/home/user", "/tmp"}, cfg.AllowedDirs)
	assert.Equal(t, []string{"123", "456"}, cfg.AllowedUsers)
	assert.Equal(t, "/home/user", cfg.ClaudeCWD)
	assert.Equal(t, "sk-test-key", cfg.APIKey)
}

func TestLoad_BaseURLSet(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"CLAUDECORD_API_KEY":  "sk-test-key",
		"CLAUDECORD_BASE_URL": "https://proxy.example.com",
		"MODEL":               "kimi-k2.6",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://proxy.example.com", cfg.BaseURL)
}

func TestLoad_ClaudeCWDOverride(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDE_CWD":         "/custom/path",
		"CLAUDECORD_API_KEY": "sk-test",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", cfg.ClaudeCWD)
}

func TestLoad_DashboardPassword(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-test",
		"DASHBOARD_PASSWORD": "secret123",
	})
	require.NoError(t, err)
	assert.Equal(t, "secret123", cfg.DashboardPassword)
}

func TestLoad_DashboardPasswordOptional(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-test",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.DashboardPassword)
}

func TestLoad_AgentsDefaultPathDefault(t *testing.T) {
	// given
	// ... env without AGENTS_DEFAULT_PATH set
	dir := t.TempDir()
	env := map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       dir,
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-test",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... AgentsDefaultPath defaults to the docker bundle location
	require.NoError(t, err)
	assert.Equal(t, "/etc/claudecord/AGENTS.md.default", cfg.AgentsDefaultPath)
}

func TestLoad_AgentsDefaultPathOverride(t *testing.T) {
	// given
	// ... env with AGENTS_DEFAULT_PATH set
	dir := t.TempDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        dir,
		"ALLOWED_USERS":       "123",
		"CLAUDECORD_API_KEY":  "sk-test",
		"AGENTS_DEFAULT_PATH": "/custom/AGENTS.md",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... AgentsDefaultPath uses the override
	require.NoError(t, err)
	assert.Equal(t, "/custom/AGENTS.md", cfg.AgentsDefaultPath)
}

func TestLoad_MinimaxAPIKey(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-test",
		"MINIMAX_API_KEY":    "minimax-test-key",
	})
	require.NoError(t, err)
	assert.Equal(t, "minimax-test-key", cfg.MinimaxAPIKey)
}

func TestLoad_MinimaxAPIKeyOptional(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-test",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.MinimaxAPIKey)
}

// --- Model tests ---

func TestLoad_ModelDefaultsToSonnetWithoutBaseURL(t *testing.T) {
	cfg, err := Load(validDiscordEnv())
	require.NoError(t, err)
	assert.Equal(t, DefaultModel, cfg.Model)
}

func TestLoad_ModelDefaultsToKimiWhenBaseURLSet(t *testing.T) {
	env := validDiscordEnv()
	env["CLAUDECORD_BASE_URL"] = "https://kimi.example.com"
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, DefaultKimiModel, cfg.Model)
}

func TestLoad_ModelExplicitWithBaseURL(t *testing.T) {
	env := validDiscordEnv()
	env["CLAUDECORD_BASE_URL"] = "https://kimi.example.com"
	env["MODEL"] = "kimi-k2.6"
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, "kimi-k2.6", cfg.Model)
}

// --- WhatsAppMediaDir tests ---

func TestLoad_WhatsAppMediaDirDefaultsUnderFirstAllowedDir(t *testing.T) {
	env := validWhatsAppEnv()
	delete(env, "WHATSAPP_MEDIA_DIR")
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, env["ALLOWED_DIRS"]+"/wa-media", cfg.WhatsAppMediaDir)

	info, err := os.Stat(cfg.WhatsAppMediaDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_WhatsAppMediaDirMustBeInsideAllowedDirs(t *testing.T) {
	dir := mustTempMediaDir()
	env := map[string]string{
		"WHATSAPP_ALLOWED_SENDERS": "123456@lid",
		"ALLOWED_DIRS":             dir,
		"CLAUDECORD_API_KEY":       "sk-test",
		"WHATSAPP_MEDIA_DIR":       "/somewhere/else",
	}
	_, err := Load(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must live under ALLOWED_DIRS")
}

func TestLoad_WhatsAppMediaDirCreatedOnLoad(t *testing.T) {
	cfg, err := Load(validWhatsAppEnv())
	require.NoError(t, err)
	info, err := os.Stat(cfg.WhatsAppMediaDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_WhatsAppMediaDirNotRequiredWhenWhatsAppDisabled(t *testing.T) {
	cfg, err := Load(validDiscordEnv())
	require.NoError(t, err)
	assert.Empty(t, cfg.WhatsAppMediaDir)
}

// --- MemoryDir tests ---

func TestLoad_MemoryDirDefaultsUnderFirstAllowedDir(t *testing.T) {
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":      "tok",
		"ALLOWED_USERS":      "1",
		"ALLOWED_DIRS":       dir,
		"CLAUDECORD_API_KEY": "sk-test",
	}
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, dir+"/claudecord-memory", cfg.MemoryDir)

	info, err := os.Stat(cfg.MemoryDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_MemoryDirOverride(t *testing.T) {
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":      "tok",
		"ALLOWED_USERS":      "1",
		"ALLOWED_DIRS":       dir,
		"CLAUDECORD_API_KEY": "sk-test",
		"MEMORY_DIR":         dir + "/notes",
	}
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, dir+"/notes", cfg.MemoryDir)

	info, err := os.Stat(cfg.MemoryDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_MemoryDirMustBeInsideAllowedDirs(t *testing.T) {
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":      "tok",
		"ALLOWED_USERS":      "1",
		"ALLOWED_DIRS":       dir,
		"CLAUDECORD_API_KEY": "sk-test",
		"MEMORY_DIR":         "/somewhere/else",
	}
	_, err := Load(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must live under ALLOWED_DIRS")
}
