# WhatsApp Media — Implementation Plan

Inbound media (images + arbitrary documents) on the WhatsApp channel, against
the API backend pointed at Kimi K2.6's Anthropic-compatible endpoint.

Outbound (bot → image), CLI mode removal, voice/video/sticker handling, and
file lifecycle/cleanup are explicitly out of scope.

## Locked design decisions

- **Mode:** API only. CLI mode left in tree, untouched.
- **Backend:** existing `internal/api/backend.go` + `anthropic-sdk-go` against
  Kimi's Anthropic-compatible endpoint via `BaseURL`.
- **Media types:** images (jpg/png/webp/gif) + documents (any MIME the user
  authors a skill for: PDF, docx, xlsx, csv, …).
- **Trigger:** any allowed sender's message with text *or* attachment fires the
  bot. No caption requirement.
- **Burst handling:** reuse `core.DebouncedBuffer` keyed on chat JID. Default
  delay 3s. (See open question below.)
- **Storage:** new env var `WHATSAPP_MEDIA_DIR`, validated at startup to live
  inside `ALLOWED_DIRS`.
- **Filename:** `<RFC3339>_<sanitized-original-or-rand>.<ext>`. Random suffix
  appended unconditionally to avoid same-second collisions.
- **Lifecycle:** none. Manual cleanup.
- **Prompt shape:** interleaved `<message>` blocks containing `<text>` and
  `<attachment>` children, preserving arrival order.
- **Vision path:** path-only on user turn → Kimi calls extended `Read` →
  tool returns multimodal `tool_result` with an `image` content block for
  image MIMEs, text otherwise.
- **Skills:** user-authored, one per non-image MIME family.

## Phase 0 — HARD GATE: Verify Kimi accepts image content in `tool_result`

The entire design hinges on Kimi K2.6's Anthropic-compat endpoint firing its
vision encoder on an `image` block delivered inside a `tool_result`, not just
on a user-turn `image` block. **If this assumption fails, Phases 4–5 are
wrong** and we revert to inline-on-user-turn for images (which the user
explicitly chose against, so the design has to be re-discussed first).

Action:

- Throwaway script (~30 lines, `cmd/scratch/kimi_vision_check/main.go` or
  ad-hoc): single-turn conversation, model declares a `Read` tool, we reply
  with a `tool_result` whose content is `[{type: "image", source: {...}}]`,
  ask "describe the image."
- Pass criteria: model produces a description that demonstrates it actually
  saw the image (not a generic "I see an image was shared").
- Also confirms the right model id string for this endpoint
  (`kimi-k2.6` vs `moonshotai/kimi-k2.6`).

**Do not start Phase 4 or Phase 5 until Phase 0 passes.** Phases 1–3 can
proceed in parallel since they don't depend on the assumption.

## Phase 1 — Config

Files: `internal/config/config.go`, `internal/config/config_test.go`.

1. Add `WhatsAppMediaDir string`. Env var `WHATSAPP_MEDIA_DIR`. Required iff
   `WhatsAppEnabled()`. At load:
   - Path containment check: must be inside one of `AllowedDirs`. Reuse the
     existing containment logic from `internal/permission/path_validator.go`
     rather than reinventing it.
   - `os.MkdirAll(path, 0700)` on startup.
2. Add `Model string`. Env var `MODEL`. **Rule:** if `BaseURL != ""`, `MODEL`
   is required (no magic default). If `BaseURL == ""`, default to the existing
   Sonnet id. Reasoning: less magic, clearer error.
3. Tests:
   - Media dir inside / outside allowed dirs.
   - Missing `WHATSAPP_MEDIA_DIR` when WA enabled.
   - `MODEL` required when `BaseURL` set; default otherwise.

## Phase 2 — Decryption + storage

New: `internal/handler/whatsapp_media.go` and `_test.go`.

4. Extend `WAClient` interface in `whatsapp.go`:
   - `Download(ctx context.Context, msg whatsmeow.DownloadableMessage) ([]byte, error)`
     wrapping `whatsmeow.Client.Download`.
5. Type `Attachment struct { MIME, OriginalName string; Bytes []byte }`.
6. `extractInbound(msg *events.Message, dl Downloader) (caption string, att *Attachment, err error)`:
   - `ImageMessage`: caption from `GetCaption()`, MIME from `GetMimetype()`,
     no original name.
   - `DocumentMessage`: caption from `GetCaption()`, MIME from `GetMimetype()`,
     original name from `GetFileName()`.
   - Other types (audio/video/sticker/location/contact): return nil — not in
     scope.
7. `saveAttachment(mediaDir string, att *Attachment, now time.Time) (path string, err error)`:
   - Filename = `<RFC3339-now>_<sanitized-original-or-rand-8>_<rand-4>.<ext>`.
     Trailing `<rand-4>` is unconditional, eliminating same-second collisions
     even when the original name is preserved.
   - Sanitize original name: strip path separators, leading dots, control
     chars, NFC-normalize, collapse whitespace, cap to 100 chars. Empty
     post-sanitization → fall back to `<rand-8>`.
   - Ext from original-name's suffix when valid; else
     `mime.ExtensionsByType(att.MIME)`; else `.bin`.
   - Write with `0600` permissions.
8. **Size cap (enforced before save):**
   - Images: 10 MiB hard limit.
   - Docs: 50 MiB hard limit.
   - Oversized → drop *just that attachment*, send a single
     "skipped (too large): <original-name-or-mime>" line via
     `WhatsAppMessenger.SendText`. Other attachments in the burst proceed.
9. Tests:
   - Filename construction with frozen `now` across MIME/name combos.
   - Sanitization: `..`, `/`, control chars, very long names, Unicode.
   - Size cap rejection (one over, one under in same handler call).
   - Ext fallback chain.

## Phase 3 — Burst batching

Files: `internal/core/buffer.go`, `internal/handler/whatsapp.go`, plus tests.

10. Extend `core.BufferedMessage`:
    ```go
    type AttachmentRef struct {
        Path, MIME, OriginalName string
    }
    type BufferedMessage struct {
        ChannelID, MessageID, Content, AuthorID string
        Attachments []AttachmentRef // platform-specific; nil on Discord
    }
    ```
    Discord paths leave it nil. Document on the type that the field is
    WhatsApp-only today.
11. In `WAHandler`, replace the per-message
    `go func() { bot.HandleMessage(...) }()` path with enqueue into a
    `*core.DebouncedBuffer` keyed on chat JID. Delay 3s. Buffer's flush
    callback constructs the prompt body (Phase 4) and invokes
    `bot.HandleMessage`. The bot's existing mutex serializes flushes if they
    overlap.
12. Permission-reply fast path (`HandleIncomingReply`) stays **before** buffer
    enqueue — unchanged in behavior.
13. Known limitation to document in a comment: a single message containing
    both a permission "yes/no" reply *and* an attachment will consume the
    reply and drop the attachment. Acceptable; vanishingly rare in practice.
14. Tests via `WAClient` mock:
    - Single message → batched after delay → handler called once.
    - 3-message burst → single batched dispatch.
    - Permission reply during waiter bypasses buffer entirely.
    - Mixed text-only + attachment-only messages preserve order in the batch.

## Phase 4 — Prompt-shape rendering

Files: `internal/core/whatsapp_prompt.go` (new) + tests.
**Depends on Phase 0 passing.**

15. `func RenderWhatsAppBatch(msgs []BufferedMessage) string`:
    ```
    <message>
      <text>caption here</text>
      <attachment path="/abs/path" mime="image/png" original_name="photo.png" />
    </message>
    <message>
      <attachment path="/abs/path" mime="application/pdf" original_name="report.pdf" />
    </message>
    <message>
      <text>just text</text>
    </message>
    ```
    - Empty captions: omit the `<text>` element, keep the `<message>` wrapper.
    - Multiple attachments per message: list them inside one `<message>`.
    - XML-escape text contents and attribute values.
16. Buffer flush callback in WAHandler: render → construct
    `WhatsAppResponder` → `bot.HandleMessage(responder, rendered)`.
17. **System prompt addition** (in `core.BuildSystemPrompt` or wherever the
    base prompts live): append a short instruction like
    > When the user message contains `<attachment>` tags, consult
    > `<available_skills>` for one whose description matches the `mime`
    > attribute and follow its instructions. For image MIMEs, use `Read` on
    > the path to view the image.

    Don't rely on skill descriptions alone to carry pattern-matching reliability.
18. Tests:
    - Empty captions → no `<text>` child.
    - Mixed text + attachment in one message.
    - Multi-message order preservation.
    - XML escaping of `<`, `>`, `&`, `"` in captions and filenames.

## Phase 5 — Extend `Read` (sentinel-string variant)

Files: `internal/tools/executor.go`, `internal/api/backend.go`, plus tests.
**Depends on Phase 0 passing.**

This is the cheap localized variant — *not* a typed-`Result` migration.
Rationale: the existing `Execute(...) (string, bool)` signature is touched by
8+ tools and their tests; widening it churns ~150–250 LoC of mechanical edits
and tests for one tool's benefit. The sentinel keeps the change in two files
and zero existing-tool churn. Migrate to a typed result later if/when more
tools need multimodal output.

19. `executeRead`:
    - Detect MIME: extension first against
      `{.jpg, .jpeg, .png, .webp, .gif}`; if no match, sniff with
      `http.DetectContentType` on first 512 bytes; if still not an image,
      fall through to the existing text path.
    - On image: full read (already capped at 10 MiB upstream),
      `base64.StdEncoding.EncodeToString`, return:
      ```
      __SWITCHBOARD_IMAGE__\t<mime>\t<base64>
      ```
      Sentinel prefix `__SWITCHBOARD_IMAGE__` is unambiguous and tab-delimited
      so the second field can't collide. The bool return stays `false` (not
      an error).
    - On non-image: existing `truncateOutput` text path, unchanged.
20. In `internal/api/backend.go::executeTools`, after calling `Execute`:
    - If the string starts with `__SWITCHBOARD_IMAGE__`: parse out
      `<mime>` and `<base64>`, build `anthropic.ToolResultBlockParam` with
      `Content` = `[ImageBlockParam{Source: Base64ImageSourceParam{...}}]`.
    - Else: existing `anthropic.NewToolResultBlock(id, text, isError)`.
21. Wire `cfg.Model` through `api.BackendFactory.Model` into
    `params.Model` in `callAPI`. Drop or default the existing
    `defaultModel` constant.
22. Tests:
    - Image by extension (jpg/png/webp/gif).
    - Image by MIME sniff (no extension).
    - Text file unchanged.
    - File missing → existing error path.
    - Sentinel parsing in `backend_test.go` (round-trip via mock client).

## Phase 6 — Permission carve-out for media reads

Files: `internal/permission/autoapprove_permission.go` or wrapping logic
in `internal/permission/permission.go`.

23. **Auto-approve `Read` calls whose path is under `WhatsAppMediaDir`,
    regardless of the global `AUTO_APPROVE_WHATSAPP` flag.**

    Rationale: the user explicitly sent the file by uploading it. Re-prompting
    "approve reading /path/foo.jpg?" once per attachment, in a 5-photo burst,
    means 5 round-trips of `WaitForReply` before the model can think.
    Effectively unusable. The carve-out is safe: `WhatsAppMediaDir` is itself
    inside `AllowedDirs`, so the existing path-validator invariant holds.
24. Construction site: thread `WhatsAppMediaDir` into the WhatsApp permission
    checker via `cmd/switchboard/main.go`. Wrapper or new
    `MediaAwarePermissionChecker` that delegates to the underlying checker
    for everything except `Read` under the media dir.
25. Tests:
    - `Read` under media dir → allowed even when wrapped checker would deny.
    - `Read` outside media dir → delegates to wrapped checker.
    - `Bash` / other tools always delegate (carve-out is `Read`-only).

## Phase 7 — Wiring

Files: `cmd/switchboard/main.go`.

26. Plumb `cfg.WhatsAppMediaDir` and a download capability from the
    `whatsmeow.Client` into `NewWhatsAppClientWrapper` and
    `NewWAHandler`.
27. Plumb `cfg.Model` into `api.BackendFactory`.
28. Wrap the WA permission checker with the media-aware carve-out from
    Phase 6.
29. Update `CLAUDE.md`:
    - New env vars: `WHATSAPP_MEDIA_DIR`, `MODEL`.
    - Brief description: attachments arrive as `<attachment>` tags inside
      `<message>` blocks; user-authored skills handle non-image MIMEs.
    - Note that `Read` is auto-approved for paths under
      `WHATSAPP_MEDIA_DIR`.

## Definition of done

- All unit tests pass.
- **Manual smoke test** (acknowledged: there's no integration coverage of
  real `whatsmeow.Client.Download`, so a real photo round-trip is the only
  way to know the decryption path works):
  - Send one image → bot describes it.
  - Send one PDF → user-authored `pdf-reader` skill kicks in, bot summarizes.
  - Send 3 images in a burst → bot responds once, mentioning all three.
  - Send an oversized image → bot replies "skipped (too large)".

## Open question (not yet decided)

**Buffer delay on the common single-message case.** With a 3s debounce, every
text-only message also waits 3s. That's a regression for one-off chats. The
caption-driven flush variant ("if the latest message is text-only with a
caption, flush immediately; otherwise debounce") preserves text latency at
the cost of a more complex rule. Was previously ruled out; flagging here in
case the latency hurts more in practice than expected.

## Out of scope (explicitly)

- Outbound (bot → WA images / image-gen tool / Minimax wiring).
- CLI mode removal (separate PR).
- Voice / video / sticker / location / contact handling.
- `WHATSAPP_MEDIA_DIR` lifecycle (TTL / clear-on-`!new` / etc.).
- Discord-side attachment support.
