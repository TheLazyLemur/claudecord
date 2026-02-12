package core

import (
	"context"
	"log/slog"
	"sync"

	"github.com/pkg/errors"
)

// Bot orchestrates backend sessions
type Bot struct {
	sessions *SessionManager
	perms    PermissionChecker
	mu       sync.Mutex
}

// NewBot creates a bot with the given dependencies
func NewBot(sessions *SessionManager, perms PermissionChecker) *Bot {
	return &Bot{
		sessions: sessions,
		perms:    perms,
	}
}

// HandleMessage processes a message via the backend
func (b *Bot) HandleMessage(responder Responder, userMessage string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	slog.Info("HandleMessage start", "msg", userMessage)
	responder.SendTyping()

	slog.Info("getting session")
	backend, err := b.sessions.GetOrCreateSession()
	if err != nil {
		return errors.Wrap(err, "getting session")
	}
	slog.Info("got session", "sessionID", backend.SessionID())

	ctx := context.Background()
	response, err := backend.Converse(ctx, userMessage, responder, b.perms)
	if err != nil {
		return errors.Wrap(err, "conversing")
	}

	if response != "" {
		if err := responder.PostResponse(response); err != nil {
			return errors.Wrap(err, "posting response")
		}
	}

	return nil
}

// NewSession starts a fresh session with optional working directory.
// Waits for any in-flight HandleMessage to finish before closing the old backend.
func (b *Bot) NewSession(workDir string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.sessions.NewSession(workDir)
}
