package core

import (
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/history"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock Backend Factory for History Tests ---

type mockHistoryBackendFactory struct {
	backends     []*mockBackend
	counter      int
	createCalled bool
	lastWorkDir  string
	err          error
}

func (m *mockHistoryBackendFactory) Create(workDir string) (Backend, error) {
	m.createCalled = true
	m.lastWorkDir = workDir
	if m.err != nil {
		return nil, m.err
	}
	m.counter++
	backend := &mockBackend{sessionID: "session-" + string(rune('0'+m.counter))}
	m.backends = append(m.backends, backend)
	return backend, nil
}

// --- Tests ---

func TestHistorySessionManager_NewSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := history.NewStore(t.TempDir())
	r.NoError(err)
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)

	// when
	err = manager.NewSession("/tmp/work")

	// then
	r.NoError(err)
	a.Equal(1, len(factory.backends))
	a.Equal("session-1", manager.GetCurrentSessionID())
	a.Equal("/tmp/work", factory.lastWorkDir)
}

func TestHistorySessionManager_GetOrCreateSession_CreatesWhenNone(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := history.NewStore(t.TempDir())
	r.NoError(err)
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)

	// when
	backend, err := manager.GetOrCreateSession()

	// then
	r.NoError(err)
	a.NotNil(backend)
	a.Equal(1, len(factory.backends))
	a.True(factory.createCalled)
}

func TestHistorySessionManager_GetOrCreateSession_ReturnsExisting(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := history.NewStore(t.TempDir())
	r.NoError(err)
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)
	r.NoError(manager.NewSession(""))
	factory.createCalled = false

	// when
	backend, err := manager.GetOrCreateSession()

	// then
	r.NoError(err)
	a.NotNil(backend)
	a.Equal(1, len(factory.backends)) // Should not create new backend
	a.False(factory.createCalled)
}

func TestHistorySessionManager_GetSession_ErrorWhenNone(t *testing.T) {
	a := assert.New(t)

	// given
	store, _ := history.NewStore(t.TempDir())
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)

	// when
	_, err := manager.GetSession()

	// then
	a.Error(err)
	a.Contains(err.Error(), "no active session")
}

func TestHistorySessionManager_Close(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := history.NewStore(t.TempDir())
	r.NoError(err)
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)
	r.NoError(manager.NewSession(""))

	// when
	err = manager.Close()

	// then
	r.NoError(err)
	a.True(factory.backends[0].closed)
	a.Equal("", manager.GetCurrentSessionID())
}

func TestHistorySessionManager_ListSessions(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := history.NewStore(t.TempDir())
	r.NoError(err)
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)

	// Create a session and close it (which saves to history)
	r.NoError(manager.NewSession(""))
	r.NoError(manager.Close())

	// when
	sessions, err := manager.ListSessions()

	// then
	r.NoError(err)
	a.Len(sessions, 1)
}

func TestHistorySessionManager_DeleteSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := history.NewStore(t.TempDir())
	r.NoError(err)
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)

	r.NoError(manager.NewSession(""))
	sessionID := manager.GetCurrentSessionID()
	r.NoError(manager.Close())

	// Verify session exists
	sessions, _ := manager.ListSessions()
	r.Equal(1, len(sessions))

	// when
	err = manager.DeleteSession(sessionID)

	// then
	r.NoError(err)
	sessions, _ = manager.ListSessions()
	a.Len(sessions, 0)
}

func TestHistorySessionManager_ResumeSession_NotFound(t *testing.T) {
	a := assert.New(t)

	// given
	store, _ := history.NewStore(t.TempDir())
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)

	// when
	err := manager.ResumeSession("nonexistent")

	// then
	a.Error(err)
	a.Contains(err.Error(), "not found")
}

func TestHistorySessionManager_NewSession_ClosesPrevious(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	store, err := history.NewStore(t.TempDir())
	r.NoError(err)
	factory := &mockHistoryBackendFactory{}
	manager := NewHistorySessionManager(factory, store)

	r.NoError(manager.NewSession(""))
	firstBackend := factory.backends[0]

	// when
	r.NoError(manager.NewSession(""))

	// then
	a.True(firstBackend.closed)
	a.Equal(2, len(factory.backends))
}

func TestHistorySessionManager_NewSession_FactoryError(t *testing.T) {
	a := assert.New(t)

	// given
	store, _ := history.NewStore(t.TempDir())
	factory := &mockHistoryBackendFactory{err: errors.New("spawn failed")}
	manager := NewHistorySessionManager(factory, store)

	// when
	err := manager.NewSession("")

	// then
	a.Error(err)
	a.Contains(err.Error(), "spawn failed")
}
