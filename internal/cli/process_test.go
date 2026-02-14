package cli

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcess_SendWritesToStdin(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	proc := &Process{
		stdin:     stdinWriter,
		stdout:    stdoutReader,
		sessionID: "test-session",
		done:      make(chan struct{}),
	}

	// when
	msg := []byte(`{"type":"user","message":{"role":"user","content":"hello"}}`)
	sendErr := make(chan error, 1)
	go func() {
		sendErr <- proc.Send(msg)
	}()

	// then
	reader := bufio.NewReader(stdinReader)
	line, err := reader.ReadString('\n')
	r.NoError(err)
	a.Equal(string(msg)+"\n", line)
	r.NoError(<-sendErr)

	stdoutWriter.Close()
	stdinReader.Close()
}

func TestProcess_ReceiveReadsFromStdout(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	stdoutReader, stdoutWriter := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()

	proc := &Process{
		stdin:     stdinWriter,
		stdout:    stdoutReader,
		sessionID: "test-session",
		done:      make(chan struct{}),
	}

	// when
	ch, err := proc.Receive()
	r.NoError(err)

	// simulate CLI sending messages
	go func() {
		stdoutWriter.Write([]byte(`{"type":"assistant","message":{"role":"assistant"}}` + "\n"))
		stdoutWriter.Write([]byte(`{"type":"result","subtype":"success"}` + "\n"))
		stdoutWriter.Close()
	}()

	// then
	var received [][]byte
	for msg := range ch {
		received = append(received, msg)
	}
	a.Len(received, 2)
	a.Contains(string(received[0]), "assistant")
	a.Contains(string(received[1]), "result")

	stdinReader.Close()
}

func TestProcess_SessionIDReturnsExtractedID(t *testing.T) {
	a := assert.New(t)

	// given
	proc := &Process{
		sessionID: "abc-123-session",
		done:      make(chan struct{}),
	}

	// when
	id := proc.SessionID()

	// then
	a.Equal("abc-123-session", id)
}

func TestProcess_CloseStopsProcess(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	stdoutReader, stdoutWriter := io.Pipe()
	stdinReader, stdinWriter := io.Pipe()

	proc := &Process{
		stdin:       stdinWriter,
		stdout:      stdoutReader,
		stdoutClose: stdoutReader, // allows Close() to unblock scanner
		sessionID:   "test-session",
		done:        make(chan struct{}),
	}

	// when
	ch, err := proc.Receive()
	r.NoError(err)

	go func() {
		time.Sleep(10 * time.Millisecond)
		proc.Close()
	}()

	// then - channel should close
	var count int
	for range ch {
		count++
	}
	a.Equal(0, count)

	stdoutWriter.Close()
	stdinReader.Close()
}

func TestBuildInitializeRequest(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// when - empty system prompt uses default
	msg := buildInitializeRequest("req-123", "")

	// then
	var parsed ControlRequest
	err := json.Unmarshal(msg, &parsed)
	r.NoError(err)
	a.Equal("control_request", parsed.Type)
	a.Equal("req-123", parsed.RequestID)
	a.Equal("initialize", parsed.Request.Subtype)
}

func TestBuildInitializeRequest_CustomPrompt(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// when - custom system prompt
	msg := buildInitializeRequest("req-456", "custom prompt")

	// then
	var parsed ControlRequest
	err := json.Unmarshal(msg, &parsed)
	r.NoError(err)
	a.Equal("custom prompt", parsed.Request.SystemPrompt)
}

// ProcessSpawner interface for testing
type MockProcessSpawner struct {
	stdinWriter  io.WriteCloser
	stdoutReader io.Reader
	started      bool
	killed       bool
}

func (m *MockProcessSpawner) Start() error {
	m.started = true
	return nil
}

func (m *MockProcessSpawner) StdinPipe() (io.WriteCloser, error) {
	return m.stdinWriter, nil
}

func (m *MockProcessSpawner) StdoutPipe() (io.Reader, error) {
	return m.stdoutReader, nil
}

func (m *MockProcessSpawner) Kill() error {
	m.killed = true
	return nil
}

func (m *MockProcessSpawner) Wait() error {
	return nil
}

func TestNewProcess_InitializesSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	spawner := &MockProcessSpawner{
		stdinWriter:  stdinWriter,
		stdoutReader: stdoutReader,
	}

	// simulate CLI responses
	go func() {
		reader := bufio.NewReader(stdinReader)
		// read init request
		line, _ := reader.ReadString('\n')
		a.Contains(line, "initialize")

		// send control_response
		stdoutWriter.Write([]byte(`{"type":"control_response","response":{"subtype":"success"}}` + "\n"))
		// send system init with session_id
		stdoutWriter.Write([]byte(`{"type":"system","subtype":"init","session_id":"new-session-abc"}` + "\n"))
	}()

	// when
	proc, err := newProcessWithSpawner(spawner, 5*time.Second, "")

	// then
	r.NoError(err)
	a.Empty(proc.SessionID()) // session_id comes later with system.init, not during init
	a.True(spawner.started)
}

func TestNewRealProcessSpawner_ResumeSession(t *testing.T) {
	a := assert.New(t)

	// when - spawner is created with resume session ID
	spawner := NewRealProcessSpawner("/tmp", "existing-session-id")

	// then - command args should include --resume
	a.Contains(strings.Join(spawner.cmd.Args, " "), "--resume")
	a.Contains(strings.Join(spawner.cmd.Args, " "), "existing-session-id")
}

func TestNewRealProcessSpawner_SetsWorkingDirectory(t *testing.T) {
	a := assert.New(t)

	// when - spawner is created with a working directory
	spawner := NewRealProcessSpawner("/custom/working/dir", "")

	// then - cmd.Dir should be set to the working directory
	a.Equal("/custom/working/dir", spawner.cmd.Dir)
}

func TestProcess_ReceiveReturnsSameChannel(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	stdoutReader, stdoutWriter := io.Pipe()
	_, stdinWriter := io.Pipe()

	proc := &Process{
		stdin:     stdinWriter,
		stdout:    stdoutReader,
		sessionID: "test-session",
		done:      make(chan struct{}),
	}

	// when
	ch1, err := proc.Receive()
	r.NoError(err)
	a.NotNil(ch1)

	ch2, err := proc.Receive()

	// then - should return same channel, no error
	r.NoError(err)
	a.Equal(ch1, ch2)

	stdoutWriter.Close()
	proc.Close()
}

func TestProcess_SendAfterClose(t *testing.T) {
	a := assert.New(t)

	// given
	_, stdinWriter := io.Pipe()
	stdoutReader, _ := io.Pipe()

	proc := &Process{
		stdin:     stdinWriter,
		stdout:    stdoutReader,
		sessionID: "test-session",
		done:      make(chan struct{}),
	}

	proc.Close()

	// when
	err := proc.Send([]byte(`{"type":"user"}`))

	// then
	a.Error(err)
}
