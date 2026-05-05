# Claudecord

Discord/WhatsApp bot that talks to any Anthropic-shaped `/v1/messages` endpoint via the Anthropic Go SDK. Tested against Anthropic, Moonshot/Kimi, Minimax, and Ollama; many other providers expose the same shape (often as an opt-in compatibility mode). It is NOT tied to Anthropic-the-company — Anthropic is just the wire format.

## What It Is

- Single-user, single-server Discord bot (user-restricted via `ALLOWED_USERS` env var)
- `@claude` as first word triggers bot
- `/new-session` starts fresh session, subsequent `@claude` continues it
- In-process session management; no disk persistence
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
- `CLAUDE_CWD` - Default working directory the agent runs in (optional, defaults to first allowed dir)
- `CLAUDECORD_API_KEY` - API key for the upstream endpoint (required)
- `CLAUDECORD_BASE_URL` - Optional base URL to point at a non-Anthropic endpoint (e.g. Moonshot/Kimi, Minimax, Ollama, or any other provider exposing an Anthropic-shaped `/v1/messages` API)
- `MODEL` - Model id. Defaults to `Kimi-for-Coding` when `CLAUDECORD_BASE_URL` is set, otherwise to a recent Sonnet. Override to use any other model id supported by the endpoint.
- `WHATSAPP_MEDIA_DIR` - Directory inbound WhatsApp attachments are decrypted into. Defaults to `<first ALLOWED_DIR>/wa-media` when `WHATSAPP_ALLOWED_SENDERS` is set; must live under one of `ALLOWED_DIRS` if overridden.
- `MEMORY_DIR` - Where the `memory` skill stores `MEMORY.md` and `daily/YYYY-MM-DD.md` logs. Defaults to `<first ALLOWED_DIR>/claudecord-memory`. Must live under `ALLOWED_DIRS`. Exported into the bot process env at startup so the skill's bash scripts inherit it.
- `THINKING_BUDGET_TOKENS` - Optional. When set to a positive integer, every API call enables extended thinking with that token budget (`thinking={type:enabled,budget_tokens:N}`). Anthropic requires N >= 1024. Confirmed working against Kimi's `api.kimi.com/coding/v1/messages` Anthropic-compatible endpoint with `kimi-for-coding`. Unset/empty disables thinking.

## Memory skill

OpenClaw-style persistent memory layered on plain Markdown files under `MEMORY_DIR`. Survives `/new-session` and container restarts.

- `MEMORY.md` — durable curated facts; one bullet per line, dated.
- `daily/YYYY-MM-DD.md` — running daily logs; appended freely.
- Scripts: `list.sh` (every file under MEMORY_DIR with sizes), `read.sh` (loads MEMORY.md + today + yesterday), `remember.sh` (append durable fact, dedupes), `note.sh` (append timestamped daily note), `search.sh <pattern>` (case-insensitive grep across all files), `get.sh <rel-path> [start] [end]` (read a file or line range).
- The model decides what to commit; the SKILL.md tells it to call `list.sh` + `read.sh` at the start of each conversation, `get.sh` for files surfaced by `list.sh` that `read.sh` doesn't auto-load (e.g. dashboard-added notes), `remember.sh` for durable facts, `note.sh` for tactical context, and `search.sh` before claiming it doesn't know.
- No semantic search, no embeddings, no eviction — matches OpenClaw's default behaviour. Add a plugin if you want recall guarantees.

## AGENTS.md context

- If `AGENTS.md` exists in the session working directory, its contents are appended to the system prompt wrapped in `<agents_md>...</agents_md>`.
- The file is re-read on every API call, so edits land on the next turn with no session restart.

## Steering (mid-loop message queueing)

- A second `@claude` message that arrives while the previous turn's tool loop is still running is queued, not dropped or rejected.
- The running loop drains the queue at two boundaries: (a) just after `tool_result` blocks are appended, riding along as extra text blocks in the same user message; and (b) at natural end-of-turn, where a non-empty queue causes the loop to continue with a fresh user message instead of returning.
- Anthropic requires every `tool_use` to be paired with a `tool_result` in the immediately-next user message, so steering text is appended to that same user message rather than as a separate turn.
- Queued messages are wrapped in `<user_steering>...</user_steering>` so the model recognizes them as course-corrections.
- The in-flight HTTP request to the model is **not** aborted; tokens already produced are kept, and the loop reads the queue right before the next API call.
- Concurrency: `Backend.mu` guards the `running` flag and `mailbox`. `Bot.mu` is an `RWMutex` — `HandleMessage` holds the read lock so multiple messages can reach the backend concurrently; `NewSession` takes the write lock so it still waits for all in-flight messages.
- Only the first caller's responder produces the combined reply. Steered callers' `Converse` returns `("", nil)` so they don't double-post.

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
- Unit tests only - mock the API client at the SDK boundary
- Interfaces for all external dependencies (Discord, etc)
- No `map[string]any` for tool inputs or protocol messages — use typed structs (`core.ToolInput`, etc). `map[string]any` only for JSON Schema literals (`InputSchema`)

## Discord Rules

- Register handlers BEFORE `dg.Open()` for message events to work
- Need `IntentsGuildMessages | IntentMessageContent` for message content
- Bot invite URL must include `bot` scope (not just app auth) - use: `?scope=bot%20applications.commands`
- Role mentions (`<@&ID>`) differ from user mentions (`<@ID>`) - bot only responds to user mentions
