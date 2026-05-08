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
	activeCaps      Capabilities
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
