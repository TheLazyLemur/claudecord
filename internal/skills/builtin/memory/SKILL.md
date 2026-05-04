---
name: memory
description: Persistent long-term memory across sessions. Plain Markdown files at $MEMORY_DIR. Use this skill at the start of any conversation to load context, when the user asks you to remember something, when you learn a durable fact about the user, or when you need to recall something said in a past conversation.
---

# Memory

The bot's session history resets on `/new-session` and on container restarts. This skill is the only thing that persists across those boundaries. If something matters beyond the current turn, it has to be written to a file here.

## How memory is stored

All files live under `$MEMORY_DIR` (an absolute path set in the bot's environment).

- `MEMORY.md` — durable facts, preferences, decisions, behavioural rules. Curated. This is the long-term layer.
- `daily/YYYY-MM-DD.md` — running notes for the day. Observations, intermediate context, things that may or may not deserve promotion to `MEMORY.md` later.

Plain Markdown. No DB, no embeddings, no hidden state. The only thing you remember is what is on disk.

## When to use

- **At the start of any conversation:** run `list.sh` to see every file under `$MEMORY_DIR`, then run `read.sh` to load `MEMORY.md` plus today's and yesterday's daily logs into context. Anything `list.sh` shows that `read.sh` didn't print is a file you can pull in on demand with `get.sh`. Do this before answering anything that might depend on prior knowledge of the user.
- **User says "remember X" / "don't forget Y":** call `remember.sh "X"` to write a durable fact to `MEMORY.md`.
- **You learn something durable on your own** (a stable preference, a project name, a recurring contact, a decision the user made): call `remember.sh` proactively. Be selective — only commit what will still matter in a month.
- **You learn something tactical** (current task state, what was discussed today, intermediate observations): call `note.sh` to append to today's daily log.
- **User refers to something you don't remember** ("the project I told you about", "that email from last week"): call `search.sh <pattern>` before saying you don't know.
- **You were corrected on a mistake:** record the correction as a rule via `remember.sh` so you don't repeat it.

## Scripts

All scripts read `$MEMORY_DIR` from the environment.

### list.sh

```bash
bash scripts/list.sh
```

Prints every file under `$MEMORY_DIR` as `<relative-path>\t<size> bytes`, sorted. Run this at the top of a new conversation alongside `read.sh` so you know what files exist beyond the always-loaded ones (e.g. extra notes the user added through the dashboard). Use the paths it prints with `get.sh` when you need their contents.

### read.sh

```bash
bash scripts/read.sh
```

Prints `MEMORY.md`, today's daily log, and yesterday's daily log (when they exist). Always safe to call. Run this once at the top of a new conversation. It does **not** print other files in `$MEMORY_DIR` — use `list.sh` to discover those.

### remember.sh

```bash
bash scripts/remember.sh "I prefer TypeScript over JavaScript for new projects."
```

Appends a single bullet to `MEMORY.md`, with the date. Skips the write if an identical line already exists. Use for durable facts only.

### note.sh

```bash
bash scripts/note.sh "User is debugging a flaky CI job in repo X."
```

Appends a timestamped bullet to today's daily log (`daily/YYYY-MM-DD.md`). Use freely for running context.

### search.sh

```bash
bash scripts/search.sh "typescript"
```

Case-insensitive grep across all memory files (`MEMORY.md` plus every file in `daily/`). Prints matching lines prefixed with their file. Use this before claiming you don't remember something.

### get.sh

```bash
bash scripts/get.sh MEMORY.md
bash scripts/get.sh daily/2026-04-30.md
bash scripts/get.sh daily/2026-04-30.md 1 50
```

Prints a specific file (or a line range from it). Use to read older daily logs, or any file surfaced by `list.sh` that `read.sh` doesn't auto-load.

## Conventions

- One fact per line in `MEMORY.md`. Bullet form, prefixed with the date. Keep it tight — this file is loaded at the start of every conversation.
- Daily logs are noisier and append-only; don't worry about brevity there.
- Never edit files by hand from this skill. The scripts are the only writers — they preserve append semantics and dedupe.
