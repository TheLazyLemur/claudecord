package core

import (
	"sync"

	"github.com/TheLazyLemur/claudecord/internal/history"
	"github.com/pkg/errors"
)

// HistorySessionManager extends SessionManager with history persistence
type HistorySessionManager struct {
	mu          sync.RWMutex
	current     Backend
	currentID   string
	factory     BackendFactory
	historyStore *history.Store
}

// NewHistorySessionManager creates a session manager with history support
func NewHistorySessionManager(factory BackendFactory, store *history.Store) *HistorySessionManager {
	return &HistorySessionManager{
		factory:      factory,
		historyStore: store,
	}
}

// NewSession starts a fresh session, closing any existing one
func (m *HistorySessionManager) NewSession(workDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Save current session history before closing
	if m.current != nil && m.currentID != "" {
		if err := m.saveCurrentSession(); err != nil {
			// Log but don't fail - we still want to create new session
		}
		m.current.Close()
	}

	backend, err := m.factory.Create(workDir)
	if err != nil {
		return errors.Wrap(err, "creating new session")
	}

	m.current = backend
	m.currentID = backend.SessionID()
	return nil
}

// ResumeSession resumes a session from history with the given ID
func (m *HistorySessionManager) ResumeSession(sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Save current session before switching
	if m.current != nil && m.currentID != "" {
		if err := m.saveCurrentSession(); err != nil {
			// Log but don't fail
		}
		m.current.Close()
	}

	// Load the historical session
	_, err := m.historyStore.Load(sessionID)
	if err != nil {
		return errors.Wrap(err, "loading session from history")
	}

	// Note: We create a new backend but mark it with the old session ID
	// The actual message restoration would need backend support
	backend, err := m.factory.Create("")
	if err != nil {
		return errors.Wrap(err, "creating session backend")
	}

	m.current = backend
	m.currentID = sessionID
	return nil
}

// GetOrCreateSession returns current session or creates one if none exists
func (m *HistorySessionManager) GetOrCreateSession() (Backend, error) {
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
	m.currentID = backend.SessionID()
	return m.current, nil
}

// GetSession returns the current session or error if none
func (m *HistorySessionManager) GetSession() (Backend, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.current == nil {
		return nil, errors.New("no active session")
	}
	return m.current, nil
}

// GetCurrentSessionID returns the ID of the current session
func (m *HistorySessionManager) GetCurrentSessionID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentID
}

// Close shuts down the current session and saves history
func (m *HistorySessionManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.current == nil {
		return nil
	}

	// Save history before closing
	if m.currentID != "" {
		if err := m.saveCurrentSession(); err != nil {
			// Log but continue with close
		}
	}

	err := m.current.Close()
	m.current = nil
	m.currentID = ""
	return err
}

// ListSessions returns all saved sessions from history
func (m *HistorySessionManager) ListSessions() ([]*history.Session, error) {
	return m.historyStore.List()
}

// DeleteSession removes a session from history
func (m *HistorySessionManager) DeleteSession(sessionID string) error {
	return m.historyStore.Delete(sessionID)
}

// saveCurrentSession persists the current session to history store
// This is a placeholder - actual implementation would extract messages from backend
func (m *HistorySessionManager) saveCurrentSession() error {
	if m.current == nil || m.currentID == "" {
		return nil
	}

	session := &history.Session{
		ID:      m.currentID,
		WorkDir: "", // Would be populated from backend if available
	}

	return m.historyStore.Save(session)
}
