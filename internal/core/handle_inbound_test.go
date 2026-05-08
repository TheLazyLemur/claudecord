package core

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubBackend struct {
	id         string
	messages   []string
	closeErr   error
	closed     bool
	converseR  string
	converseErr error
}

func (s *stubBackend) Converse(_ context.Context, msg string, _ Outbound, _ PermissionChecker) (string, error) {
	s.messages = append(s.messages, msg)
	return s.converseR, s.converseErr
}
func (s *stubBackend) SessionID() string { return s.id }
func (s *stubBackend) Close() error      { s.closed = true; return s.closeErr }

type stubFactory struct {
	created []string
	next    func() Backend
}

func (f *stubFactory) Create(workDir string, _ Capabilities) (Backend, error) {
	f.created = append(f.created, workDir)
	return f.next(), nil
}

type stubResponder struct {
	posted    []string
	updates   []string
	reactions []string
}

func (s *stubResponder) SendTyping() error                 { return nil }
func (s *stubResponder) PostResponse(content string) error { s.posted = append(s.posted, content); return nil }
func (s *stubResponder) AddReaction(emoji string) error    { s.reactions = append(s.reactions, emoji); return nil }
func (s *stubResponder) SendUpdate(message string) error   { s.updates = append(s.updates, message); return nil }

type capturingFactory struct {
	backend *stubBackend
	lastCaps Capabilities
}

func (f *capturingFactory) Create(_ string, caps Capabilities) (Backend, error) {
	f.lastCaps = caps
	return f.backend, nil
}

func TestHandleInbound_RejectsEmptySessionKey(t *testing.T) {
	// given
	// ... a bot with a valid backend factory
	be := &stubBackend{id: "b1"}
	f := &stubFactory{next: func() Backend { return be }}
	mgr := NewSessionManager(f, nil)
	bot := NewBot(mgr, nil)

	// when
	// ... an inbound with an empty SessionKey is dispatched
	err := bot.HandleInbound(Inbound{})

	// then
	// ... a non-nil error mentioning SessionKey is returned
	if err == nil {
		t.Fatalf("expected non-nil error for empty SessionKey")
	}
	if !strings.Contains(err.Error(), "SessionKey") {
		t.Fatalf("expected error to mention SessionKey, got: %v", err)
	}
}

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

func TestHandleInbound_ConverseErrorPropagates(t *testing.T) {
	// given
	// ... a backend whose Converse returns an error
	converseErr := errors.New("upstream failure")
	be := &stubBackend{id: "b1", converseErr: converseErr}
	f := &stubFactory{next: func() Backend { return be }}
	mgr := NewSessionManager(f, nil)
	bot := NewBot(mgr, nil)
	r := &stubResponder{}

	// when
	// ... an inbound is dispatched
	err := bot.HandleInbound(Inbound{SessionKey: "k1", Text: "hi", Reply: r})

	// then
	// ... HandleInbound returns a non-nil error wrapping the converse error
	if err == nil {
		t.Fatalf("expected non-nil error, got nil")
	}
	if !errors.Is(err, converseErr) {
		t.Fatalf("expected error to wrap converseErr, got: %v", err)
	}
}

func TestHandleInbound_CapabilitiesForwardedToFactory(t *testing.T) {
	// given
	// ... a capturing factory and a bot
	be := &stubBackend{id: "b1", converseR: "ok"}
	f := &capturingFactory{backend: be}
	mgr := NewSessionManager(f, nil)
	bot := NewBot(mgr, nil)
	r := &stubResponder{}

	// when
	// ... an inbound arrives with Reactions capability set
	_ = bot.HandleInbound(Inbound{
		SessionKey:   "k1",
		Text:         "hi",
		Reply:        r,
		Capabilities: Capabilities{Reactions: true},
	})

	// then
	// ... the factory received the same capabilities when creating the session
	if !f.lastCaps.Reactions {
		t.Fatalf("expected Reactions capability to be forwarded to factory, got: %+v", f.lastCaps)
	}
}

func TestHandleInbound_NoReactionsCapsForwardedToFactory(t *testing.T) {
	// given
	// ... a capturing factory and a bot
	be := &stubBackend{id: "b1", converseR: "ok"}
	f := &capturingFactory{backend: be}
	mgr := NewSessionManager(f, nil)
	bot := NewBot(mgr, nil)
	r := &stubResponder{}

	// when
	// ... an inbound arrives with Reactions capability false
	_ = bot.HandleInbound(Inbound{
		SessionKey:   "k1",
		Text:         "hi",
		Reply:        r,
		Capabilities: Capabilities{Reactions: false},
	})

	// then
	// ... the factory received caps with Reactions false
	if f.lastCaps.Reactions {
		t.Fatalf("expected Reactions false to be forwarded, got: %+v", f.lastCaps)
	}
}
