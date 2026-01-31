package core

import (
	"sync"

	"github.com/pkg/errors"
)

// SessionManager manages the single active backend session
type SessionManager struct {
	mu      sync.RWMutex
	current Backend
	factory BackendFactory
}

// NewSessionManager creates a session manager with the given backend factory
func NewSessionManager(factory BackendFactory) *SessionManager {
	return &SessionManager{factory: factory}
}

// NewSession starts a fresh session, closing any existing one
func (m *SessionManager) NewSession(workDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current != nil {
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
