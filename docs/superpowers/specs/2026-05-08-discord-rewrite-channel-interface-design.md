# Discord rewrite + shared channel interface

Status: approved
Date: 2026-05-08

## Goal

Rip out the current Discord adapter, introduce a shared `ChannelPlugin` interface, refactor WhatsApp and dashboard onto it, then re-implement Discord against the same interface with a cleaner threading model: an `@claude` in a plain channel opens a new Discord thread; once inside a bot-owned thread, subsequent `@claude` messages stay there. The thread is the session scope.

## Non-goals

- Cross-channel `message`/broadcast tool. Not built.
- Concurrent sessions. One global active session, same as today.
- Per-session working directory override. Dropped along with `/new-session`.
- Persistent session storage. The memory skill is the only continuity across resets.

## Architecture

```
┌────────────────────────┐  ┌────────────────────────┐  ┌────────────────────────┐
│ Discord ChannelPlugin  │  │ WhatsApp ChannelPlugin │  │ Dashboard ChannelPlugin│
└──────────┬─────────────┘  └──────────┬─────────────┘  └──────────┬─────────────┘
           │ Inbound{SessionKey, Reply}│                           │
           └─────────────┬─────────────┴───────────────────────────┘
                         ▼
                   ┌─────────────┐
                   │   Bot       │── single active session ──▶  Backend
                   └─────────────┘
                         ▲
                         │ Outbound (PostResponse / SendUpdate / AddReaction)
                         └────────────────────────────────────────
```

- Each platform implements one `ChannelPlugin`.
- `Bot` is plugin-agnostic; it only reads `Inbound.SessionKey` to decide reset-or-continue.
- `cmd/claudecord/main.go` registers whichever plugins have configured tokens.

## Channel interface

```go
// internal/core/channel.go

type SessionKey string

type Inbound struct {
    SessionKey  SessionKey
    Text        string
    Attachments []Attachment
    Reply       Outbound
}

type Outbound interface {
    SendTyping() error
    PostResponse(content string) error
    SendUpdate(content string) error
    AddReaction(emoji string) error // no-op when Capabilities.Reactions == false
}
// Implementation note: Implemented as `core.Outbound` directly. The earlier
// `core.Responder` interface was retired during the refactor; all plugins
// implement `Outbound`.

type Capabilities struct {
    Reactions bool
    Typing    bool
}

type ChannelPlugin interface {
    ID() string
    Capabilities() Capabilities
    Start(ctx context.Context, deliver func(Inbound)) error
    Stop() error
}
```

`Outbound` is bound at receive time and travels with `Inbound` (matches current `Responder` flow). `SessionKey` is opaque to `Bot`; the plugin chooses how to construct it.

## Discord plugin behaviour

| Inbound                                                  | Action                                                | SessionKey                       |
|----------------------------------------------------------|-------------------------------------------------------|----------------------------------|
| `@claude` in a plain channel                             | Plugin opens new thread, posts there                  | `discord:thread:<newThreadID>`   |
| `@claude` inside an existing thread the bot owns         | Use that thread                                       | `discord:thread:<threadID>`      |
| `@claude` inside a thread the bot does NOT own           | Treat as plain channel: open new sibling under parent | `discord:thread:<newThreadID>`   |
| Message without `@claude`                                | Ignored                                               | n/a                              |
| DM                                                       | Use DM channel id                                     | `discord:dm:<userID>`            |

- `@claude` is required everywhere, including inside threads.
- `send_update` posts in the same thread as the conversation. No "Updates" sub-thread.
- `react_emoji` registered only when active plugin reports `Reactions: true`.

## WhatsApp plugin

- `SessionKey` = `whatsapp:<chatJID>`.
- Existing burst batching, media handling, `WHATSAPP_MEDIA_DIR` plumbing all preserved — wrapped in the new plugin shape.

## Dashboard plugin

- `SessionKey` = `dashboard:<sessionUUID>`.
- Existing SSE handler preserved — wrapped in the new plugin shape.

## Session lifecycle

```
on inbound:
  if inbound.SessionKey != activeKey:
      flush memory on current backend
      close current backend
      create new backend
      activeKey = inbound.SessionKey
  hand inbound to active backend
```

- One global active session. Switching surfaces (e.g. WhatsApp → Discord thread → another Discord thread) flushes and replaces.
- No `/new-session` command. New Discord thread is the only Discord-side reset; for WhatsApp/dashboard the session rolls until a different `SessionKey` arrives.
- Memory skill carries durable facts across resets.

## What gets deleted

- `internal/handler/discord.go` — entire file. Token/intents/`dg.New`+`dg.Open` extracted to a thin `connect()` helper inside `internal/channels/discord/`.
- `internal/core/responder.go` `DiscordResponder`.
- `core.DiscordTools()` registration site, `react_emoji` and `send_update` slash routing in `internal/core/tools.go` — re-registered conditionally based on plugin capabilities.
- `/new-session` slash command + `case "new-session"` handler.
- `CreateThread`, `StartThread`, chunk-into-thread logic in `DiscordClientWrapper` — moved inside the new Discord plugin.
- `internal/core/bot.go` `HandleMessage(responder Responder, ...)` signature — replaced by `HandleInbound(Inbound)`.
- Tests for the deleted code — rewritten against the plugin contract.

## What is kept

- `DISCORD_TOKEN`, `ALLOWED_USERS` env parsing.
- `discordgo` dependency, intent flags (`IntentsGuildMessages | IntentMessageContent`), session lifecycle plumbing.
- `core.ChunkMessage`, `core.MaxDiscordMessageLen`.

## New layout

```
internal/
  core/
    channel.go            # ChannelPlugin, Inbound, Outbound, Capabilities, SessionKey
    bot.go                # HandleInbound(Inbound) error
  channels/
    discord/
      plugin.go           # implements ChannelPlugin
      outbound.go         # implements Outbound
      threads.go          # thread bookkeeping
      connect.go          # discordgo session lifecycle
    whatsapp/
      plugin.go           # wraps existing handler logic
    dashboard/
      plugin.go           # wraps existing SSE handler logic
cmd/claudecord/
  main.go                 # registers plugins per configured tokens
```

## Testing

- **Per-plugin unit tests**: table-driven `(platformEvent) → (SessionKey, action)` with mocked SDK boundaries (`testify/mock`). Discord covers all four rows of the table above; WhatsApp keeps existing burst/media coverage; dashboard keeps SSE coverage. All retargeted at the plugin's `Start` callback.
- **Core unit tests**: `HandleInbound` flows — same key continues, different key flushes/closes/recreates, memory flush errors swallowed not propagated. Capability gating: `react_emoji` tool only registered when active plugin reports `Reactions: true`.
- **Mocks**: one mock `ChannelPlugin` for `bot_test.go`. No real Discord/WhatsApp/HTTP in tests.
- **Manual smoke test**: @claude in a Discord channel (thread spawned), reply in thread (same session), @claude in a different channel (new thread, previous session reset), then a WhatsApp message (Discord session reset).

## Open considerations

- Discord thread name. Auto-generated from first message? Fixed string? Default to first 50 chars of inbound text; revisit if it's annoying.
- DM behaviour and ALLOWED_USERS. DMs use `discord:dm:<userID>`; auth check unchanged.
- WhatsApp/dashboard reset path. There is no manual reset from the user side after this change. Acceptable because (a) memory persists facts, (b) process restart still resets, (c) cross-surface activity resets implicitly.
