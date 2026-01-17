# Claudecord

Discord bot that integrates Claude Code CLI via the stdio protocol.

## Requirements

- Go 1.21+
- Claude Code CLI installed and authenticated
- Discord bot token

## Setup

1. Create a Discord application at https://discord.com/developers/applications
2. Create a bot and copy the token
3. Enable MESSAGE CONTENT INTENT in the bot settings
4. Invite bot to your server with: `https://discord.com/oauth2/authorize?client_id=YOUR_APP_ID&scope=bot%20applications.commands&permissions=326417590272`

## Configuration

Set environment variables in `.env`:

```
DISCORD_TOKEN=your_bot_token
ALLOWED_DIRS=/path/to/dir1,/path/to/dir2
ALLOWED_USERS=discord_user_id1,discord_user_id2
```

- `ALLOWED_DIRS` - directories Claude can access (applied recursively)
- `ALLOWED_USERS` - Discord user IDs allowed to use the bot

Uses direnv (`.envrc`) to auto-load `.env`.

## Usage

Build and run:

```
go build -o claudecord ./cmd/claudecord
./claudecord
```

In Discord:
- `@claude <message>` - send message to Claude (continues existing session)
- `/new-session` - start a fresh Claude session
- `/new-session dir:/path/to/project` - start session in specific directory

## How It Works

The bot spawns a Claude Code CLI subprocess and communicates via stdin/stdout using the stdio protocol. Tool permissions are auto-approved based on the allowed directories configuration.

Long responses are automatically posted in threads.

## License

MIT
