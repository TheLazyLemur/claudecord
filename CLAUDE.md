# Claudecord

Discord bot integrating Claude Code CLI via stdio protocol.

## What It Is

- Single-user, single-server Discord bot (user-restricted via `ALLOWED_USERS` env var)
- `@claude` as first word triggers bot
- `/new-session` starts fresh session, subsequent `@claude` continues it
- Uses Claude CLI's built-in session management (no disk persistence)
- Threads for long messages/responses
- Auto-approve tools with directory restrictions via hooks
- Allowed directories configurable via env var, applied recursively

## Architecture

Hexagonal with simplified layout:
```
./cmd/claudecord
./internal/core
./internal/core/interfaces.go
./internal/db
./internal/handler (discord handlers)
```

## Dependencies

- discordgo for Discord
- slog (stdlib) for logging
- pkg/errors for error wrapping
- testify/mock for test mocks

## Config

Env vars:
- `DISCORD_TOKEN` - Discord bot token (required)
- `ALLOWED_DIRS` - Comma-separated list of allowed directories (required)
- `ALLOWED_USERS` - Comma-separated Discord user IDs allowed to use bot (required)
- `CLAUDE_CWD` - Default working directory for Claude CLI (optional, defaults to first allowed dir)

## Coding Rules

- TDD required - write failing test first
- Minimal comments - only when logic isn't obvious
- Unit tests only - mock CLI interface, no integration tests against real CLI
- Interfaces for all external dependencies (CLI, Discord, etc)

## CLI Protocol Rules

- On initialize: return immediately after receiving `control_response` - don't wait for `system.init` (it comes after first user message)
- `Receive()` returns a persistent channel - call once, reuse for all messages in session
- Serialize message handling with mutex - CLI can only process one turn at a time
- Accumulate text from multiple `assistant` messages, separate with newlines
- `result` message signals turn complete - post accumulated response then

## Discord Rules

- Register handlers BEFORE `dg.Open()` for message events to work
- Need `IntentsGuildMessages | IntentMessageContent` for message content
- Bot invite URL must include `bot` scope (not just app auth) - use: `?scope=bot%20applications.commands`
- Role mentions (`<@&ID>`) differ from user mentions (`<@ID>`) - bot only responds to user mentions

## Reference

Claude CLI stdio protocol docs: https://github.com/TheLazyLemur/claude-code-stdio-protocol
