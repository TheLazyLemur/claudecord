package core

import (
	"context"
	"log/slog"
	"sync"

	"github.com/pkg/errors"
)

// FlushFunc is invoked against the outgoing backend just before it is
// closed during NewSession, so the agent can persist any unsaved memory
// state. Errors and panics are swallowed; flushes never block the reset.
type FlushFunc func(ctx context.Context, current Backend)

// SessionManager manages the single active backend session
type SessionManager struct {
	mu      sync.RWMutex
	current Backend
	factory BackendFactory
	flush   FlushFunc
}

// NewSessionManager creates a session manager. If flush is non-nil it runs
// against the outgoing backend before it is closed on each NewSession.
func NewSessionManager(factory BackendFactory, flush FlushFunc) *SessionManager {
	return &SessionManager{factory: factory, flush: flush}
}

// NewSession starts a fresh session, closing any existing one
func (m *SessionManager) NewSession(workDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current != nil {
		m.runFlush(m.current)
		m.current.Close()
		m.current = nil
	}

	backend, err := m.factory.Create(workDir)
	if err != nil {
		return errors.Wrap(err, "creating new session")
	}

	m.current = backend
	return nil
}

func (m *SessionManager) runFlush(current Backend) {
	if m.flush == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			slog.Warn("memory flush panicked", "panic", r)
		}
	}()
	m.flush(context.Background(), current)
}

// GetOrCreateSession returns current session or creates one if none exists
func (m *SessionManager) GetOrCreateSession() (Backend, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current != nil {
		return m.current, nil
	}

	backend, err := m.factory.Create("")
	if err != nil {
		return nil, errors.Wrap(err, "creating session")
	}

	m.current = backend
	return m.current, nil
}

// GetSession returns the current session or error if none
func (m *SessionManager) GetSession() (Backend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.current == nil {
		return nil, errors.New("no active session")
	}
	return m.current, nil
}

// Close shuts down the current session
func (m *SessionManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil {
		return nil
	}

	err := m.current.Close()
	m.current = nil
	return err
}
