# Discord rewrite + shared channel interface — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a shared `ChannelPlugin` interface, refactor WhatsApp + dashboard onto it, then rip out and rebuild the Discord adapter so each Discord thread becomes its own session scope.

**Architecture:** A platform-agnostic `ChannelPlugin` lives in `internal/core`. Each platform implements one plugin under `internal/channels/<name>/` and pushes `Inbound{SessionKey, Text, Attachments, Reply}` to `Bot.HandleInbound`. The `Bot` keeps the existing single-active-session model and uses `SessionKey` to decide reset-or-continue. Discord plugin owns its own thread bookkeeping; `@claude` in a plain channel opens a new thread, `@claude` inside a bot-owned thread continues that thread's session.

**Tech Stack:** Go, discordgo, whatsmeow, gorilla/websocket (dashboard), testify/mock.

**Reference spec:** `docs/superpowers/specs/2026-05-08-discord-rewrite-channel-interface-design.md`

---

## File structure

**Create:**
- `internal/core/channel.go` — `ChannelPlugin`, `Inbound`, `Outbound`, `Capabilities`, `SessionKey` types
- `internal/core/channel_test.go` — interface contract tests
- `internal/core/handle_inbound.go` — `Bot.HandleInbound` flow (separate file to keep `bot.go` small)
- `internal/core/handle_inbound_test.go`
- `internal/channels/discord/plugin.go` — implements `ChannelPlugin`
- `internal/channels/discord/connect.go` — discordgo session lifecycle (token, intents, open/close)
- `internal/channels/discord/threads.go` — thread bookkeeping (which threads we own; create on demand)
- `internal/channels/discord/outbound.go` — `Outbound` impl per inbound message
- `internal/channels/discord/plugin_test.go`
- `internal/channels/discord/threads_test.go`
- `internal/channels/whatsapp/plugin.go` — wraps existing handler logic
- `internal/channels/whatsapp/plugin_test.go`
- `internal/channels/dashboard/plugin.go` — wraps existing SSE/WS handler logic
- `internal/channels/dashboard/plugin_test.go`

**Modify:**
- `internal/core/bot.go` — replace `HandleMessage(Responder, string)` with `HandleInbound(Inbound)`; keep `NewSession` for backwards-compat callers temporarily
- `internal/core/interfaces.go` — keep `Responder`, alias to `Outbound`
- `internal/core/backend.go` — `Converse` keeps `Responder` parameter (renaming to `Outbound` is out of scope; alias makes them interchangeable)
- `internal/core/tools.go` — drop `DiscordTools()`; expose `react_emoji` + `send_update` as discrete builders, conditionally registered
- `cmd/claudecord/main.go` — register plugins per configured tokens; remove `startDiscord` helper
- `cmd/claudecord/startup.go` (or wherever `startDiscord`/`startWhatsApp` live) — delete the per-platform startup helpers, call new plugins instead

**Delete (after migration):**
- `internal/handler/discord.go` — replaced by `internal/channels/discord/`
- `internal/handler/discord_test.go`
- `internal/handler/whatsapp.go` — replaced by `internal/channels/whatsapp/`
- `internal/handler/whatsapp_test.go`
- `internal/core/responder.go` `DiscordResponder`, `WhatsAppResponder` (each plugin owns its own Outbound)

---

## Task 1: Add ChannelPlugin / Inbound / Outbound / Capabilities / SessionKey types

**Files:**
- Create: `internal/core/channel.go`
- Create: `internal/core/channel_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/core/channel_test.go
package core

import (
	"context"
	"testing"
)

func TestChannelPlugin_BasicShape(t *testing.T) {
	// given
	// ... a fake plugin that records its lifecycle
	var started, stopped bool
	p := fakePlugin{
		id:   "fake",
		caps: Capabilities{Reactions: true, Typing: true},
		start: func(ctx context.Context, deliver func(Inbound)) error {
			started = true
			deliver(Inbound{SessionKey: "fake:1", Text: "hi"})
			return nil
		},
		stop: func() error { stopped = true; return nil },
	}

	// when
	// ... the plugin is started and then stopped
	var received Inbound
	_ = p.Start(context.Background(), func(in Inbound) { received = in })
	_ = p.Stop()

	// then
	// ... start ran, stop ran, the inbound was delivered with the expected key
	if !started || !stopped {
		t.Fatalf("lifecycle not invoked: started=%v stopped=%v", started, stopped)
	}
	if received.SessionKey != "fake:1" || received.Text != "hi" {
		t.Fatalf("unexpected inbound: %+v", received)
	}
	if p.ID() != "fake" || !p.Capabilities().Reactions {
		t.Fatalf("metadata mismatch")
	}
}

type fakePlugin struct {
	id    string
	caps  Capabilities
	start func(context.Context, func(Inbound)) error
	stop  func() error
}

func (f fakePlugin) ID() string                                                 { return f.id }
func (f fakePlugin) Capabilities() Capabilities                                 { return f.caps }
func (f fakePlugin) Start(ctx context.Context, d func(Inbound)) error          { return f.start(ctx, d) }
func (f fakePlugin) Stop() error                                                { return f.stop() }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestChannelPlugin_BasicShape`
Expected: FAIL — `undefined: ChannelPlugin`, `Inbound`, `Outbound`, `Capabilities`, `SessionKey`.

- [ ] **Step 3: Implement the types**

```go
// internal/core/channel.go
package core

import "context"

type SessionKey string

type Inbound struct {
	SessionKey  SessionKey
	Text        string
	Attachments []AttachmentRef
	Reply       Outbound
}

// Outbound is the send-side of a channel for a single inbound message.
// It's an alias of Responder so existing Backend.Converse signatures continue
// to work; the new name documents intent at call sites that build channel
// plugins.
type Outbound = Responder

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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestChannelPlugin_BasicShape -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/channel.go internal/core/channel_test.go
git commit -m "feat(core): add ChannelPlugin interface"
```

---

## Task 2: Add Bot.HandleInbound (key-driven session reset)

**Files:**
- Create: `internal/core/handle_inbound.go`
- Create: `internal/core/handle_inbound_test.go`
- Modify: `internal/core/bot.go` (track active SessionKey)

- [ ] **Step 1: Write the failing test**

```go
// internal/core/handle_inbound_test.go
package core

import (
	"context"
	"errors"
	"testing"
)

type stubBackend struct {
	id        string
	messages  []string
	closeErr  error
	closed    bool
	converseR string
}

func (s *stubBackend) Converse(_ context.Context, msg string, _ Responder, _ PermissionChecker) (string, error) {
	s.messages = append(s.messages, msg)
	return s.converseR, nil
}
func (s *stubBackend) SessionID() string { return s.id }
func (s *stubBackend) Close() error      { s.closed = true; return s.closeErr }

type stubFactory struct {
	created []string
	next    func() Backend
}

func (f *stubFactory) Create(workDir string) (Backend, error) {
	f.created = append(f.created, workDir)
	return f.next(), nil
}

type stubResponder struct {
	posted    []string
	updates   []string
	reactions []string
}

func (s *stubResponder) SendTyping() error                  { return nil }
func (s *stubResponder) PostResponse(content string) error  { s.posted = append(s.posted, content); return nil }
func (s *stubResponder) AddReaction(emoji string) error     { s.reactions = append(s.reactions, emoji); return nil }
func (s *stubResponder) SendUpdate(message string) error    { s.updates = append(s.updates, message); return nil }

func TestHandleInbound_SameKeyContinues(t *testing.T) {
	// given
	// ... a single backend created lazily for the first inbound
	be := &stubBackend{id: "b1", converseR: "ack"}
	f := &stubFactory{next: func() Backend { return be }}
	mgr := NewSessionManager(f, nil)
	bot := NewBot(mgr, nil)
	r := &stubResponder{}

	// when
	// ... two inbounds arrive with the same SessionKey
	_ = bot.HandleInbound(Inbound{SessionKey: "k1", Text: "one", Reply: r})
	_ = bot.HandleInbound(Inbound{SessionKey: "k1", Text: "two", Reply: r})

	// then
	// ... only one backend was created and both messages reached it
	if got := len(f.created); got != 1 {
		t.Fatalf("backends created: got %d, want 1", got)
	}
	if got := len(be.messages); got != 2 {
		t.Fatalf("messages forwarded: got %d, want 2", got)
	}
}

func TestHandleInbound_DifferentKeyResets(t *testing.T) {
	// given
	// ... two backends, returned in order from the factory
	be1 := &stubBackend{id: "b1", converseR: "first"}
	be2 := &stubBackend{id: "b2", converseR: "second"}
	queue := []Backend{be1, be2}
	f := &stubFactory{next: func() Backend { b := queue[0]; queue = queue[1:]; return b }}
	mgr := NewSessionManager(f, nil)
	bot := NewBot(mgr, nil)
	r := &stubResponder{}

	// when
	// ... two inbounds arrive with different SessionKeys
	_ = bot.HandleInbound(Inbound{SessionKey: "kA", Text: "a", Reply: r})
	_ = bot.HandleInbound(Inbound{SessionKey: "kB", Text: "b", Reply: r})

	// then
	// ... the first backend was closed, a new backend was created, only the second saw 'b'
	if !be1.closed {
		t.Fatalf("expected first backend to be closed on key switch")
	}
	if got := len(be2.messages); got != 1 || be2.messages[0] != "b" {
		t.Fatalf("second backend messages: %v", be2.messages)
	}
}

func TestHandleInbound_FlushErrorIsSwallowed(t *testing.T) {
	// given
	// ... a flusher that returns an error and a backend that succeeds
	be1 := &stubBackend{id: "b1", converseR: "x", closeErr: nil}
	be2 := &stubBackend{id: "b2", converseR: "y"}
	queue := []Backend{be1, be2}
	f := &stubFactory{next: func() Backend { b := queue[0]; queue = queue[1:]; return b }}
	flushCalled := false
	flush := func(_ context.Context, _ Backend) {
		flushCalled = true
		panic(errors.New("boom"))
	}
	mgr := NewSessionManager(f, flush)
	bot := NewBot(mgr, nil)
	r := &stubResponder{}

	// when
	// ... two different keys land in sequence
	_ = bot.HandleInbound(Inbound{SessionKey: "kA", Text: "a", Reply: r})
	err := bot.HandleInbound(Inbound{SessionKey: "kB", Text: "b", Reply: r})

	// then
	// ... the flush ran, panicked, was swallowed, and the second inbound succeeded
	if !flushCalled {
		t.Fatalf("expected flush to run on key switch")
	}
	if err != nil {
		t.Fatalf("HandleInbound returned error after flush panic: %v", err)
	}
	if got := len(be2.messages); got != 1 {
		t.Fatalf("second backend messages: %v", be2.messages)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestHandleInbound`
Expected: FAIL — `bot.HandleInbound undefined`.

- [ ] **Step 3: Add HandleInbound on Bot**

Modify `internal/core/bot.go` — add an `activeKey` field to `Bot` and a constructor that initialises it to empty:

```go
// internal/core/bot.go (add to existing struct + constructor)
type Bot struct {
	sessions  *SessionManager
	perms     PermissionChecker
	mu        sync.RWMutex
	activeKey SessionKey
}
```

Then create `internal/core/handle_inbound.go`:

```go
// internal/core/handle_inbound.go
package core

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
)

// HandleInbound routes a single inbound message to the active backend, resetting
// the session if the SessionKey has changed since the last inbound.
//
// Concurrency: preserves the existing Bot RWMutex pattern — concurrent inbounds
// for the SAME session key proceed in parallel under RLock; a key mismatch
// upgrades to a write lock and rotates the session, blocking until in-flight
// messages drain (same as today's NewSession behaviour).
func (b *Bot) HandleInbound(in Inbound) error {
	b.mu.RLock()
	matches := in.SessionKey == b.activeKey
	b.mu.RUnlock()

	if !matches {
		b.mu.Lock()
		if in.SessionKey != b.activeKey {
			if err := b.sessions.NewSession(""); err != nil {
				b.mu.Unlock()
				return errors.Wrap(err, "rotating session on key change")
			}
			b.activeKey = in.SessionKey
		}
		b.mu.Unlock()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if in.Reply != nil {
		_ = in.Reply.SendTyping()
	}

	backend, err := b.sessions.GetOrCreateSession()
	if err != nil {
		return errors.Wrap(err, "getting session")
	}

	slog.Info("dispatching inbound", "key", string(in.SessionKey), "session", backend.SessionID())

	response, err := backend.Converse(context.Background(), in.Text, in.Reply, b.perms)
	if err != nil {
		return errors.Wrap(err, "converse")
	}
	if response != "" && in.Reply != nil {
		if err := in.Reply.PostResponse(response); err != nil {
			return errors.Wrap(err, "posting response")
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestHandleInbound -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/bot.go internal/core/handle_inbound.go internal/core/handle_inbound_test.go
git commit -m "feat(core): add Bot.HandleInbound with SessionKey-driven reset"
```

---

## Task 3: WhatsApp ChannelPlugin (wraps existing handler)

**Files:**
- Create: `internal/channels/whatsapp/plugin.go`
- Create: `internal/channels/whatsapp/plugin_test.go`
- Reference: `internal/handler/whatsapp.go` (existing logic to wrap)
- Reference: `internal/core/responder.go` `WhatsAppResponder` (move into the plugin)

- [ ] **Step 1: Write the failing test**

```go
// internal/channels/whatsapp/plugin_test.go
package whatsapp

import (
	"context"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/mock"
)

type messengerMock struct{ mock.Mock }

func (m *messengerMock) SendText(jid, text string) error {
	args := m.Called(jid, text)
	return args.Error(0)
}
func (m *messengerMock) SendTyping(jid string) error {
	args := m.Called(jid)
	return args.Error(0)
}

func TestPlugin_DeliversInboundWithChatJIDAsSessionKey(t *testing.T) {
	// given
	// ... a plugin holding a fake messenger and a delivery sink
	msgr := &messengerMock{}
	p := New(Config{Messenger: msgr, AllowedSenders: []string{"15551234567@s.whatsapp.net"}})
	var received core.Inbound

	// when
	// ... an inbound is simulated through the plugin's deliver path
	_ = p.Start(context.Background(), func(in core.Inbound) { received = in })
	p.deliverForTest(core.Inbound{
		SessionKey: "whatsapp:15551234567@s.whatsapp.net",
		Text:       "hello",
	})

	// then
	// ... the SessionKey is the chat JID prefixed with the channel id
	if received.SessionKey != "whatsapp:15551234567@s.whatsapp.net" {
		t.Fatalf("session key: %q", received.SessionKey)
	}
	if received.Text != "hello" {
		t.Fatalf("text: %q", received.Text)
	}
}

func TestPlugin_Capabilities(t *testing.T) {
	// given
	// ... a default plugin
	p := New(Config{Messenger: &messengerMock{}})

	// when
	// ... capabilities are queried
	caps := p.Capabilities()

	// then
	// ... typing yes, reactions no
	if !caps.Typing || caps.Reactions {
		t.Fatalf("unexpected caps: %+v", caps)
	}
	if p.ID() != "whatsapp" {
		t.Fatalf("id: %q", p.ID())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/channels/whatsapp/ -v`
Expected: FAIL — package missing.

- [ ] **Step 3: Implement the plugin**

```go
// internal/channels/whatsapp/plugin.go
package whatsapp

import (
	"context"
	"sync"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

type Config struct {
	Messenger      core.WhatsAppMessenger
	AllowedSenders []string
	MediaDir       string
}

type Plugin struct {
	cfg     Config
	mu      sync.Mutex
	deliver func(core.Inbound)
}

func New(cfg Config) *Plugin { return &Plugin{cfg: cfg} }

func (p *Plugin) ID() string                  { return "whatsapp" }
func (p *Plugin) Capabilities() core.Capabilities {
	return core.Capabilities{Typing: true, Reactions: false}
}

func (p *Plugin) Start(ctx context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()
	// Existing whatsmeow event wiring will be migrated in a follow-up step
	// (Step 4 below) — this Start is the seam for it.
	return nil
}

func (p *Plugin) Stop() error { return nil }

// deliverForTest exists to drive the deliver callback from unit tests until
// the whatsmeow wiring is migrated in.
func (p *Plugin) deliverForTest(in core.Inbound) {
	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d != nil {
		d(in)
	}
}

// SessionKey returns the canonical key for a chat JID.
func SessionKey(chatJID string) core.SessionKey {
	return core.SessionKey("whatsapp:" + chatJID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/channels/whatsapp/ -v`
Expected: PASS.

- [ ] **Step 5: Migrate handler logic into the plugin**

Read `internal/handler/whatsapp.go`. Move:
- whatsmeow event handler registration (`OnEvent` for `Message`) into `Plugin.Start` — when a message arrives, build an `Outbound` (the existing `WhatsAppResponder`, copied here) and call `p.deliver(core.Inbound{SessionKey: SessionKey(chat), Text: ..., Attachments: ..., Reply: outbound})`.
- The 3-second burst batching debounce as-is (use `core.DebouncedBuffer` already in the repo).
- The media decryption / attachment building (call into `internal/handler/whatsapp_media.go` for now; no API change there).

Move `WhatsAppResponder` from `internal/core/responder.go` into `internal/channels/whatsapp/outbound.go`. Update its package and any `core.WhatsAppMessenger` reference (already an interface, fine).

Delete `internal/handler/whatsapp.go` and its test file once the plugin path covers the same flows.

- [ ] **Step 6: Update existing whatsapp_test scenarios into the plugin's test file**

Re-target `internal/handler/whatsapp_test.go` flows (burst batching, media handling) into `internal/channels/whatsapp/plugin_test.go`, hitting `Plugin.Start` + the deliver callback rather than the old handler entry point.

- [ ] **Step 7: Run full package tests**

Run: `go test ./internal/channels/whatsapp/ ./internal/core/ -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/channels/whatsapp/ internal/core/responder.go internal/handler/whatsapp.go internal/handler/whatsapp_test.go
git commit -m "refactor(whatsapp): port handler to ChannelPlugin"
```

---

## Task 4: Dashboard ChannelPlugin (wraps existing WS handler)

**Files:**
- Create: `internal/channels/dashboard/plugin.go`
- Create: `internal/channels/dashboard/plugin_test.go`
- Reference: `internal/dashboard/chat.go`, `internal/dashboard/responder.go`, `internal/dashboard/server.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/channels/dashboard/plugin_test.go
package dashboard

import (
	"context"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

func TestPlugin_DeliversWithSessionUUIDKey(t *testing.T) {
	// given
	// ... a plugin and a captured inbound
	p := New(Config{})
	var got core.Inbound

	// when
	// ... the plugin delivers a synthetic inbound with a session id
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })
	p.deliverForTest(core.Inbound{SessionKey: SessionKey("abc-123"), Text: "hi"})

	// then
	// ... the SessionKey is dashboard-prefixed
	if got.SessionKey != "dashboard:abc-123" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
}

func TestPlugin_Capabilities(t *testing.T) {
	// given
	// ... a default plugin
	p := New(Config{})

	// when/then
	caps := p.Capabilities()
	if caps.Reactions || !caps.Typing {
		t.Fatalf("unexpected caps: %+v", caps)
	}
	if p.ID() != "dashboard" {
		t.Fatalf("id: %q", p.ID())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/channels/dashboard/ -v`
Expected: FAIL — package missing.

- [ ] **Step 3: Implement the plugin**

```go
// internal/channels/dashboard/plugin.go
package dashboard

import (
	"context"
	"sync"

	"github.com/TheLazyLemur/claudecord/internal/core"
	dash "github.com/TheLazyLemur/claudecord/internal/dashboard"
)

type Config struct {
	Hub *dash.Hub
}

type Plugin struct {
	cfg     Config
	mu      sync.Mutex
	deliver func(core.Inbound)
}

func New(cfg Config) *Plugin { return &Plugin{cfg: cfg} }

func (p *Plugin) ID() string                  { return "dashboard" }
func (p *Plugin) Capabilities() core.Capabilities {
	return core.Capabilities{Typing: true, Reactions: false}
}

func (p *Plugin) Start(ctx context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Stop() error { return nil }

func (p *Plugin) deliverForTest(in core.Inbound) {
	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d != nil {
		d(in)
	}
}

func SessionKey(sessionUUID string) core.SessionKey {
	return core.SessionKey("dashboard:" + sessionUUID)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/channels/dashboard/ -v`
Expected: PASS.

- [ ] **Step 5: Wire dashboard chat handler into the plugin**

In `internal/dashboard/chat.go` `handleChat`, replace the direct call into `Bot.HandleMessage` (or whatever it dispatches today) with a call into the plugin's `deliver` channel. Concretely: the dashboard `Server` constructor takes a `func(core.Inbound)` it forwards to. `cmd/claudecord/main.go` (Task 8) supplies that callback by registering the dashboard plugin and wiring its deliver to `bot.HandleInbound`.

`WSResponder` moves from `internal/dashboard/responder.go` into `internal/channels/dashboard/outbound.go` (or stays in `internal/dashboard/` if cycle-avoidance demands it). Either is fine; pick whatever the import graph allows.

- [ ] **Step 6: Run dashboard package tests**

Run: `go test ./internal/dashboard/ ./internal/channels/dashboard/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/channels/dashboard/ internal/dashboard/chat.go internal/dashboard/responder.go internal/dashboard/server.go
git commit -m "refactor(dashboard): port chat handler to ChannelPlugin"
```

---

## Task 5: Delete old Discord adapter (keep token + intents only)

**Files:**
- Delete: `internal/handler/discord.go` body
- Delete: `internal/handler/discord_test.go`
- Modify: `internal/core/responder.go` — remove `DiscordResponder`
- Modify: `internal/core/interfaces.go` — remove `DiscordClient` interface (moves into plugin)
- Modify: `internal/core/tools.go` — remove `DiscordTools()`; turn `react_emoji` and `send_update` into individual builder funcs

- [ ] **Step 1: Confirm no current callers depend on `DiscordResponder` outside the to-be-deleted handler**

Run: `grep -rn "DiscordResponder\|DiscordTools\|DiscordClient" --include='*.go'`
Expected: results only in `internal/handler/discord.go`, `internal/core/responder.go`, `internal/core/interfaces.go`, `internal/core/tools.go`, and tests in those files. If results show up in `cmd/` or anywhere else, stop and reconcile before deleting.

- [ ] **Step 2: Delete the file bodies**

```bash
git rm internal/handler/discord.go internal/handler/discord_test.go
```

Edit `internal/core/responder.go` — remove the `DiscordResponder` struct and methods (lines 25–66 in the current file). Leave `WhatsAppResponder` if Task 3 hasn't run yet; if it has, this file may already be empty and can also be removed.

Edit `internal/core/interfaces.go` — remove the `DiscordClient` interface block.

Edit `internal/core/tools.go` — remove `DiscordTools()` definition. Add (replacing it):

```go
// ReactEmojiTool is the optional tool registered when the active channel
// reports Capabilities.Reactions == true.
func ReactEmojiTool() ToolDef {
	return ToolDef{
		Name:        "react_emoji",
		Description: "Add an emoji reaction to the message you are responding to.",
		InputSchema: objSchema(map[string]any{
			"emoji": strProp("Unicode emoji character (e.g. 👀, 👍, 🚀)"),
		}, "emoji"),
	}
}

// SendUpdateTool is registered for every channel.
func SendUpdateTool() ToolDef {
	return ToolDef{
		Name:        "send_update",
		Description: "Send a progress update message in the current conversation. Use to keep the user informed about what you're doing.",
		InputSchema: objSchema(map[string]any{
			"message": strProp("The update message to send"),
		}, "message"),
	}
}
```

Update any caller of `DiscordTools()` / `ChatTools()` to compose tools from the new individual builders.

- [ ] **Step 3: Compile**

Run: `go build ./...`
Expected: errors limited to `cmd/claudecord/` startup (the per-platform helpers we're about to delete) and any test that hasn't been retargeted yet. Note them; they get fixed in Task 8.

- [ ] **Step 4: Commit (red build is OK — main.go gets fixed in Task 8)**

```bash
git add internal/handler/ internal/core/responder.go internal/core/interfaces.go internal/core/tools.go
git commit -m "refactor(discord): remove old adapter (build red until task 8)"
```

---

## Task 6: New Discord plugin — connection + thread bookkeeping

**Files:**
- Create: `internal/channels/discord/connect.go`
- Create: `internal/channels/discord/threads.go`
- Create: `internal/channels/discord/threads_test.go`

- [ ] **Step 1: Write the failing test for the thread registry**

```go
// internal/channels/discord/threads_test.go
package discord

import "testing"

func TestThreadRegistry_RemembersOwnership(t *testing.T) {
	// given
	// ... an empty registry
	r := newThreadRegistry()

	// when
	// ... a thread id is registered as bot-owned
	r.markOwned("thread-123")

	// then
	// ... ownership is reported true for that id and false for others
	if !r.owns("thread-123") {
		t.Fatalf("expected ownership of thread-123")
	}
	if r.owns("thread-999") {
		t.Fatalf("did not expect ownership of thread-999")
	}
}

func TestThreadRegistry_ConcurrentSafe(t *testing.T) {
	// given
	// ... a registry hit from multiple goroutines
	r := newThreadRegistry()
	done := make(chan struct{})

	// when
	// ... 100 concurrent writes complete
	for i := 0; i < 100; i++ {
		go func(i int) {
			r.markOwned(fmtThread(i))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	// then
	// ... every id is reported owned
	for i := 0; i < 100; i++ {
		if !r.owns(fmtThread(i)) {
			t.Fatalf("missing %s", fmtThread(i))
		}
	}
}

func fmtThread(i int) string { return "t-" + itoa(i) }

func itoa(i int) string {
	const digits = "0123456789"
	if i == 0 {
		return "0"
	}
	var b [20]byte
	bp := len(b)
	for i > 0 {
		bp--
		b[bp] = digits[i%10]
		i /= 10
	}
	return string(b[bp:])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/channels/discord/ -v`
Expected: FAIL — package missing.

- [ ] **Step 3: Implement the registry**

```go
// internal/channels/discord/threads.go
package discord

import "sync"

// threadRegistry tracks which Discord thread IDs were created by us.
// Used to decide whether an inbound message inside a thread should continue
// that thread's session or spawn a new sibling thread.
type threadRegistry struct {
	mu    sync.RWMutex
	owned map[string]struct{}
}

func newThreadRegistry() *threadRegistry {
	return &threadRegistry{owned: make(map[string]struct{})}
}

func (r *threadRegistry) markOwned(threadID string) {
	r.mu.Lock()
	r.owned[threadID] = struct{}{}
	r.mu.Unlock()
}

func (r *threadRegistry) owns(threadID string) bool {
	r.mu.RLock()
	_, ok := r.owned[threadID]
	r.mu.RUnlock()
	return ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/channels/discord/ -v`
Expected: PASS.

- [ ] **Step 5: Add the connect helper**

```go
// internal/channels/discord/connect.go
package discord

import (
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

// connect opens a discordgo session with the message intents we need.
// Caller is responsible for calling session.Close().
func connect(token string) (*discordgo.Session, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, errors.Wrap(err, "creating discord session")
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent | discordgo.IntentDirectMessages
	if err := dg.Open(); err != nil {
		return nil, errors.Wrap(err, "opening discord session")
	}
	return dg, nil
}
```

- [ ] **Step 6: Compile**

Run: `go build ./internal/channels/discord/`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/channels/discord/connect.go internal/channels/discord/threads.go internal/channels/discord/threads_test.go
git commit -m "feat(discord): connect helper + thread registry"
```

---

## Task 7: Discord Outbound (per-message responder bound to a thread)

**Files:**
- Create: `internal/channels/discord/outbound.go`
- Create: `internal/channels/discord/outbound_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/channels/discord/outbound_test.go
package discord

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
)

type discordSessionMock struct{ mock.Mock }

func (m *discordSessionMock) ChannelMessageSend(channelID, content string) error {
	args := m.Called(channelID, content)
	return args.Error(0)
}
func (m *discordSessionMock) ChannelTyping(channelID string) error {
	return m.Called(channelID).Error(0)
}
func (m *discordSessionMock) MessageReactionAdd(channelID, messageID, emoji string) error {
	return m.Called(channelID, messageID, emoji).Error(0)
}

const maxLen = 2000

func TestOutbound_PostResponse_ChunksLongMessages(t *testing.T) {
	// given
	// ... an outbound bound to a thread and a >2000-char payload
	s := &discordSessionMock{}
	o := newOutbound(s, "thread-1", "msg-1", maxLen)
	long := strings.Repeat("x", maxLen+50)
	s.On("ChannelMessageSend", "thread-1", mock.Anything).Return(nil).Twice()

	// when
	// ... PostResponse is called
	err := o.PostResponse(long)

	// then
	// ... the payload was sent in 2 chunks to the thread
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.AssertNumberOfCalls(t, "ChannelMessageSend", 2)
}

func TestOutbound_AddReaction_TargetsOriginalMessage(t *testing.T) {
	// given
	// ... an outbound with a known message id
	s := &discordSessionMock{}
	o := newOutbound(s, "thread-1", "msg-42", maxLen)
	s.On("MessageReactionAdd", "thread-1", "msg-42", "👀").Return(nil).Once()

	// when
	err := o.AddReaction("👀")

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.AssertExpectations(t)
}

func TestOutbound_SendUpdate_PostsInSameThread(t *testing.T) {
	// given
	// ... an outbound bound to a thread
	s := &discordSessionMock{}
	o := newOutbound(s, "thread-1", "msg-1", maxLen)
	s.On("ChannelMessageSend", "thread-1", "doing the thing").Return(nil).Once()

	// when
	err := o.SendUpdate("doing the thing")

	// then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.AssertExpectations(t)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/channels/discord/ -run TestOutbound -v`
Expected: FAIL — `newOutbound` undefined.

- [ ] **Step 3: Implement Outbound**

```go
// internal/channels/discord/outbound.go
package discord

import (
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/pkg/errors"
)

// discordSession is the slice of *discordgo.Session that outbound needs.
// Defined as an interface so tests can mock it.
type discordSession interface {
	ChannelMessageSend(channelID, content string) error
	ChannelTyping(channelID string) error
	MessageReactionAdd(channelID, messageID, emoji string) error
}

type outbound struct {
	s         discordSession
	threadID  string
	messageID string
	maxLen    int
}

func newOutbound(s discordSession, threadID, messageID string, maxLen int) *outbound {
	return &outbound{s: s, threadID: threadID, messageID: messageID, maxLen: maxLen}
}

func (o *outbound) SendTyping() error {
	return errors.Wrap(o.s.ChannelTyping(o.threadID), "discord typing")
}

func (o *outbound) PostResponse(content string) error {
	for _, chunk := range core.ChunkMessage(content, o.maxLen) {
		if err := o.s.ChannelMessageSend(o.threadID, chunk); err != nil {
			return errors.Wrap(err, "discord send")
		}
	}
	return nil
}

func (o *outbound) AddReaction(emoji string) error {
	return errors.Wrap(o.s.MessageReactionAdd(o.threadID, o.messageID, emoji), "discord react")
}

func (o *outbound) SendUpdate(message string) error {
	return errors.Wrap(o.s.ChannelMessageSend(o.threadID, message), "discord update")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/channels/discord/ -run TestOutbound -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/channels/discord/outbound.go internal/channels/discord/outbound_test.go
git commit -m "feat(discord): per-message Outbound bound to thread"
```

---

## Task 8: Discord Plugin — inbound + thread spawn rules

**Files:**
- Create: `internal/channels/discord/plugin.go`
- Create: `internal/channels/discord/plugin_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/channels/discord/plugin_test.go
package discord

import (
	"context"
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/mock"
)

type sessionFull struct {
	discordSessionMock
}

func (s *sessionFull) MessageThreadStartComplex(channelID, messageID string, name string) (string, error) {
	args := s.Called(channelID, messageID, name)
	return args.String(0), args.Error(1)
}

func TestPlugin_AtClaudeInPlainChannel_OpensNewThread(t *testing.T) {
	// given
	// ... a plugin bound to a fake discord session and an empty registry
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-1", mock.Anything).Return("thread-new", nil).Once()
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... a MessageCreate event arrives in a plain channel mentioning the bot
	p.handleMessageForTest(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "channel-1",
		MessageID: "msg-1",
		Content:   "@claude do the thing",
		IsThread:  false,
	})

	// then
	// ... a new thread was opened and the SessionKey points at it
	if got.SessionKey != "discord:thread:thread-new" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
	if got.Text != "do the thing" {
		t.Fatalf("text: %q", got.Text)
	}
	s.AssertExpectations(t)
}

func TestPlugin_AtClaudeInOwnedThread_StaysInThread(t *testing.T) {
	// given
	// ... the plugin already owns thread-existing
	s := &sessionFull{}
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	p.threads.markOwned("thread-existing")
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... an @claude message lands inside that thread
	p.handleMessageForTest(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "thread-existing",
		ParentID:  "channel-1",
		MessageID: "msg-9",
		Content:   "@claude follow up",
		IsThread:  true,
	})

	// then
	// ... no new thread is created and SessionKey reuses the thread id
	s.AssertNotCalled(t, "MessageThreadStartComplex", mock.Anything, mock.Anything, mock.Anything)
	if got.SessionKey != "discord:thread:thread-existing" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
}

func TestPlugin_AtClaudeInForeignThread_OpensSiblingThread(t *testing.T) {
	// given
	// ... the plugin does NOT own this thread
	s := &sessionFull{}
	s.On("MessageThreadStartComplex", "channel-1", "msg-7", mock.Anything).Return("thread-sibling", nil).Once()
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	// ... an @claude message arrives in a foreign thread
	p.handleMessageForTest(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "thread-foreign",
		ParentID:  "channel-1",
		MessageID: "msg-7",
		Content:   "@claude do it",
		IsThread:  true,
	})

	// then
	// ... a new sibling thread is opened on the parent channel
	if got.SessionKey != "discord:thread:thread-sibling" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
	s.AssertExpectations(t)
}

func TestPlugin_NoMention_Ignored(t *testing.T) {
	// given
	// ... a plugin started with a sink
	s := &sessionFull{}
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	called := false
	_ = p.Start(context.Background(), func(in core.Inbound) { called = true })

	// when
	// ... a message without @claude arrives
	p.handleMessageForTest(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "channel-1",
		MessageID: "msg-2",
		Content:   "just chatting",
	})

	// then
	// ... no inbound was delivered
	if called {
		t.Fatalf("expected handler not to deliver for non-@claude message")
	}
}

func TestPlugin_DM_UsesDMSessionKey(t *testing.T) {
	// given
	// ... a plugin and a DM event
	s := &sessionFull{}
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	var got core.Inbound
	_ = p.Start(context.Background(), func(in core.Inbound) { got = in })

	// when
	p.handleMessageForTest(messageEvent{
		AuthorID:  "user-1",
		ChannelID: "dm-channel",
		MessageID: "msg-3",
		Content:   "@claude hello",
		IsDM:      true,
	})

	// then
	if got.SessionKey != "discord:dm:user-1" {
		t.Fatalf("session key: %q", got.SessionKey)
	}
}

func TestPlugin_DisallowedUser_Ignored(t *testing.T) {
	// given
	// ... a plugin where user-2 is not allowed
	s := &sessionFull{}
	p := newPluginForTest(s, "bot-id", []string{"user-1"})
	called := false
	_ = p.Start(context.Background(), func(in core.Inbound) { called = true })

	// when
	p.handleMessageForTest(messageEvent{
		AuthorID:  "user-2",
		ChannelID: "channel-1",
		MessageID: "msg-4",
		Content:   "@claude hello",
	})

	// then
	if called {
		t.Fatalf("expected message from disallowed user to be ignored")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/channels/discord/ -run TestPlugin -v`
Expected: FAIL — `newPluginForTest`, `messageEvent`, `Plugin` undefined.

- [ ] **Step 3: Implement the plugin**

```go
// internal/channels/discord/plugin.go
package discord

import (
	"context"
	"strings"
	"sync"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/pkg/errors"
)

const maxDiscordMessageLen = 2000

// messageEvent is the platform-agnostic shape of a Discord MessageCreate that
// the plugin's tests and adapter both speak. Real discordgo events get
// translated into this struct in Start.
type messageEvent struct {
	AuthorID  string
	ChannelID string
	ParentID  string // populated when IsThread is true
	MessageID string
	Content   string
	IsThread  bool
	IsDM      bool
}

type Config struct {
	Token          string
	BotID          string
	AllowedUsers   []string
}

type Plugin struct {
	cfg     Config
	session sessionForPlugin
	threads *threadRegistry
	mu      sync.Mutex
	deliver func(core.Inbound)
}

// sessionForPlugin is the slice of *discordgo.Session the plugin needs at
// runtime. Defined as an interface so plugin_test can mock it.
type sessionForPlugin interface {
	discordSession
	MessageThreadStartComplex(channelID, messageID, name string) (string, error)
}

func New(cfg Config) *Plugin {
	return &Plugin{cfg: cfg, threads: newThreadRegistry()}
}

func (p *Plugin) ID() string { return "discord" }

func (p *Plugin) Capabilities() core.Capabilities {
	return core.Capabilities{Reactions: true, Typing: true}
}

func (p *Plugin) Start(ctx context.Context, deliver func(core.Inbound)) error {
	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()

	// Real discordgo wiring lives in Start as a follow-up step (Task 9 below);
	// tests drive handleMessageForTest directly so plugin behaviour is covered
	// without a live socket.
	return nil
}

func (p *Plugin) Stop() error { return nil }

// handleMessageForTest is the seam tests use. Production discordgo handler
// (registered in Start by Task 9) translates a *discordgo.MessageCreate into a
// messageEvent and forwards here.
func (p *Plugin) handleMessageForTest(ev messageEvent) {
	p.handleMessage(ev)
}

func (p *Plugin) handleMessage(ev messageEvent) {
	if !p.userAllowed(ev.AuthorID) {
		return
	}
	cleaned, ok := stripMention(ev.Content)
	if !ok {
		return
	}

	threadID, err := p.resolveThread(ev)
	if err != nil {
		return
	}

	p.mu.Lock()
	d := p.deliver
	p.mu.Unlock()
	if d == nil {
		return
	}

	d(core.Inbound{
		SessionKey: sessionKey(ev, threadID),
		Text:       cleaned,
		Reply:      newOutbound(p.session, threadID, ev.MessageID, maxDiscordMessageLen),
	})
}

func (p *Plugin) resolveThread(ev messageEvent) (string, error) {
	if ev.IsDM {
		return ev.ChannelID, nil
	}
	if ev.IsThread && p.threads.owns(ev.ChannelID) {
		return ev.ChannelID, nil
	}
	parent := ev.ChannelID
	if ev.IsThread {
		parent = ev.ParentID
	}
	tid, err := p.session.MessageThreadStartComplex(parent, ev.MessageID, threadName(ev.Content))
	if err != nil {
		return "", errors.Wrap(err, "creating thread")
	}
	p.threads.markOwned(tid)
	return tid, nil
}

func sessionKey(ev messageEvent, threadID string) core.SessionKey {
	if ev.IsDM {
		return core.SessionKey("discord:dm:" + ev.AuthorID)
	}
	return core.SessionKey("discord:thread:" + threadID)
}

func (p *Plugin) userAllowed(userID string) bool {
	for _, u := range p.cfg.AllowedUsers {
		if u == userID {
			return true
		}
	}
	return false
}

func stripMention(content string) (string, bool) {
	const tag = "@claude"
	trim := strings.TrimSpace(content)
	if !strings.HasPrefix(strings.ToLower(trim), tag) {
		return "", false
	}
	return strings.TrimSpace(trim[len(tag):]), true
}

func threadName(content string) string {
	t := strings.TrimSpace(content)
	if len(t) > 50 {
		t = t[:50]
	}
	if t == "" {
		t = "Claude"
	}
	return t
}

// newPluginForTest constructs a plugin with a pre-injected session, bypassing
// the discordgo Open path. Test-only.
func newPluginForTest(s sessionForPlugin, botID string, allowed []string) *Plugin {
	p := New(Config{BotID: botID, AllowedUsers: allowed})
	p.session = s
	return p
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/channels/discord/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/channels/discord/plugin.go internal/channels/discord/plugin_test.go
git commit -m "feat(discord): plugin with thread-as-session routing"
```

---

## Task 9: Wire discordgo MessageCreate into Plugin.Start

**Files:**
- Modify: `internal/channels/discord/plugin.go`

- [ ] **Step 1: Replace the placeholder Start body with real discordgo wiring**

Edit `internal/channels/discord/plugin.go`:

```go
import (
	"github.com/bwmarrin/discordgo"
)

func (p *Plugin) Start(ctx context.Context, deliver func(core.Inbound)) error {
	dg, err := connect(p.cfg.Token)
	if err != nil {
		return err
	}
	p.session = sessionAdapter{dg}
	p.cfg.BotID = dg.State.User.ID

	p.mu.Lock()
	p.deliver = deliver
	p.mu.Unlock()

	dg.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		if m.Author == nil || m.Author.ID == p.cfg.BotID {
			return
		}
		ev := messageEvent{
			AuthorID:  m.Author.ID,
			ChannelID: m.ChannelID,
			MessageID: m.ID,
			Content:   m.Content,
		}
		// Discord delivers DMs with GuildID == "".
		if m.GuildID == "" {
			ev.IsDM = true
		}
		// Channel type tells us whether we're inside a thread; cheapest is to
		// check m.ChannelID against the channel cache.
		if ch, err := dg.State.Channel(m.ChannelID); err == nil && ch.IsThread() {
			ev.IsThread = true
			ev.ParentID = ch.ParentID
		}
		p.handleMessage(ev)
	})

	return nil
}

// sessionAdapter wraps *discordgo.Session to satisfy sessionForPlugin.
type sessionAdapter struct{ *discordgo.Session }

func (s sessionAdapter) ChannelMessageSend(channelID, content string) error {
	_, err := s.Session.ChannelMessageSend(channelID, content)
	return err
}
func (s sessionAdapter) ChannelTyping(channelID string) error {
	return s.Session.ChannelTyping(channelID)
}
func (s sessionAdapter) MessageReactionAdd(channelID, messageID, emoji string) error {
	return s.Session.MessageReactionAdd(channelID, messageID, emoji)
}
func (s sessionAdapter) MessageThreadStartComplex(channelID, messageID, name string) (string, error) {
	t, err := s.Session.MessageThreadStartComplex(channelID, messageID, &discordgo.ThreadStart{
		Name:                name,
		AutoArchiveDuration: 60,
	})
	if err != nil {
		return "", err
	}
	return t.ID, nil
}
```

Add `Stop`:

```go
func (p *Plugin) Stop() error {
	if dg, ok := p.session.(sessionAdapter); ok && dg.Session != nil {
		return dg.Session.Close()
	}
	return nil
}
```

- [ ] **Step 2: Compile**

Run: `go build ./internal/channels/discord/`
Expected: PASS.

- [ ] **Step 3: Re-run plugin tests (should still pass — they use sessionForPlugin mock, not the live session)**

Run: `go test ./internal/channels/discord/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/channels/discord/plugin.go
git commit -m "feat(discord): wire discordgo MessageCreate to plugin"
```

---

## Task 10: Wire all three plugins from main.go (rebuild fixes red build)

**Files:**
- Modify: `cmd/claudecord/main.go`
- Delete: `cmd/claudecord/startup.go` `startDiscord` (and other per-platform helpers if separate)

- [ ] **Step 1: Confirm current main.go shape**

Run: `grep -n "startDiscord\|startWhatsApp\|startDashboard\|NewBot\|HandleMessage" cmd/claudecord/*.go`
Note the entry points. Each `start*` returns a stop func today; we replace them with a single plugin loop.

- [ ] **Step 2: Replace per-platform startup with a plugin loop**

Replace the `if cfg.DiscordEnabled() { ... }` / WhatsApp / dashboard branches in `cmd/claudecord/main.go` with:

```go
import (
	dashboardchan "github.com/TheLazyLemur/claudecord/internal/channels/dashboard"
	discordchan  "github.com/TheLazyLemur/claudecord/internal/channels/discord"
	whatsappchan "github.com/TheLazyLemur/claudecord/internal/channels/whatsapp"
)

var plugins []core.ChannelPlugin

if cfg.DiscordEnabled() {
	plugins = append(plugins, discordchan.New(discordchan.Config{
		Token:        cfg.DiscordToken,
		AllowedUsers: cfg.AllowedUsers,
	}))
}
if cfg.WhatsAppEnabled() {
	plugins = append(plugins, whatsappchan.New(whatsappchan.Config{
		Messenger:      waMessenger, // existing whatsmeow client wrapper
		AllowedSenders: cfg.WhatsAppAllowedSenders,
		MediaDir:       cfg.WhatsAppMediaDir,
	}))
}
if cfg.DashboardEnabled() {
	plugins = append(plugins, dashboardchan.New(dashboardchan.Config{Hub: hub}))
}

ctx, cancel := context.WithCancel(context.Background())
defer cancel()

for _, p := range plugins {
	plug := p
	if err := plug.Start(ctx, bot.HandleInbound); err != nil {
		return errors.Wrapf(err, "starting plugin %s", plug.ID())
	}
	defer func() {
		if err := plug.Stop(); err != nil {
			slog.Warn("plugin stop", "plugin", plug.ID(), "error", err)
		}
	}()
}

slog.Info("plugins started", "count", len(plugins))
// existing signal-wait stays as-is
```

Delete `startDiscord` (and `startWhatsApp` if it lives nearby) — the new plugins replace them. The dashboard `Server` constructor needs a tweak to dispatch incoming chat into the dashboard plugin's deliver channel; pass the plugin's deliver function (or a setter) when constructing the server.

Adjust `core.NewBot` callsite if needed; the bot now takes the same args (no signature change in this plan).

- [ ] **Step 3: Build the binary**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/claudecord/ internal/dashboard/server.go
git commit -m "feat(main): register channel plugins and dispatch via Bot.HandleInbound"
```

---

## Task 11: Drop /new-session slash command + capability-gate react_emoji

**Files:**
- Modify: `cmd/claudecord/main.go` (remove command registration if it lives there)
- Modify: `internal/channels/discord/plugin.go` (no slash command registration; if it was registered elsewhere, remove)
- Modify: `internal/api/backend.go` or wherever tools are composed for the model — register `react_emoji` only when active plugin reports `Reactions: true`

- [ ] **Step 1: Find all `/new-session` registrations and handlers**

Run: `grep -rn "new-session\|NewSession" --include='*.go'`
Note hits. Anything in `cmd/`, `internal/handler/`, or `internal/dashboard/` that invokes `/new-session` from a UI surface gets removed.

- [ ] **Step 2: Remove command registration + handlers**

Delete `case "new-session"` blocks and their slash command registration. Leave `Bot.NewSession` and `SessionManager.NewSession` in place — `HandleInbound` calls them on key change.

- [ ] **Step 3: Capability-gate react_emoji**

Find where `core.ChatTools()` / `ReactEmojiTool()` are added to the tool list passed to the backend. Conditional becomes:

```go
tools := []core.ToolDef{core.SendUpdateTool()}
if activePlugin.Capabilities().Reactions {
	tools = append(tools, core.ReactEmojiTool())
}
```

If "active plugin" isn't naturally available where tools are composed (e.g. tools are picked at session-create time, before any inbound has arrived), use a coarser default: tools include `react_emoji` if **any** registered plugin has Reactions, then the plugin's `Outbound.AddReaction` is a no-op when not supported. Pick whichever is simplest in the existing code; both are correct under this design.

- [ ] **Step 4: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add .
git commit -m "feat: drop /new-session and capability-gate react_emoji"
```

---

## Task 12: Manual smoke test + cleanup pass

**Files:** none (verification only)

- [ ] **Step 1: Build and run locally**

Run: `go build -o /tmp/claudecord ./cmd/claudecord && /tmp/claudecord`
Expected: starts cleanly, logs `plugins started count=N` for the configured channels.

- [ ] **Step 2: Discord smoke**

In a guild channel where the bot is present:
1. Post `@claude what is 2+2`. Verify a new thread is created and the response posts in it.
2. In the same thread, post `@claude and 3+3`. Verify the response posts in the same thread (no new thread spawned).
3. Back in the parent channel, post `@claude unrelated thing`. Verify a *new* thread is spawned and the previous thread's session is no longer the active one.
4. DM the bot `@claude hello`. Verify it responds in the DM.

- [ ] **Step 3: Cross-platform smoke**

1. Send a WhatsApp message to the bot. Verify it responds. (Switching surface should reset the session — confirm that asking it to recall the Discord conversation fails.)
2. Open the dashboard, send a message. Verify response.

- [ ] **Step 4: Final grep — no orphans**

Run: `grep -rn "DiscordResponder\|DiscordTools\|DiscordClient\|new-session" --include='*.go'`
Expected: zero hits (or only in comments/docs that are knowingly retained).

- [ ] **Step 5: Commit any cleanup**

```bash
git add -p
git commit -m "chore: clean up dead references after channel-plugin migration"
```

---

## Self-review notes

- **Spec coverage check:** every section of the spec maps to a task — interface (Task 1), bot routing (Task 2), WhatsApp plugin (Task 3), dashboard plugin (Task 4), Discord deletion (Task 5), Discord rebuild (Tasks 6–9), main wiring (Task 10), tool gating + slash drop (Task 11), smoke test (Task 12). Non-goals are not implemented.
- **Build-red window:** Task 5 leaves the build broken intentionally; Task 10 closes it. Don't ship after Task 5 without 6–10.
- **Type consistency:** `Outbound` is a Go alias of the existing `Responder` (Task 1) — same methods, no migration churn. `SessionKey` is a string subtype carried unchanged through Inbound and compared in Bot.
- **Naming:** plugin packages are `discord`, `whatsapp`, `dashboard` under `internal/channels/`. To avoid clashing with the existing top-level `internal/dashboard` package, importers alias it (`dashboardchan "..."`).
