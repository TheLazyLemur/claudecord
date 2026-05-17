# Switchboard

Bot that bridges multiple messaging channels (Discord, WhatsApp, web dashboard) to a swappable AI agent via any Anthropic-shaped `/v1/messages` endpoint. Tested against Anthropic, Moonshot/Kimi, Minimax, and Ollama. Not tied to Anthropic-the-company — Anthropic is just the wire format.

## What It Is

- Single-user bot across channels (Discord, WhatsApp, web dashboard)
- User-restricted via `ALLOWED_USERS` (Discord) or `WHATSAPP_ALLOWED_SENDERS`
- Mention the bot in Discord or message via WhatsApp/dashboard to trigger a turn
- `/new-session` starts a fresh session; subsequent messages continue it
- In-process session management; no disk persistence
- Long responses posted in threads
- Tools run autonomously; path containment against `ALLOWED_DIRS` is the only safety net

## Requirements

- Go 1.24+
- An API key for an Anthropic-shaped `/v1/messages` endpoint (Anthropic, Kimi, Minimax, Ollama, etc.)
- A Discord bot token and/or WhatsApp configuration (`WHATSAPP_ALLOWED_SENDERS`)

## Configuration

`ALLOWED_DIRS` and `SWITCHBOARD_API_KEY` are always required. At least one platform (`DISCORD_TOKEN` or `WHATSAPP_ALLOWED_SENDERS`) must also be set. `ALLOWED_USERS` is required only when `DISCORD_TOKEN` is set.

| Variable | Required | Default | Notes |
|---|---|---|---|
| `DISCORD_TOKEN` | if no WhatsApp | — | Discord bot token |
| `WHATSAPP_ALLOWED_SENDERS` | if no Discord | — | Comma-separated phone numbers |
| `ALLOWED_DIRS` | yes | — | Comma-separated paths; tool access is confined to these (recursive) |
| `ALLOWED_USERS` | if Discord | — | Comma-separated Discord user IDs |
| `SWITCHBOARD_API_KEY` | yes | — | API key for the upstream endpoint |
| `SWITCHBOARD_BASE_URL` | no | Anthropic | Base URL for non-Anthropic endpoints |
| `MODEL` | no | `claude-sonnet-4-20250514` (Anthropic) or `Kimi-for-Coding` (custom base URL) | Model ID passed to the API |
| `AGENT_CWD` | no | first `ALLOWED_DIRS` entry | Default working directory for the agent |
| `WEBHOOK_PORT` | no | `5005` | Port for inbound webhooks / dashboard |
| `DASHBOARD_PASSWORD` | no | — | Password for web dashboard auth |
| `MEMORY_DIR` | no | `<first ALLOWED_DIR>/switchboard-memory` (falls back to `<first ALLOWED_DIR>/claudecord-memory` if that legacy directory exists) | Persistent memory files |
| `DISCORD_MEDIA_DIR` | no | `<first ALLOWED_DIR>/discord-media` | Where Discord attachments are saved |
| `WHATSAPP_MEDIA_DIR` | no | `<first ALLOWED_DIR>/wa-media` | Where WhatsApp attachments are decrypted |
| `WHATSAPP_DB_PATH` | no | `whatsapp.db` | WhatsApp session database path |
| `THINKING_BUDGET_TOKENS` | no | disabled | Enable extended thinking; must be ≥ 1024 |
| `WEB_SEARCH_API_KEY` | no | — | Brave Search API key for the `WebSearch` tool |
| `RESEND_API_KEY` | no | — | Resend API key for email skills |
| `AGENTS_DEFAULT_PATH` | no | `/etc/switchboard/AGENTS.md.default` | Bundled default AGENTS.md |

**Legacy fallbacks (deprecated, emit a warning):** `CLAUDECORD_API_KEY` → `SWITCHBOARD_API_KEY`, `CLAUDECORD_BASE_URL` → `SWITCHBOARD_BASE_URL`, `CLAUDE_CWD` → `AGENT_CWD`.

## Build & Run

```bash
go build -o switchboard ./cmd/switchboard
./switchboard
```

A `Dockerfile` is included for containerised deployments.

## Usage

**Discord:** mention the bot (`<@BOT_ID> your message`) to start or continue a session. Use `/new-session` to clear the session.

**WhatsApp:** send a message from an allowed sender number; the bot responds in the same chat.

**Dashboard:** available at the configured `WEBHOOK_PORT` when `DASHBOARD_PASSWORD` is set.

If `AGENTS.md` exists in `AGENT_CWD`, its contents are appended to the system prompt on every API call.

## How It Works

Switchboard connects each channel to an agent loop that calls an Anthropic-shaped `/v1/messages` HTTP API via the Anthropic Go SDK. Tools execute autonomously; file-system access is path-contained to `ALLOWED_DIRS`. Long model responses are split into Discord threads automatically.

Mid-loop steering: a second message that arrives while a tool loop is running is queued and injected at the next safe boundary rather than dropped.

## License

MIT
