# Memory Skill Roadmap

The current `memory` skill (`internal/skills/builtin/memory/`) is a deliberate
subset of OpenClaw's stock memory: `MEMORY.md` + `daily/YYYY-MM-DD.md`,
grep-based search, model-driven writes via bash scripts. This doc tracks the
gaps vs OpenClaw and the candidate additions to close them.

See also: `CLAUDE.md` "Memory skill" section, OpenClaw memory docs at
<https://docs.openclaw.ai/concepts/memory>.

## Current state

- `MEMORY.md` — durable bullets, dated.
- `daily/YYYY-MM-DD.md` — append-only daily logs.
- Scripts: `read.sh`, `remember.sh`, `note.sh`, `search.sh`, `get.sh`.
- `search.sh` = `grep -i`. No embeddings, no semantic recall.
- No consolidation pass. No compaction-flush hook. No `DREAMS.md`.
- Stored under `MEMORY_DIR` (Fly volume, persisted across deploys).
- Dashboard memory editor (PR #18) for read/write/delete from the UI.

## Gap analysis vs OpenClaw

| OpenClaw feature | Switchboard state | Gap impact |
|---|---|---|
| Memory-flush before context compaction | None | Low today (no compaction; `/new-session` is a hard reset). Becomes high-leverage if rolling sessions are added. |
| Dreaming / consolidation cron | None | Medium. Manual curation works while `MEMORY.md` is small; degrades as it grows. |
| `DREAMS.md` audit log | None | Low standalone; cheap to add once consolidation exists. |
| Semantic `memory_search` | grep | Low at current scale; revisit past ~500 entries. |
| Named tools (`memory_search`, `memory_get`) | bash scripts | Cosmetic. Skip. |

## Proposed additions

Ranked by leverage-per-effort.

### 1. Pre-`/new-session` flush (HIGH leverage, LOW effort)

Before the session manager tears down the backend on `/new-session`, run
one final agent turn with a system message instructing the model to commit
durable facts to `MEMORY.md` via `remember.sh` and tactical context to
today's daily note via `note.sh`.

- **Hook point:** `dashboard.handleNewSession` and the equivalent
  Discord/WA new-session paths in `handler/`. Fire before
  `s.sessionMgr.NewSession(workDir)`.
- **First-session start:** skip the flush — nothing to commit on first
  boot. Only run on session *replacement*.
- **Prompt:** short system message — "this session is ending. Use
  `remember.sh` for durable facts and `note.sh` for tactical context worth
  keeping. Be brief; only commit things actually established this
  session."
- **Failure mode:** hard timeout (~30s); on error or timeout, log and
  proceed with the new session. Never block the reset.
- **Opt-out:** `MEMORY_FLUSH_DISABLED=1` env flag.
- **Estimated work:** ~50 LoC Go + a prompt template.

### 2. Consolidation pass (MEDIUM leverage, MEDIUM effort)

Background job that reads new `daily/*.md` since the last successful run
and asks the model to extract candidate durable facts. Promoted facts
get appended to `MEMORY.md`; the run is summarised in `DREAMS.md`.

- **Trigger:** dashboard "Consolidate now" button first; cron / Go
  ticker layered on later. Both call the same entry point.
- **Hook point:** new `internal/memory/consolidate.go`. Uses the existing
  `core.SessionManager` (or a one-shot equivalent) to fire a single agent
  invocation against the memory dir.
- **State:** `MEMORY_DIR/.consolidate-state.json`, a single struct of
  `{"last_processed_date":"YYYY-MM-DD","last_run_at":"<RFC3339>"}`. Plain
  file — no sqlite for one row. If we grow other persistent bot state
  later, migrate to sqlite then. Read at start, written atomically on
  success.
- **Window algorithm:**
  - `start = last_processed_date + 1 day` (or oldest daily log if state
    is empty).
  - `end = today - 1 day` (today is in flight, always excluded).
  - Process every `daily/<date>.md` in `[start, end]` that exists.
  - On success, advance `last_processed_date = end`.
  - If `start > end`, no-op.
  - Edits to already-processed daily logs are **not** re-consolidated.
    Trade-off: avoids duplicate facts and re-billing for known content.
- **Prompt:** loads the in-window daily notes + current `MEMORY.md`,
  asks the model to propose new durable bullets not already in
  `MEMORY.md`. Output as JSON array.
- **Promotion abstraction:**
  ```go
  type FactPromoter interface {
      Promote(ctx context.Context, fact string) error
  }

  type AutoPromoter struct { memoryDir string } // shells out to remember.sh
  ```
  Consolidation takes a `FactPromoter` parameter. Day one: `AutoPromoter`
  everywhere — facts go straight into `MEMORY.md`. Later, a
  `ReviewPromoter` could write proposals to `DREAMS.md` and require a
  dashboard click; the consolidation loop wouldn't change.
- **`DREAMS.md` ships in this PR:** dated summary per run — date range,
  daily files read, facts promoted (and, when review flow ships, facts
  rejected with reason).
- **Daily logs are read-only.** No deletion, no edit, ever.
- **Failure mode:** never advance state on partial failure. Bail with
  the run logged to `DREAMS.md` so it's auditable.
- **Estimated work:** ~250 LoC Go + state file I/O + prompt template +
  dashboard WS message + button, mirroring the existing memory editor
  pattern.

### 3. `DREAMS.md` (LOW leverage standalone, ships with #2)

Human-readable audit of consolidation runs: timestamp, candidates
considered, what was promoted, why. Skip until #2 ships.

### 4. Transcript persistence + transcript-fed dreaming (HIGH leverage, HIGH effort)

Daily logs only capture what the model thought to write down via
`note.sh`. Persisting full session transcripts and feeding them into
consolidation catches misses where a fact was clearly established in the
conversation but never noted at the time. Mirrors OpenClaw's optional
transcript ingestion (`sessions/*.jsonl`).

- **Persistence:** every user/assistant turn and tool call written to
  `MEMORY_DIR/transcripts/<session-id>.jsonl` as the conversation
  progresses. Append-only, one file per session, redacted at write time
  (strip secrets / API keys / etc.).
- **Consolidation upgrade:** after daily-log consolidation has run,
  optionally feed the in-window transcripts into a second pass.
  Dedupe candidates against what daily-log consolidation already
  promoted (Jaccard ≥ 0.9, OpenClaw-style).
- **Cost knobs:** transcript-fed dreaming is opt-in
  (`MEMORY_DREAM_TRANSCRIPTS=1`), runs less frequently than the daily
  pass (e.g. weekly), and can be capped to last N days of transcripts
  to bound token spend.
- **Trade-off:** much higher token cost per run (10–100x), but catches
  facts the model failed to note. Ship only after #2 has run for long
  enough to know whether note discipline is the bottleneck.
- **Storage:** rotate / cap transcripts at e.g. 90 days; older ones
  archived or deleted on a schedule.

### 5. Semantic search (LOW leverage at current scale, HIGH effort)

Out of scope until `MEMORY.md` exceeds ~500 entries or you start missing
facts you know are stored. When that happens, candidate stacks:
LanceDB (file-based, no server, fits the Fly single-volume model) or
SQLite-vec.

## Explicitly out of scope

- Cross-user / cross-platform memory keys. Single-user single-server bot,
  no value.
- Auto-eviction by TTL. Memory is cheap; durable facts are valuable.
  Consolidation can dedupe but not delete.
- Replacing bash scripts with named tools. Cosmetic only.

## Suggested order

1. ✅ Pre-`/new-session` flush — shipped (#20).
2. Daily-log consolidation: dashboard button first, ticker second. Same
   PR or two, author's call. `DREAMS.md` ships inside this PR.
3. Transcript persistence + transcript-fed dreaming — only after #2
   has run long enough to know whether note discipline is the
   bottleneck.
4. Semantic search only when grep starts to bite.
