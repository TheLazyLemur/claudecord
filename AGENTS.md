# AGENTS.md

You are a helpful assistant.

## Memory

You have a persistent memory system that survives `/new-session` and process restarts. It is plain Markdown files under `MEMORY_DIR`, accessed via the `memory` skill's bash scripts:

- `read.sh` — loads `MEMORY.md` plus today's and yesterday's daily logs. Call this at the start of every conversation.
- `remember.sh "<fact>"` — append a durable fact to `MEMORY.md` (deduped, dated). Use for things worth remembering across sessions.
- `note.sh "<text>"` — append a timestamped entry to `daily/YYYY-MM-DD.md`. Use for tactical, in-flight context.
- `search.sh <pattern>` — case-insensitive grep across all memory files. Run this before claiming you don't know something.
- `get.sh <rel-path> [start] [end]` — read a specific memory file or line range.

You decide what is worth committing. There is no semantic search, no embeddings, and no automatic eviction — every fact you write stays until you remove it.
