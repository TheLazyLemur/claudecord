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
	mgr := NewSessionManager(factory, nil)

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
	mgr := NewSessionManager(factory, nil)

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
	mgr := NewSessionManager(factory, nil)
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
	mgr := NewSessionManager(factory, nil)
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
	mgr := NewSessionManager(factory, nil)

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
	mgr := NewSessionManager(factory, nil)
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
	mgr := NewSessionManager(&mockBackendFactory{}, nil)

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
	mgr := NewSessionManager(factory, nil)
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
	mgr := NewSessionManager(&mockBackendFactory{}, nil)

	// when
	err := mgr.Close()

	// then
	assert.NoError(t, err)
}

func TestSessionManager_CurrentSessionID_EmptyWhenNoSession(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a session manager with no active session
	mgr := NewSessionManager(&mockBackendFactory{}, nil)

	// when
	// ... CurrentSessionID is called
	id := mgr.CurrentSessionID()

	// then
	// ... it returns the empty string
	a.Equal("", id)
}

func TestSessionManager_CurrentSessionID_ReturnsActiveID(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a session manager with an active session
	factory := &mockBackendFactory{backend: &mockBackend{sessionID: "active"}}
	mgr := NewSessionManager(factory, nil)
	r.NoError(mgr.NewSession(""))

	// when
	// ... CurrentSessionID is called
	id := mgr.CurrentSessionID()

	// then
	// ... it returns the current backend's session id
	a.Equal("active", id)
}

func TestSessionManager_CurrentSessionID_ReflectsNewSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a session manager that has been swapped to a new session
	first := &mockBackend{sessionID: "first"}
	second := &mockBackend{sessionID: "second"}
	factory := &mockBackendFactory{backend: first}
	mgr := NewSessionManager(factory, nil)
	r.NoError(mgr.NewSession(""))
	factory.backend = second
	r.NoError(mgr.NewSession(""))

	// when
	// ... CurrentSessionID is called
	id := mgr.CurrentSessionID()

	// then
	// ... it returns the new backend's session id
	a.Equal("second", id)
}

func TestSessionManager_NewSession_FactoryError(t *testing.T) {
	a := assert.New(t)

	// given
	factory := &mockBackendFactory{err: errors.New("spawn failed")}
	mgr := NewSessionManager(factory, nil)

	// when
	err := mgr.NewSession("")

	// then
	a.Error(err)
	a.Contains(err.Error(), "spawn failed")
}

func TestSessionManager_NewSession_RunsFlushBeforeClose(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a session manager with a flush func and a current backend
	firstBackend := &mockBackend{sessionID: "session-1"}
	secondBackend := &mockBackend{sessionID: "session-2"}
	factory := &mockBackendFactory{backend: firstBackend}

	var flushedBackend Backend
	var flushedBeforeClose bool
	flushFn := func(ctx context.Context, current Backend) {
		flushedBackend = current
		// closure captures the firstBackend pointer; check closed flag at this moment
		if mb, ok := current.(*mockBackend); ok {
			flushedBeforeClose = !mb.closed
		}
	}

	mgr := NewSessionManager(factory, flushFn)
	r.NoError(mgr.NewSession(""))
	factory.backend = secondBackend

	// when
	// ... a new session is started
	r.NoError(mgr.NewSession(""))

	// then
	// ... the flush func was called against the previous backend before it was closed
	a.Same(firstBackend, flushedBackend)
	a.True(flushedBeforeClose, "flush should run before the old backend is closed")
	a.True(firstBackend.closed)
}

func TestSessionManager_NewSession_NoFlushOnFirstSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a session manager with a flush func and no current backend
	factory := &mockBackendFactory{backend: &mockBackend{sessionID: "first"}}
	flushCalled := false
	flushFn := func(ctx context.Context, current Backend) {
		flushCalled = true
	}
	mgr := NewSessionManager(factory, flushFn)

	// when
	// ... NewSession is called for the very first time
	r.NoError(mgr.NewSession(""))

	// then
	// ... the flush func was not called
	a.False(flushCalled, "flush should not run on first session")
}

func TestSessionManager_NewSession_NilFlushIsNoop(t *testing.T) {
	r := require.New(t)

	// given
	// ... a session manager constructed without a flush func
	first := &mockBackend{sessionID: "first"}
	second := &mockBackend{sessionID: "second"}
	factory := &mockBackendFactory{backend: first}
	mgr := NewSessionManager(factory, nil)
	r.NoError(mgr.NewSession(""))
	factory.backend = second

	// when
	// ... NewSession is called again with no flush func
	err := mgr.NewSession("")

	// then
	// ... no panic, behaves like the unflushed manager
	r.NoError(err)
}

func TestSessionManager_NewSession_FlushPanicDoesNotBlock(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	// ... a flush func that panics
	first := &mockBackend{sessionID: "first"}
	second := &mockBackend{sessionID: "second"}
	factory := &mockBackendFactory{backend: first}
	flushFn := func(ctx context.Context, current Backend) {
		panic("boom")
	}
	mgr := NewSessionManager(factory, flushFn)
	r.NoError(mgr.NewSession(""))
	factory.backend = second

	// when
	// ... NewSession is called and the flush func panics
	err := mgr.NewSession("")

	// then
	// ... the panic is recovered and the new session is created
	r.NoError(err)
	a.True(first.closed)
	sess, _ := mgr.GetSession()
	a.Equal("second", sess.SessionID())
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
