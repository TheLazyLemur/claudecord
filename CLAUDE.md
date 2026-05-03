# Claudecord

Discord bot integrating Claude Code CLI via stdio protocol.

## What It Is

- Single-user, single-server Discord bot (user-restricted via `ALLOWED_USERS` env var)
- `@claude` as first word triggers bot
- `/new-session` starts fresh session, subsequent `@claude` continues it
- Uses Claude CLI's built-in session management (no disk persistence)
- Threads for long messages/responses
- Fully autonomous: tools run without interactive prompts. The only safety net is path containment against `ALLOWED_DIRS`.
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
- `MODEL` - Anthropic model id. Defaults to `Kimi-for-Coding` when `CLAUDECORD_BASE_URL` is set, otherwise to a recent Sonnet. Override to use any other model id supported by the endpoint.
- `WHATSAPP_MEDIA_DIR` - Directory inbound WhatsApp attachments are decrypted into. Defaults to `<first ALLOWED_DIR>/wa-media` when `WHATSAPP_ALLOWED_SENDERS` is set; must live under one of `ALLOWED_DIRS` if overridden.

## WhatsApp media

- Inbound images and documents are decrypted into `WHATSAPP_MEDIA_DIR` and surfaced as `<attachment path mime original_name />` tags inside `<message>` blocks in the prompt body.
- Bursts (messages from the same chat within ~3s) are batched into a single dispatch.
- Image MIMEs: the model calls `Read` on the path; the tool returns an `image` `tool_result` block so the vision encoder fires.
- Other MIMEs: user-authored skills handle them, matching on the `mime` attribute.
- `Read` is auto-approved for paths under `WHATSAPP_MEDIA_DIR` regardless of `AUTO_APPROVE_WHATSAPP`, since the user explicitly uploaded the file.
- Size caps: images 10 MiB, docs 50 MiB. Oversized attachments are dropped with a "skipped (too large)" reply; siblings in the same burst still flow.

## Coding Rules

- TDD required - write failing test first
- Minimal comments - only when logic isn't obvious
- Unit tests only - mock CLI interface, no integration tests against real CLI
- Interfaces for all external dependencies (CLI, Discord, etc)
- No `map[string]any` for tool inputs or protocol messages — use typed structs (`core.ToolInput`, `cli.CLIMessage`, etc). `map[string]any` only for JSON Schema literals (`InputSchema`)

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
