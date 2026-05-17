# Switchboard rename — design

**Date:** 2026-05-17
**Status:** approved

## Goal

Rename the project from `claudecord` to `switchboard`. The old name fused
"claude" + "discord"; both halves are now wrong — agents are swappable and no
single channel is primary. `switchboard` reflects what the project is: a hub
that connects any messaging channel to any agent.

Scope is **code, build, and docs**. The GitHub repository rename and the Fly.io
deployment migration are handled by the user separately.

## Non-goals

- Renaming the Fly.io app or its persistent volume.
- Renaming the `DefaultModel` constant value (`claude-sonnet-4-20250514`) — it
  is a real Anthropic model ID, not the project name.
- Renaming the `~/.claude` mount in the Makefile — that is Claude Code's own
  config directory, unrelated to the project.
- Renaming the `CLAUDE.md` file — it is Claude Code's instructions file; only
  `claudecord` mentions inside its body change.

## Changes

### 1. Module path & directory

- `go.mod`: module `github.com/TheLazyLemur/claudecord` →
  `github.com/TheLazyLemur/switchboard`.
- Rewrite the import path in every `.go` file that imports an internal package.
- Rename directory `cmd/claudecord/` → `cmd/switchboard/`.

### 2. Environment variables — rename with fallback

A small helper in `internal/config` reads the new variable name first and falls
back to the old name, emitting a `slog` deprecation warning when the old name is
used. This keeps existing deployments running through the cutover.

| Old | New |
|-----|-----|
| `CLAUDECORD_API_KEY` | `SWITCHBOARD_API_KEY` |
| `CLAUDECORD_BASE_URL` | `SWITCHBOARD_BASE_URL` |
| `CLAUDE_CWD` | `AGENT_CWD` |

### 3. Identifiers & strings

- Config struct field `ClaudeCWD` → `AgentCWD`.
- Bundled AGENTS.md default path `/etc/claudecord/AGENTS.md.default` →
  `/etc/switchboard/AGENTS.md.default`.
- Discord thread-name fallback string `"Claude"` → `"Switchboard"`
  (`internal/channels/discord/plugin.go`).

### 4. Memory directory default — existence fallback

When `MEMORY_DIR` is unset, the default was `<first ALLOWED_DIR>/claudecord-memory`.
New behaviour:

1. Compute the new default `<first ALLOWED_DIR>/switchboard-memory`.
2. If that directory does not exist **and** `<first ALLOWED_DIR>/claudecord-memory`
   does exist, use the old path and log a deprecation warning.
3. Otherwise use the new path.

This preserves memory for installs that relied on the unset default.

### 5. Build & deployment files

- **Dockerfile** — build output `/claudecord`, `./cmd/claudecord`, the binary
  copy, `/etc/claudecord/`, and `exec claudecord` all become `switchboard`.
- **Makefile** — `./cmd/claudecord` → `./cmd/switchboard`; `IMAGE_NAME` and
  `CONTAINER_NAME` → `switchboard`. The `~/.claude` mount is left untouched.
- **fly.toml** — cosmetic + env only: update the header comment and rename
  `CLAUDE_CWD` → `AGENT_CWD`. The Fly `app` name, the volume `source`
  (`claude_config`), and the `/root/claudecord-memory` paths are **left as-is**
  to avoid a deployment migration.

### 6. Documentation

- `README.md`, `CLAUDE.md` body, and `docs/**` — replace `claudecord` mentions.
- Reword the "Claude/Discord bot" framing to channel- and agent-agnostic
  language consistent with the project's actual scope.
- Historical design docs under `docs/superpowers/` describing past work may keep
  the old name where it is part of that work's record.

## Verification

- `go build ./...` and `go test ./...` pass.
- `docker build` succeeds locally (repo rule: Dockerfile changes verified
  locally, since `go build` does not exercise the build context).
- `grep -ri claudecord` returns nothing outside historical design docs.
- Manual: bot starts with the old env var names and logs the deprecation
  warning; starts cleanly with the new names and no warning.
