package core

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCLIProcess struct {
	sessionID string
	closed    bool
}

func (m *mockCLIProcess) Send(msg []byte) error {
	if m.closed {
		return errors.New("process closed")
	}
	return nil
}

func (m *mockCLIProcess) Receive() (<-chan []byte, error) {
	ch := make(chan []byte)
	close(ch)
	return ch, nil
}

func (m *mockCLIProcess) Close() error {
	m.closed = true
	return nil
}

func (m *mockCLIProcess) SessionID() string {
	return m.sessionID
}

func TestSessionManager_NewSession_CreatesSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	factory := &mockProcessFactory{
		process: &mockCLIProcess{sessionID: "session-1"},
	}
	mgr := NewSessionManager(factory)

	// when
	err := mgr.NewSession("")

	// then
	r.NoError(err)
	a.True(factory.createCalled)
	a.Empty(factory.lastResumeID)
	sess, _ := mgr.GetSession()
	a.NotNil(sess)
	a.Equal("session-1", sess.SessionID())
}

func TestSessionManager_NewSession_PassesWorkDirToFactory(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	factory := &mockProcessFactory{
		process: &mockCLIProcess{sessionID: "session-1"},
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
	firstProc := &mockCLIProcess{sessionID: "session-1"}
	secondProc := &mockCLIProcess{sessionID: "session-2"}
	factory := &mockProcessFactory{process: firstProc}
	mgr := NewSessionManager(factory)
	r.NoError(mgr.NewSession("/first/dir"))
	a.Equal("/first/dir", factory.lastWorkDir)

	// prepare second process
	factory.process = secondProc

	// when - new session with different dir
	err := mgr.NewSession("/second/dir")

	// then
	r.NoError(err)
	a.True(firstProc.closed, "old session should be closed")
	a.Equal("/second/dir", factory.lastWorkDir, "new session should use new workDir")
	sess, _ := mgr.GetSession()
	a.Equal("session-2", sess.SessionID())
}

func TestSessionManager_NewSession_ClosesPreviousSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	firstProc := &mockCLIProcess{sessionID: "session-1"}
	secondProc := &mockCLIProcess{sessionID: "session-2"}
	factory := &mockProcessFactory{process: firstProc}
	mgr := NewSessionManager(factory)
	r.NoError(mgr.NewSession(""))

	factory.process = secondProc

	// when
	err := mgr.NewSession("")

	// then
	r.NoError(err)
	a.True(firstProc.closed)
	sess, _ := mgr.GetSession()
	a.Equal("session-2", sess.SessionID())
}

func TestSessionManager_GetOrCreateSession_CreatesIfNone(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	factory := &mockProcessFactory{
		process: &mockCLIProcess{sessionID: "auto-created"},
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
	factory := &mockProcessFactory{
		process: &mockCLIProcess{sessionID: "existing"},
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
	mgr := NewSessionManager(&mockProcessFactory{})

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
	proc := &mockCLIProcess{sessionID: "to-close"}
	factory := &mockProcessFactory{process: proc}
	mgr := NewSessionManager(factory)
	r.NoError(mgr.NewSession(""))

	// when
	err := mgr.Close()

	// then
	r.NoError(err)
	a.True(proc.closed)
	sess, _ := mgr.GetSession()
	a.Nil(sess)
}

func TestSessionManager_Close_NoopIfNoSession(t *testing.T) {
	// given
	mgr := NewSessionManager(&mockProcessFactory{})

	// when
	err := mgr.Close()

	// then
	assert.NoError(t, err)
}

func TestSessionManager_NewSession_FactoryError(t *testing.T) {
	a := assert.New(t)

	// given
	factory := &mockProcessFactory{err: errors.New("spawn failed")}
	mgr := NewSessionManager(factory)

	// when
	err := mgr.NewSession("")

	// then
	a.Error(err)
	a.Contains(err.Error(), "spawn failed")
}

type mockProcessFactory struct {
	process      *mockCLIProcess
	err          error
	createCalled bool
	lastResumeID string
	lastWorkDir  string
}

func (f *mockProcessFactory) Create(resumeSessionID, workDir string) (CLIProcess, error) {
	f.createCalled = true
	f.lastResumeID = resumeSessionID
	f.lastWorkDir = workDir
	if f.err != nil {
		return nil, f.err
	}
	return f.process, nil
}
