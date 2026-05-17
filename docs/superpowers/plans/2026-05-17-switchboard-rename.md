# Switchboard Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the project from `claudecord` to `switchboard` across code, build files, and docs.

**Architecture:** Mechanical rename of the Go module path, `cmd/` directory, identifiers, and strings. Environment variables carrying the old name are renamed but keep a legacy fallback with a deprecation warning, so existing deployments keep working. The memory-directory default also falls back to the old `claudecord-memory` folder when it already exists.

**Tech Stack:** Go 1.26, `slog`, testify, Docker, Fly.io.

**Reference spec:** `docs/superpowers/specs/2026-05-17-switchboard-rename-design.md`

**Platform note:** Commands assume macOS `sed` (`sed -i ''`). On Linux use `sed -i`.

---

### Task 1: Rename module path and `cmd/` directory

**Files:**
- Modify: `go.mod`
- Rename: `cmd/claudecord/` → `cmd/switchboard/` (4 files: `main.go`, `server.go`, `discord.go`, `whatsapp.go`)
- Modify: every `.go` file importing `github.com/TheLazyLemur/claudecord` (28 files)

- [ ] **Step 1: Rewrite the module path**

```bash
go mod edit -module github.com/TheLazyLemur/switchboard
```

- [ ] **Step 2: Rename the command directory**

```bash
git mv cmd/claudecord cmd/switchboard
```

- [ ] **Step 3: Rewrite all internal import paths**

```bash
grep -rl "TheLazyLemur/claudecord" --include="*.go" . \
  | xargs sed -i '' 's|TheLazyLemur/claudecord|TheLazyLemur/switchboard|g'
```

- [ ] **Step 4: Verify the build compiles**

Run: `go build ./...`
Expected: no output, exit 0. (Config string/env changes come in later tasks; the build must still pass here because only import paths changed.)

- [ ] **Step 5: Verify no import path references remain**

Run: `grep -rn "TheLazyLemur/claudecord" --include="*.go" .`
Expected: no output.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "refactor: rename module path and cmd dir to switchboard"
```

---

### Task 2: Rename environment variables with legacy fallback

Renames `CLAUDECORD_API_KEY` → `SWITCHBOARD_API_KEY`, `CLAUDECORD_BASE_URL` → `SWITCHBOARD_BASE_URL`, `CLAUDE_CWD` → `AGENT_CWD`, and the config field `ClaudeCWD` → `AgentCWD`. A helper reads the new name first and falls back to the old name with a `slog` deprecation warning.

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `cmd/switchboard/server.go:30`, `cmd/switchboard/main.go:42`, `cmd/switchboard/main.go:69`

- [ ] **Step 1: Migrate existing config tests to the new env var names and field**

```bash
sed -i '' \
  -e 's/CLAUDECORD_API_KEY/SWITCHBOARD_API_KEY/g' \
  -e 's/CLAUDECORD_BASE_URL/SWITCHBOARD_BASE_URL/g' \
  -e 's/"CLAUDE_CWD"/"AGENT_CWD"/g' \
  -e 's/cfg\.ClaudeCWD/cfg.AgentCWD/g' \
  -e 's/TestLoad_ClaudeCWDOverride/TestLoad_AgentCWDOverride/g' \
  -e 's/claudecord-cfg-test-/switchboard-cfg-test-/g' \
  -e 's#/etc/claudecord/#/etc/switchboard/#g' \
  internal/config/config_test.go
```

- [ ] **Step 2: Add the failing fallback tests**

Append to `internal/config/config_test.go`:

```go
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
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/config/`
Expected: FAIL — `cfg.AgentCWD` undefined and `SWITCHBOARD_API_KEY` not read by `Load`.

- [ ] **Step 4: Rename the config struct field**

In `internal/config/config.go`, change line 21 `ClaudeCWD    string` to `AgentCWD     string`, and update the comment on line 55 from `<ClaudeCWD>/AGENTS.md` to `<AgentCWD>/AGENTS.md`. Update the comments on lines 24 and 26 to name the new env vars (`SWITCHBOARD_API_KEY`, `SWITCHBOARD_BASE_URL`).

- [ ] **Step 5: Add the `envOrLegacy` helper**

Add `"log/slog"` to the imports in `internal/config/config.go`, then add this function after `splitAndTrim`:

```go
// envOrLegacy returns env[key], falling back to env[legacyKey] when key is
// unset. Using the legacy key logs a one-line deprecation warning.
func envOrLegacy(env map[string]string, key, legacyKey string) string {
	if v := env[key]; v != "" {
		return v
	}
	if v := env[legacyKey]; v != "" {
		slog.Warn("deprecated env var; rename it", "deprecated", legacyKey, "use", key)
		return v
	}
	return ""
}
```

- [ ] **Step 6: Use the helper in `Load`**

In `internal/config/config.go`:
- Line 116: `claudeCwd := env["CLAUDE_CWD"]` → `agentCwd := envOrLegacy(env, "AGENT_CWD", "CLAUDE_CWD")`, and update the following two lines that reference `claudeCwd` to `agentCwd`.
- Line 126: `apiKey := env["CLAUDECORD_API_KEY"]` → `apiKey := envOrLegacy(env, "SWITCHBOARD_API_KEY", "CLAUDECORD_API_KEY")`.
- Line 128: error text `"CLAUDECORD_API_KEY required"` → `"SWITCHBOARD_API_KEY required"`.
- Line 131: `baseURL := env["CLAUDECORD_BASE_URL"]` → `baseURL := envOrLegacy(env, "SWITCHBOARD_BASE_URL", "CLAUDECORD_BASE_URL")`.
- Line 169: `/etc/claudecord/AGENTS.md.default` → `/etc/switchboard/AGENTS.md.default`.
- Line 196: struct literal field `ClaudeCWD: claudeCwd,` → `AgentCWD: agentCwd,`.

- [ ] **Step 7: Add both new and legacy keys to `LoadFromEnv`**

In the `env` map in `LoadFromEnv` (`internal/config/config.go`), replace the `CLAUDE_CWD`, `CLAUDECORD_API_KEY`, and `CLAUDECORD_BASE_URL` entries with all six keys:

```go
		"AGENT_CWD":                os.Getenv("AGENT_CWD"),
		"CLAUDE_CWD":               os.Getenv("CLAUDE_CWD"),
		"SWITCHBOARD_API_KEY":      os.Getenv("SWITCHBOARD_API_KEY"),
		"CLAUDECORD_API_KEY":       os.Getenv("CLAUDECORD_API_KEY"),
		"SWITCHBOARD_BASE_URL":     os.Getenv("SWITCHBOARD_BASE_URL"),
		"CLAUDECORD_BASE_URL":      os.Getenv("CLAUDECORD_BASE_URL"),
```

- [ ] **Step 8: Update the `cmd/switchboard` consumers of the renamed field**

In `cmd/switchboard/server.go:30` and `cmd/switchboard/main.go` lines 42 and 69, replace `cfg.ClaudeCWD` with `cfg.AgentCWD`:

```bash
sed -i '' 's/cfg\.ClaudeCWD/cfg.AgentCWD/g' cmd/switchboard/server.go cmd/switchboard/main.go
```

- [ ] **Step 9: Run the tests to verify they pass**

Run: `go test ./internal/config/ ./cmd/...`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/config/ cmd/switchboard/
git commit -m "refactor: rename env vars to SWITCHBOARD_* with legacy fallback"
```

---

### Task 3: Memory directory default with legacy fallback

When `MEMORY_DIR` is unset, the default becomes `<first ALLOWED_DIR>/switchboard-memory`, but falls back to an existing `<first ALLOWED_DIR>/claudecord-memory` directory if present.

**Files:**
- Modify: `internal/config/config.go:184-187`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Update the existing default-dir test and add fallback tests**

In `internal/config/config_test.go`, in `TestLoad_MemoryDirDefaultsUnderFirstAllowedDir`, change the assertion from `dir+"/claudecord-memory"` to `dir+"/switchboard-memory"`.

Then append:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoad_MemoryDir`
Expected: FAIL — default is still `claudecord-memory`.

- [ ] **Step 3: Implement the default-dir resolver**

In `internal/config/config.go`, replace lines 184-187:

```go
	memoryDir := env["MEMORY_DIR"]
	if memoryDir == "" {
		memoryDir = filepath.Join(allowedDirs[0], "claudecord-memory")
	}
```

with:

```go
	memoryDir := env["MEMORY_DIR"]
	if memoryDir == "" {
		memoryDir = defaultMemoryDir(allowedDirs[0])
	}
```

Then add this function after `pathInsideAllowedDirs`:

```go
// defaultMemoryDir picks the memory directory when MEMORY_DIR is unset. It
// prefers <base>/switchboard-memory but falls back to a pre-existing legacy
// <base>/claudecord-memory directory so renamed installs keep their memory.
func defaultMemoryDir(base string) string {
	newDir := filepath.Join(base, "switchboard-memory")
	if info, err := os.Stat(newDir); err == nil && info.IsDir() {
		return newDir
	}
	legacyDir := filepath.Join(base, "claudecord-memory")
	if info, err := os.Stat(legacyDir); err == nil && info.IsDir() {
		slog.Warn("using legacy memory directory; rename it to switchboard-memory", "dir", legacyDir)
		return legacyDir
	}
	return newDir
}
```

Also update the doc comment on the `MemoryDir` field (line 51-52) from `claudecord-memory` to `switchboard-memory`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/config/`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): default memory dir to switchboard-memory with legacy fallback"
```

---

### Task 4: Rename remaining code identifiers and strings

Covers the Discord thread-name fallback, the dashboard session cookie, the builtin-skills directory, dashboard HTML, and cosmetic `claude` references in test data.

**Files:**
- Modify: `internal/channels/discord/plugin.go:248`
- Modify: `internal/dashboard/auth.go:10`
- Modify: `internal/skills/embed.go:51`
- Modify: `internal/skills/embed_test.go:77`
- Modify: `internal/dashboard/static/index.html:6`
- Modify: `internal/dashboard/static/login.html:6,15`
- Modify: `internal/dashboard/server_test.go:94`
- Modify: `internal/channels/discord/translate_test.go`
- Modify: `internal/channels/discord/plugin_test.go`

- [ ] **Step 1: Rename the Discord thread-name fallback**

In `internal/channels/discord/plugin.go:248`, change `t = "Claude"` to `t = "Switchboard"`.

- [ ] **Step 2: Rename the dashboard session cookie**

In `internal/dashboard/auth.go:10`, change `const sessionCookieName = "claudecord_session"` to `const sessionCookieName = "switchboard_session"`. (Existing dashboard sessions will need a re-login; acceptable.)

- [ ] **Step 3: Rename the builtin-skills directory**

In `internal/skills/embed.go:51`, change `filepath.Join(home, ".claudecord", "skills", "builtin")` to `filepath.Join(home, ".switchboard", "skills", "builtin")`.

In `internal/skills/embed_test.go:77`, change the assertion `assert.Contains(t, dir, ".claudecord")` to `assert.Contains(t, dir, ".switchboard")`.

- [ ] **Step 4: Rename the dashboard HTML titles**

```bash
sed -i '' 's/Claudecord/Switchboard/g' \
  internal/dashboard/static/index.html \
  internal/dashboard/static/login.html
```

In `internal/dashboard/server_test.go:94`, change `assert.Contains(t, rec.Body.String(), "Claudecord Dashboard")` to `assert.Contains(t, rec.Body.String(), "Switchboard Dashboard")`.

- [ ] **Step 5: Rename cosmetic `@claude` references in Discord tests**

```bash
sed -i '' \
  -e 's/@claude/@switchboard/g' \
  -e 's/RejectsLiteralAtClaude/RejectsLiteralAtMention/g' \
  internal/channels/discord/translate_test.go \
  internal/channels/discord/plugin_test.go
```

(These are example message contents and a test name; the trigger logic in `stripMention` matches `<@botID>`, not literal text, so behaviour is unchanged.)

- [ ] **Step 6: Run the affected tests**

Run: `go test ./internal/channels/discord/ ./internal/dashboard/ ./internal/skills/`
Expected: PASS.

- [ ] **Step 7: Verify no `claude` identifiers remain in code**

Run: `grep -rni "claudecord\|claude" --include="*.go" --include="*.html" internal/ cmd/ | grep -v "claude-sonnet"`
Expected: no output.

- [ ] **Step 8: Commit**

```bash
git add internal/ cmd/
git commit -m "refactor: rename remaining claude identifiers to switchboard"
```

---

### Task 5: Rename build and deployment files

**Files:**
- Modify: `Dockerfile` (lines 9, 45, 48, 72)
- Modify: `Makefile` (lines 2, 7, 8)
- Modify: `fly.toml` (lines 1, 13)
- Modify: `.gitignore` (line 2)

- [ ] **Step 1: Update the Dockerfile**

```bash
sed -i '' \
  -e 's#go build -o /claudecord ./cmd/claudecord#go build -o /switchboard ./cmd/switchboard#' \
  -e 's#COPY --from=builder /claudecord /usr/local/bin/claudecord#COPY --from=builder /switchboard /usr/local/bin/switchboard#' \
  -e 's#/etc/claudecord/AGENTS.md.default#/etc/switchboard/AGENTS.md.default#' \
  -e 's/^exec claudecord$/exec switchboard/' \
  Dockerfile
```

Verify: `grep -n claudecord Dockerfile` → no output.

- [ ] **Step 2: Update the Makefile**

```bash
sed -i '' \
  -e 's#./cmd/claudecord#./cmd/switchboard#' \
  -e 's/IMAGE_NAME := claudecord/IMAGE_NAME := switchboard/' \
  -e 's/CONTAINER_NAME := claudecord/CONTAINER_NAME := switchboard/' \
  Makefile
```

Note: do NOT change the `-v ~/.claude:/root/.claude` mount — that is Claude Code's own config directory.
Verify: `grep -n "claudecord" Makefile` → no output.

- [ ] **Step 3: Update fly.toml (cosmetic + env only)**

In `fly.toml`:
- Line 1 comment: change `generated for claudecord` to `generated for switchboard`.
- Line 13: change `CLAUDE_CWD = '/root/workspace'` to `AGENT_CWD = '/root/workspace'`.

Do NOT change: `app = 'claudecord'` (line 6), `source = 'claude_config'` (line 29), or the `/root/claudecord-memory` paths (lines 12, 14) — these are deployment identity, handled separately by the user.

- [ ] **Step 4: Update .gitignore**

In `.gitignore:2`, change `/claudecord` to `/switchboard` (the built binary). Leave `.claude/worktrees/` untouched.

- [ ] **Step 5: Verify the Docker build**

Run: `docker build -t switchboard .`
Expected: build succeeds. (Repo rule: Dockerfile changes are verified locally because `go build` does not exercise the build context.)

- [ ] **Step 6: Commit**

```bash
git add Dockerfile Makefile fly.toml .gitignore
git commit -m "build: rename build and deployment files to switchboard"
```

---

### Task 6: Rename documentation

**Files:**
- Modify: `README.md`
- Modify: `CLAUDE.md`
- Modify: `docs/skills-roadmap.md`, `docs/whatsapp-media-plan.md`, `docs/worktree-sessions.md`

- [ ] **Step 1: Replace `claudecord` mentions in active docs**

```bash
sed -i '' \
  -e 's/claudecord/switchboard/g' \
  -e 's/Claudecord/Switchboard/g' \
  README.md docs/skills-roadmap.md docs/whatsapp-media-plan.md docs/worktree-sessions.md
```

- [ ] **Step 2: Update CLAUDE.md body**

In `CLAUDE.md`, manually update:
- The `# Claudecord` heading → `# Switchboard`.
- The opening description and "What It Is" section — reword so it does not describe the project as a "Discord/WhatsApp bot" with Discord primary; describe it as a bot bridging multiple messaging channels (Discord, WhatsApp, dashboard) to swappable agents.
- The env var names in the Config section: `CLAUDECORD_API_KEY` → `SWITCHBOARD_API_KEY`, `CLAUDECORD_BASE_URL` → `SWITCHBOARD_BASE_URL`, `CLAUDE_CWD` → `AGENT_CWD`. Add a note that the old names still work with a deprecation warning.
- The `claudecord-memory` default in the `MEMORY_DIR` description → `switchboard-memory`, noting the legacy fallback.
- Any remaining `claudecord` path or name references.

Do NOT rename the `CLAUDE.md` file itself — it is Claude Code's instructions file.

- [ ] **Step 3: Verify active docs are clean**

Run: `grep -rni "claudecord" README.md CLAUDE.md docs/skills-roadmap.md docs/whatsapp-media-plan.md docs/worktree-sessions.md`
Expected: no output.

(Historical files under `docs/superpowers/plans/` and `docs/superpowers/specs/` describing past work keep the old name as a record. The `docs/superpowers/specs/2026-05-17-switchboard-rename-design.md` spec intentionally references `claudecord` as the old name — leave it.)

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md docs/
git commit -m "docs: rename claudecord to switchboard"
```

---

### Task 7: Final verification

**Files:** none modified — verification only.

- [ ] **Step 1: Build the whole module**

Run: `go build ./...`
Expected: no output, exit 0.

- [ ] **Step 2: Run the full test suite**

Run: `go test ./...`
Expected: all packages PASS.

- [ ] **Step 3: Run go vet**

Run: `go vet ./...`
Expected: no output.

- [ ] **Step 4: Confirm no stray references remain**

Run: `grep -rni "claudecord" --exclude-dir=.git --exclude-dir=docs/superpowers .`
Expected: no output. (The `docs/superpowers/` tree is excluded because historical plans/specs and the rename spec legitimately reference the old name.)

- [ ] **Step 5: Confirm the legacy env var fallback works end to end**

Run:
```bash
CLAUDECORD_API_KEY=sk-test DISCORD_TOKEN=x ALLOWED_USERS=1 ALLOWED_DIRS=/tmp \
  go run ./cmd/switchboard 2>&1 | head -5
```
Expected: a log line warning `deprecated env var` naming `CLAUDECORD_API_KEY` and `SWITCHBOARD_API_KEY` (the process may then fail on the invalid Discord token — that is fine; only the deprecation warning is being checked).

- [ ] **Step 6: Commit any final cleanup**

If steps 1-5 surfaced nothing to fix, no commit is needed. Otherwise fix inline and:

```bash
git add -A
git commit -m "chore: switchboard rename cleanup"
```
