# Built-in Skills Roadmap

Candidate built-in skills to add, ranked by practical fit to this bot's
existing surface (WhatsApp + Discord, inbound media flow, `Read`/`Bash`/
`Fetch`/`WebSearch` tools, single-user single-server deployment).

Each skill follows the same shape as `pdf-reader`:
`internal/skills/builtin/<name>/SKILL.md` plus an optional
`scripts/*.sh`. Debian package additions go in `Dockerfile`.

## Tier 1 — round out the WhatsApp media flow

These three close the document MIMEs that already arrive as `<attachment>`
tags but currently have no matching skill, so the model finds nothing in
`<available_skills>`.

### docx-reader
- **What:** extract plain text from a `.docx` attachment.
- **Trigger:** `<attachment mime="application/vnd.openxmlformats-officedocument.wordprocessingml.document" .../>`.
- **Implementation:** `pandoc -f docx -t plain <path>`.
- **Debian pkg:** `pandoc` (already installed in runtime image).
- **Cost:** zero marginal — already in image. High value — Word docs are the most common non-PDF doc family.

### xlsx-reader
- **What:** dump an Excel workbook as CSV (one chunk per sheet).
- **Trigger:** `<attachment mime="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" .../>`.
- **Implementation:** `xlsx2csv -a <path>` (prints all sheets), or `ssconvert <path> /dev/stdout` if pandoc/gnumeric is already in.
- **Debian pkg:** `xlsx2csv` (Python, ~5 MB) or pull in via `pip install xlsx2csv`. Avoid `gnumeric` — it's huge.
- **Cost:** small. High value — invoices, statements, lists.

### csv-reader
- **What:** pretty-print a `.csv` so the model sees aligned columns.
- **Trigger:** `<attachment mime="text/csv" .../>`.
- **Implementation:** `column -ts, < <path> | head -200` (cap rows so we don't blow the context).
- **Debian pkg:** `bsdmainutils` or `util-linux` (if `column` not present).
- **Cost:** trivial. Skip if you don't care — `Read` already returns raw CSV which is usable but ugly.

## Tier 2 — utilities that play to existing strengths

### archive-peek
- **What:** list contents of `.zip`/`.tar.gz`/`.tar` attachments and optionally extract one named file to a temp path so the model can `Read` it.
- **Trigger:** `<attachment mime="application/zip" .../>` etc.
- **Implementation:** `unzip -l <path>` / `tar -tzf <path>`; `unzip -p <path> <inner>` to extract one entry.
- **Debian pkg:** `unzip` (`tar` already present).
- **Cost:** small. Useful for code dumps, log bundles, screenshot zips.

### exif-info
- **What:** dump EXIF metadata from an image attachment — date, GPS, camera, dimensions.
- **Trigger:** model decides; useful when the user asks "where was this taken" or "when was this".
- **Implementation:** `exiftool -j <path>` (JSON output is easy for the model to parse).
- **Debian pkg:** `libimage-exiftool-perl`.
- **Cost:** ~20 MB. Medium value — fills a gap vision can't reliably answer.

### link-summarize
- **What:** fetch a URL, convert HTML → clean plain text, hand it back to the model.
- **Trigger:** user shares a link and asks about it.
- **Implementation:** `curl -sL <url> | pandoc -f html -t plain` (pandoc already installed).
- **Debian pkg:** `pandoc` (already in image).
- **Cost:** zero marginal. High value — `Fetch` returns raw HTML today which is hard for the model to skim.

## Tier 3 — bigger lifts

These are higher-value but each adds real complexity. Each is opt-in.

### memory / notes
- **What:** addresses the "no long-term memory" gap. Per-chat-JID notes file the bot reads/writes; the model decides what's worth remembering.
- **Trigger:** model invokes when the user says "remember X", "what did I tell you about Y", or proactively when committing useful facts.
- **Implementation:**
  - `scripts/append.sh <chat-jid> <text>` → appends to `~/.claudecord/memory/<chat-jid>.md` with a timestamp.
  - `scripts/read.sh <chat-jid>` → cat the file.
  - `scripts/search.sh <chat-jid> <pattern>` → `grep -i <pattern>` over the file.
- **Storage:** `~/.claudecord/memory/`. Should live on the Fly volume if persistence matters across deploys.
- **System prompt addendum:** "If the user asks you to remember something, call the `memory` skill with the chat JID. At the start of a conversation, consider reading the chat's memory file for context."
- **Debian pkg:** none.
- **Cost:** small code, but real product decision — see "Open questions" below.

### voice-transcribe
- **What:** transcribe inbound WhatsApp voice notes. Currently dropped (plan says voice is out of scope).
- **Pre-req:** extend `extractInbound` in `internal/handler/whatsapp_media.go` to also handle `AudioMessage` (currently returns `nil`). Treat as another attachment MIME (e.g. `audio/ogg`).
- **Implementation choice:**
  - **(a) Local:** bundle `whisper.cpp` + a small (`ggml-base.en` ~140 MB) model. Pros: no API call, no key. Cons: image bloat, RAM, slow on Fly nano.
  - **(b) API:** POST the file to OpenAI Whisper or Groq's hosted Whisper. Pros: fast, small. Cons: another API key, third-party data exposure.
- **Trigger:** `<attachment mime="audio/ogg" ...>` in the prompt, skill matches on `audio/`.
- **Recommendation:** start with (b) behind an env var (`WHISPER_API_KEY`); only build (a) if you find you're transcribing constantly enough that the API cost matters.

### screenshot-url
- **What:** render a webpage to a PNG so the model can describe a layout, debug a page, or grab a chart from a dashboard.
- **Implementation:** `chromium --headless --disable-gpu --screenshot=/tmp/out.png --window-size=1280,1600 <url>` then return the path so vision can `Read` it.
- **Debian pkg:** `chromium`.
- **Cost:** ~250 MB image bloat, plus chromium needs to start fresh per call (~1–2s overhead). High utility but expensive — only justified if you actually use it.

## Tier 4 — explicitly skip

Documented so we don't keep re-evaluating them.

- **Calculator / unit converter** — the model does these directly with no tool.
- **Reverse image search** — niche, requires third-party API.
- **QR decode (`zbarimg`)** — fun but I can't think of a real recurring need.
- **"Search news" / "search web"** — already covered by the `WebSearch` (Brave) tool.
- **Timezone / world clock** — model can compute this from the system prompt's date.

## Suggested order

1. **docx-reader + link-summarize** in one PR (pandoc already in image).
2. **xlsx-reader + csv-reader + archive-peek** in one PR (Office + zip closes most attachments users actually send).
3. **memory** as its own PR — needs design discussion before code (see open questions).
4. **exif-info** standalone, small.
5. **voice-transcribe** only after the above land and you've decided (a) vs (b).
6. **screenshot-url** last, if at all.

## Open questions before building memory

- **Scope key:** per chat JID, per sender JID, or global? JID-keyed is private-by-default but loses cross-channel context.
- **Read trigger:** auto-prepend memory contents to every prompt (always-on context, costs tokens), or require the model to explicitly read it (cheaper, more variance)?
- **Write policy:** model-decided ("commit this if useful") or user-explicit ("remember X")? Mix is fine but worth deciding the default.
- **Eviction:** none (file grows forever), TTL, or summarize-and-truncate when over N lines?
- **Where stored:** `~/.claudecord/memory/` on the container ephemeral disk (lost on redeploy) vs. on the Fly volume (persisted, has to be inside `ALLOWED_DIRS` for `Read` access).
