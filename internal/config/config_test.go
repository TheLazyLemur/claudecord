package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDiscordEnv() map[string]string {
	return map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
	}
}

func validWhatsAppEnv() map[string]string {
	dir := mustTempMediaDir()
	return map[string]string{
		"WHATSAPP_ALLOWED_SENDERS": "123456@lid",
		"ALLOWED_DIRS":             dir,
		"SWITCHBOARD_API_KEY":      "sk-test",
		"WHATSAPP_MEDIA_DIR":       dir + "/media",
	}
}

func mustTempMediaDir() string {
	dir, err := os.MkdirTemp("", "switchboard-cfg-test-")
	if err != nil {
		panic(err)
	}
	return dir
}

// --- Platform requirement tests ---

func TestLoad_RequiresAtLeastOnePlatform(t *testing.T) {
	_, err := Load(map[string]string{
		"ALLOWED_DIRS":        "/tmp",
		"SWITCHBOARD_API_KEY": "sk-test",
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
		"DISCORD_TOKEN":       "token",
		"ALLOWED_DIRS":        "/tmp",
		"SWITCHBOARD_API_KEY": "sk-test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ALLOWED_USERS required")
}

func TestLoad_DiscordValidatesAllowedUsersNumeric(t *testing.T) {
	_, err := Load(map[string]string{
		"DISCORD_TOKEN":       "token",
		"ALLOWED_DIRS":        "/tmp",
		"ALLOWED_USERS":       "notanumber",
		"SWITCHBOARD_API_KEY": "sk-test",
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
	assert.Contains(t, err.Error(), "SWITCHBOARD_API_KEY required")
}

func TestLoad_Success(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user, /tmp",
		"ALLOWED_USERS":       "123, 456",
		"SWITCHBOARD_API_KEY": "sk-test-key",
	})
	require.NoError(t, err)

	assert.Equal(t, "mytoken", cfg.DiscordToken)
	assert.Equal(t, []string{"/home/user", "/tmp"}, cfg.AllowedDirs)
	assert.Equal(t, []string{"123", "456"}, cfg.AllowedUsers)
	assert.Equal(t, "/home/user", cfg.AgentCWD)
	assert.Equal(t, "sk-test-key", cfg.APIKey)
}

func TestLoad_BaseURLSet(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":        "mytoken",
		"ALLOWED_DIRS":         "/home/user",
		"ALLOWED_USERS":        "123",
		"SWITCHBOARD_API_KEY":  "sk-test-key",
		"SWITCHBOARD_BASE_URL": "https://proxy.example.com",
		"MODEL":                "kimi-k2.6",
	})
	require.NoError(t, err)
	assert.Equal(t, "https://proxy.example.com", cfg.BaseURL)
}

func TestLoad_AgentCWDOverride(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"AGENT_CWD":           "/custom/path",
		"SWITCHBOARD_API_KEY": "sk-test",
	})
	require.NoError(t, err)
	assert.Equal(t, "/custom/path", cfg.AgentCWD)
}

func TestLoad_DashboardPassword(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
		"DASHBOARD_PASSWORD":  "secret123",
	})
	require.NoError(t, err)
	assert.Equal(t, "secret123", cfg.DashboardPassword)
}

func TestLoad_DashboardPasswordOptional(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.DashboardPassword)
}

func TestLoad_AgentsDefaultPathDefault(t *testing.T) {
	// given
	// ... env without AGENTS_DEFAULT_PATH set
	dir := t.TempDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        dir,
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... AgentsDefaultPath defaults to the docker bundle location
	require.NoError(t, err)
	assert.Equal(t, "/etc/switchboard/AGENTS.md.default", cfg.AgentsDefaultPath)
}

func TestLoad_AgentsDefaultPathOverride(t *testing.T) {
	// given
	// ... env with AGENTS_DEFAULT_PATH set
	dir := t.TempDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        dir,
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
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

func TestLoad_WebSearchAPIKey(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
		"WEB_SEARCH_API_KEY":  "brave-test-key",
	})
	require.NoError(t, err)
	assert.Equal(t, "brave-test-key", cfg.WebSearchAPIKey)
}

func TestLoad_WebSearchAPIKeyOptional(t *testing.T) {
	cfg, err := Load(map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
	})
	require.NoError(t, err)
	assert.Empty(t, cfg.WebSearchAPIKey)
}

// --- Model tests ---

func TestLoad_ModelDefaultsToSonnetWithoutBaseURL(t *testing.T) {
	cfg, err := Load(validDiscordEnv())
	require.NoError(t, err)
	assert.Equal(t, DefaultModel, cfg.Model)
}

func TestLoad_ModelDefaultsToKimiWhenBaseURLSet(t *testing.T) {
	env := validDiscordEnv()
	env["SWITCHBOARD_BASE_URL"] = "https://kimi.example.com"
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, DefaultKimiModel, cfg.Model)
}

func TestLoad_ModelExplicitWithBaseURL(t *testing.T) {
	env := validDiscordEnv()
	env["SWITCHBOARD_BASE_URL"] = "https://kimi.example.com"
	env["MODEL"] = "kimi-k2.6"
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, "kimi-k2.6", cfg.Model)
}

// --- Thinking budget tests ---

func thinkingTestEnv(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	return map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        dir,
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
	}
}

func TestLoad_ThinkingBudgetDefaultsToZero(t *testing.T) {
	// given
	// ... no THINKING_BUDGET_TOKENS set
	env := thinkingTestEnv(t)

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... thinking is disabled (budget == 0)
	require.NoError(t, err)
	assert.Equal(t, 0, cfg.ThinkingBudgetTokens)
}

func TestLoad_ThinkingBudgetParsed(t *testing.T) {
	// given
	// ... THINKING_BUDGET_TOKENS set to a valid value
	env := thinkingTestEnv(t)
	env["THINKING_BUDGET_TOKENS"] = "4096"

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... the budget is parsed
	require.NoError(t, err)
	assert.Equal(t, 4096, cfg.ThinkingBudgetTokens)
}

func TestLoad_ThinkingBudgetRejectsBelowMinimum(t *testing.T) {
	// given
	// ... THINKING_BUDGET_TOKENS below Anthropic's 1024 minimum
	env := thinkingTestEnv(t)
	env["THINKING_BUDGET_TOKENS"] = "1023"

	// when
	// ... config is loaded
	_, err := Load(env)

	// then
	// ... an error is returned
	require.Error(t, err)
	assert.Contains(t, err.Error(), "THINKING_BUDGET_TOKENS")
}

func TestLoad_ThinkingBudgetRejectsNonNumeric(t *testing.T) {
	// given
	// ... THINKING_BUDGET_TOKENS set to a non-numeric value
	env := thinkingTestEnv(t)
	env["THINKING_BUDGET_TOKENS"] = "yes"

	// when
	// ... config is loaded
	_, err := Load(env)

	// then
	// ... an error is returned
	require.Error(t, err)
	assert.Contains(t, err.Error(), "THINKING_BUDGET_TOKENS")
}

// --- WhatsAppMediaDir tests ---

func TestLoad_WhatsAppMediaDirDefaultsUnderFirstAllowedDir(t *testing.T) {
	env := validWhatsAppEnv()
	delete(env, "WHATSAPP_MEDIA_DIR")
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, env["ALLOWED_DIRS"]+"/wa-media", cfg.WhatsAppMediaDir)
	require.NoError(t, cfg.EnsureDirs())

	info, err := os.Stat(cfg.WhatsAppMediaDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_WhatsAppMediaDirMustBeInsideAllowedDirs(t *testing.T) {
	dir := mustTempMediaDir()
	env := map[string]string{
		"WHATSAPP_ALLOWED_SENDERS": "123456@lid",
		"ALLOWED_DIRS":             dir,
		"SWITCHBOARD_API_KEY":      "sk-test",
		"WHATSAPP_MEDIA_DIR":       "/somewhere/else",
	}
	_, err := Load(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must live under ALLOWED_DIRS")
}

func TestLoad_WhatsAppMediaDirCreatedByEnsureDirs(t *testing.T) {
	cfg, err := Load(validWhatsAppEnv())
	require.NoError(t, err)
	require.NoError(t, cfg.EnsureDirs())
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
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "1",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
	}
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, dir+"/switchboard-memory", cfg.MemoryDir)
	require.NoError(t, cfg.EnsureDirs())

	info, err := os.Stat(cfg.MemoryDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_MemoryDirOverride(t *testing.T) {
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "1",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
		"MEMORY_DIR":          dir + "/notes",
	}
	cfg, err := Load(env)
	require.NoError(t, err)
	assert.Equal(t, dir+"/notes", cfg.MemoryDir)
	require.NoError(t, cfg.EnsureDirs())

	info, err := os.Stat(cfg.MemoryDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_MemoryDirFallsBackToLegacyDir(t *testing.T) {
	// given
	// ... an allowed dir that already contains a claudecord-memory folder
	dir := mustTempMediaDir()
	require.NoError(t, os.Mkdir(dir+"/claudecord-memory", 0o700))
	env := map[string]string{
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "1",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
	}

	// when
	// ... config is loaded with MEMORY_DIR unset
	cfg, err := Load(env)

	// then
	// ... MemoryDir uses the existing legacy folder
	require.NoError(t, err)
	assert.Equal(t, dir+"/claudecord-memory", cfg.MemoryDir)
}

func TestLoad_MemoryDirPrefersNewDirWhenBothExist(t *testing.T) {
	// given
	// ... an allowed dir containing both legacy and new memory folders
	dir := mustTempMediaDir()
	require.NoError(t, os.Mkdir(dir+"/claudecord-memory", 0o700))
	require.NoError(t, os.Mkdir(dir+"/switchboard-memory", 0o700))
	env := map[string]string{
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "1",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
	}

	// when
	// ... config is loaded with MEMORY_DIR unset
	cfg, err := Load(env)

	// then
	// ... MemoryDir uses the new folder
	require.NoError(t, err)
	assert.Equal(t, dir+"/switchboard-memory", cfg.MemoryDir)
}

func TestLoad_MemoryDirMustBeInsideAllowedDirs(t *testing.T) {
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "1",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
		"MEMORY_DIR":          "/somewhere/else",
	}
	_, err := Load(env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must live under ALLOWED_DIRS")
}

// --- DiscordMediaDir tests ---

func TestLoad_DiscordMediaDirDefaultsUnderFirstAllowedDir(t *testing.T) {
	// given
	// ... a Discord-only config with no DISCORD_MEDIA_DIR set
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "123",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... DiscordMediaDir defaults to <first ALLOWED_DIR>/discord-media
	require.NoError(t, err)
	assert.Equal(t, dir+"/discord-media", cfg.DiscordMediaDir)
	require.NoError(t, cfg.EnsureDirs())
	info, err := os.Stat(cfg.DiscordMediaDir)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestLoad_DiscordMediaDirOverride(t *testing.T) {
	// given
	// ... a Discord-only config with DISCORD_MEDIA_DIR set explicitly
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "123",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
		"DISCORD_MEDIA_DIR":   dir + "/attachments",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... DiscordMediaDir uses the override
	require.NoError(t, err)
	assert.Equal(t, dir+"/attachments", cfg.DiscordMediaDir)
}

func TestLoad_DiscordMediaDirMustBeInsideAllowedDirs(t *testing.T) {
	// given
	// ... DISCORD_MEDIA_DIR set outside ALLOWED_DIRS
	dir := mustTempMediaDir()
	env := map[string]string{
		"DISCORD_TOKEN":       "tok",
		"ALLOWED_USERS":       "123",
		"ALLOWED_DIRS":        dir,
		"SWITCHBOARD_API_KEY": "sk-test",
		"DISCORD_MEDIA_DIR":   "/somewhere/else",
	}

	// when
	// ... config is loaded
	_, err := Load(env)

	// then
	// ... an error is returned
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must live under ALLOWED_DIRS")
}

func TestLoad_DiscordMediaDirNotSetWhenDiscordDisabled(t *testing.T) {
	// given
	// ... a WhatsApp-only config (no Discord token)
	env := validWhatsAppEnv()

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... DiscordMediaDir is empty
	require.NoError(t, err)
	assert.Empty(t, cfg.DiscordMediaDir)
}

// --- Env var rename / legacy fallback tests ---

func TestLoad_APIKeyFromLegacyEnvVar(t *testing.T) {
	// given
	// ... env with only the legacy CLAUDECORD_API_KEY set
	env := map[string]string{
		"DISCORD_TOKEN":      "mytoken",
		"ALLOWED_DIRS":       "/home/user",
		"ALLOWED_USERS":      "123",
		"CLAUDECORD_API_KEY": "sk-legacy",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... the legacy key still populates APIKey
	require.NoError(t, err)
	assert.Equal(t, "sk-legacy", cfg.APIKey)
}

func TestLoad_NewAPIKeyTakesPrecedenceOverLegacy(t *testing.T) {
	// given
	// ... env with both the new and legacy API key set
	env := map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-new",
		"CLAUDECORD_API_KEY":  "sk-legacy",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... the new key wins
	require.NoError(t, err)
	assert.Equal(t, "sk-new", cfg.APIKey)
}

func TestLoad_BaseURLFromLegacyEnvVar(t *testing.T) {
	// given
	// ... env with only the legacy CLAUDECORD_BASE_URL set
	env := map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
		"CLAUDECORD_BASE_URL": "https://legacy.example.com",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... the legacy key still populates BaseURL
	require.NoError(t, err)
	assert.Equal(t, "https://legacy.example.com", cfg.BaseURL)
}

func TestLoad_AgentCWDFromLegacyEnvVar(t *testing.T) {
	// given
	// ... env with only the legacy CLAUDE_CWD set
	env := map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
		"CLAUDE_CWD":          "/legacy/path",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... the legacy key still populates AgentCWD
	require.NoError(t, err)
	assert.Equal(t, "/legacy/path", cfg.AgentCWD)
}

func TestLoad_NewAgentCWDTakesPrecedenceOverLegacy(t *testing.T) {
	// given
	// ... env with both the new AGENT_CWD and legacy CLAUDE_CWD set
	env := map[string]string{
		"DISCORD_TOKEN":       "mytoken",
		"ALLOWED_DIRS":        "/home/user",
		"ALLOWED_USERS":       "123",
		"SWITCHBOARD_API_KEY": "sk-test",
		"AGENT_CWD":           "/new/path",
		"CLAUDE_CWD":          "/legacy/path",
	}

	// when
	// ... config is loaded
	cfg, err := Load(env)

	// then
	// ... the new var wins
	require.NoError(t, err)
	assert.Equal(t, "/new/path", cfg.AgentCWD)
}
