package core

import (
	"sync"
	"time"
)

type Bot struct {
	sessions        *SessionManager
	perms           PermissionChecker
	mu              sync.RWMutex
	activeKey       SessionKey
	converseTimeout time.Duration
}

// NewBot creates a bot with the given dependencies
func NewBot(sessions *SessionManager, perms PermissionChecker) *Bot {
	return &Bot{
		sessions:        sessions,
		perms:           perms,
		converseTimeout: 10 * time.Minute,
	}
}

// NewSession starts a fresh session with optional working directory.
// Waits for any in-flight HandleInbound to finish before closing the old backend.
func (b *Bot) NewSession(workDir string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.sessions.NewSession(workDir)
}
