package core

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockBackend struct {
	sessionID string
	closed    bool
}

func (m *mockBackend) Converse(ctx context.Context, msg string, responder Responder, perms PermissionChecker) (string, error) {
	return "", nil
}

func (m *mockBackend) Close() error {
	m.closed = true
	return nil
}

func (m *mockBackend) SessionID() string {
	return m.sessionID
}

func TestSessionManager_NewSession_CreatesSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	factory := &mockBackendFactory{
		backend: &mockBackend{sessionID: "session-1"},
	}
	mgr := NewSessionManager(factory)

	// when
	err := mgr.NewSession("")

	// then
	r.NoError(err)
	a.True(factory.createCalled)
	sess, _ := mgr.GetSession()
	a.NotNil(sess)
	a.Equal("session-1", sess.SessionID())
}

func TestSessionManager_NewSession_PassesWorkDirToFactory(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	factory := &mockBackendFactory{
		backend: &mockBackend{sessionID: "session-1"},
	}
	mgr := NewSessionManager(factory)

	// when
	err := mgr.NewSession("/custom/work/dir")

	// then
	r.NoError(err)
	a.Equal("/custom/work/dir", factory.lastWorkDir)
}

func TestSessionManager_NewSession_ClosesOldAndCreatesNewWithDifferentWorkDir(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given - first session with /first/dir
	firstBackend := &mockBackend{sessionID: "session-1"}
	secondBackend := &mockBackend{sessionID: "session-2"}
	factory := &mockBackendFactory{backend: firstBackend}
	mgr := NewSessionManager(factory)
	r.NoError(mgr.NewSession("/first/dir"))
	a.Equal("/first/dir", factory.lastWorkDir)

	// prepare second backend
	factory.backend = secondBackend

	// when - new session with different dir
	err := mgr.NewSession("/second/dir")

	// then
	r.NoError(err)
	a.True(firstBackend.closed, "old session should be closed")
	a.Equal("/second/dir", factory.lastWorkDir, "new session should use new workDir")
	sess, _ := mgr.GetSession()
	a.Equal("session-2", sess.SessionID())
}

func TestSessionManager_NewSession_ClosesPreviousSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	firstBackend := &mockBackend{sessionID: "session-1"}
	secondBackend := &mockBackend{sessionID: "session-2"}
	factory := &mockBackendFactory{backend: firstBackend}
	mgr := NewSessionManager(factory)
	r.NoError(mgr.NewSession(""))

	factory.backend = secondBackend

	// when
	err := mgr.NewSession("")

	// then
	r.NoError(err)
	a.True(firstBackend.closed)
	sess, _ := mgr.GetSession()
	a.Equal("session-2", sess.SessionID())
}

func TestSessionManager_GetOrCreateSession_CreatesIfNone(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	factory := &mockBackendFactory{
		backend: &mockBackend{sessionID: "auto-created"},
	}
	mgr := NewSessionManager(factory)

	// when
	sess, err := mgr.GetOrCreateSession()

	// then
	r.NoError(err)
	a.NotNil(sess)
	a.Equal("auto-created", sess.SessionID())
	a.True(factory.createCalled)
}

func TestSessionManager_GetOrCreateSession_ReturnsExisting(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	factory := &mockBackendFactory{
		backend: &mockBackend{sessionID: "existing"},
	}
	mgr := NewSessionManager(factory)
	r.NoError(mgr.NewSession(""))
	factory.createCalled = false

	// when
	sess, err := mgr.GetOrCreateSession()

	// then
	r.NoError(err)
	a.Equal("existing", sess.SessionID())
	a.False(factory.createCalled)
}

func TestSessionManager_GetSession_ReturnsNilIfNone(t *testing.T) {
	a := assert.New(t)

	// given
	mgr := NewSessionManager(&mockBackendFactory{})

	// when
	sess, err := mgr.GetSession()

	// then
	a.Nil(sess)
	a.Error(err)
}

func TestSessionManager_Close_ClosesCurrentSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	backend := &mockBackend{sessionID: "to-close"}
	factory := &mockBackendFactory{backend: backend}
	mgr := NewSessionManager(factory)
	r.NoError(mgr.NewSession(""))

	// when
	err := mgr.Close()

	// then
	r.NoError(err)
	a.True(backend.closed)
	sess, _ := mgr.GetSession()
	a.Nil(sess)
}

func TestSessionManager_Close_NoopIfNoSession(t *testing.T) {
	// given
	mgr := NewSessionManager(&mockBackendFactory{})

	// when
	err := mgr.Close()

	// then
	assert.NoError(t, err)
}

func TestSessionManager_NewSession_FactoryError(t *testing.T) {
	a := assert.New(t)

	// given
	factory := &mockBackendFactory{err: errors.New("spawn failed")}
	mgr := NewSessionManager(factory)

	// when
	err := mgr.NewSession("")

	// then
	a.Error(err)
	a.Contains(err.Error(), "spawn failed")
}

type mockBackendFactory struct {
	backend      *mockBackend
	err          error
	createCalled bool
	lastWorkDir  string
}

func (f *mockBackendFactory) Create(workDir string) (Backend, error) {
	f.createCalled = true
	f.lastWorkDir = workDir
	if f.err != nil {
		return nil, f.err
	}
	return f.backend, nil
}
