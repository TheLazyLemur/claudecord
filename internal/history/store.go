package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/pkg/errors"
)

// Message represents a single message in a conversation
type Message struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// Session represents a persisted conversation session
type Session struct {
	ID        string    `json:"id"`
	WorkDir   string    `json:"work_dir"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

// Summary returns a brief summary of the session for listing
func (s *Session) Summary() string {
	msgCount := len(s.Messages)
	lastActivity := s.UpdatedAt.Format("Jan 2 15:04")
	return fmt.Sprintf("%s: %d messages (last: %s)", s.ID[:8], msgCount, lastActivity)
}

// Store persists session history to disk
type Store struct {
	baseDir string
	mu      sync.RWMutex
}

// NewStore creates a new history store at the given base directory
func NewStore(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, errors.Wrap(err, "creating history directory")
	}
	return &Store{baseDir: baseDir}, nil
}

// Save persists a session to disk
func (s *Store) Save(session *Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	session.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return errors.Wrap(err, "marshaling session")
	}

	path := s.sessionPath(session.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return errors.Wrap(err, "creating session directory")
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return errors.Wrap(err, "writing session file")
	}

	return nil
}

// Load retrieves a session by ID
func (s *Store) Load(sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.sessionPath(sessionID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.Errorf("session %s not found", sessionID)
		}
		return nil, errors.Wrap(err, "reading session file")
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, errors.Wrap(err, "unmarshaling session")
	}

	return &session, nil
}

// List returns all sessions sorted by most recently updated
func (s *Store) List() ([]*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.baseDir)
	if err != nil {
		return nil, errors.Wrap(err, "reading history directory")
	}

	var sessions []*Session
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(s.baseDir, entry.Name()))
		if err != nil {
			continue // skip unreadable files
		}

		var session Session
		if err := json.Unmarshal(data, &session); err != nil {
			continue // skip invalid files
		}

		sessions = append(sessions, &session)
	}

	// Sort by most recently updated
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	return sessions, nil
}

// Delete removes a session from the store
func (s *Store) Delete(sessionID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.sessionPath(sessionID)
	if err := os.Remove(path); err != nil {
		return errors.Wrap(err, "deleting session file")
	}

	return nil
}

// sessionPath returns the file path for a session
func (s *Store) sessionPath(sessionID string) string {
	return filepath.Join(s.baseDir, sessionID+".json")
}
