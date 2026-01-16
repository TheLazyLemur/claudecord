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

func TestExtractSessionID_FromSystemInit(t *testing.T) {
	a := assert.New(t)

	// given
	msg := `{"type":"system","subtype":"init","session_id":"my-session-123"}`

	// when
	id, err := extractSessionID([]byte(msg))

	// then
	a.NoError(err)
	a.Equal("my-session-123", id)
}

func TestExtractSessionID_NonInitMessage(t *testing.T) {
	a := assert.New(t)

	// given
	msg := `{"type":"assistant","message":{"role":"assistant"}}`

	// when
	id, err := extractSessionID([]byte(msg))

	// then
	a.NoError(err)
	a.Empty(id)
}

func TestBuildInitializeRequest(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// when
	msg := buildInitializeRequest("req-123")

	// then
	var parsed map[string]any
	err := json.Unmarshal(msg, &parsed)
	r.NoError(err)
	a.Equal("control_request", parsed["type"])
	a.Equal("req-123", parsed["request_id"])

	req := parsed["request"].(map[string]any)
	a.Equal("initialize", req["subtype"])
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
	proc, err := newProcessWithSpawner(spawner, "/tmp", "", 5*time.Second)

	// then
	r.NoError(err)
	a.Equal("new-session-abc", proc.SessionID())
	a.True(spawner.started)
}

func TestNewProcess_ResumeSession(t *testing.T) {
	a := assert.New(t)
	r := require.New(t)

	// given
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	var capturedArgs []string
	spawner := &MockProcessSpawnerWithArgs{
		stdinWriter:  stdinWriter,
		stdoutReader: stdoutReader,
		argsCapture:  &capturedArgs,
	}

	// simulate CLI responses
	go func() {
		reader := bufio.NewReader(stdinReader)
		reader.ReadString('\n')
		stdoutWriter.Write([]byte(`{"type":"control_response","response":{"subtype":"success"}}` + "\n"))
		stdoutWriter.Write([]byte(`{"type":"system","subtype":"init","session_id":"resumed-session"}` + "\n"))
	}()

	// when
	proc, err := newProcessWithSpawnerAndArgs(spawner, "/tmp", "existing-session-id", 5*time.Second)

	// then
	r.NoError(err)
	a.Equal("resumed-session", proc.SessionID())
	a.Contains(strings.Join(capturedArgs, " "), "--resume")
	a.Contains(strings.Join(capturedArgs, " "), "existing-session-id")
}

type MockProcessSpawnerWithArgs struct {
	stdinWriter  io.WriteCloser
	stdoutReader io.Reader
	argsCapture  *[]string
	started      bool
}

func (m *MockProcessSpawnerWithArgs) Start() error {
	m.started = true
	return nil
}

func (m *MockProcessSpawnerWithArgs) StdinPipe() (io.WriteCloser, error) {
	return m.stdinWriter, nil
}

func (m *MockProcessSpawnerWithArgs) StdoutPipe() (io.Reader, error) {
	return m.stdoutReader, nil
}

func (m *MockProcessSpawnerWithArgs) Kill() error {
	return nil
}

func (m *MockProcessSpawnerWithArgs) Wait() error {
	return nil
}

func (m *MockProcessSpawnerWithArgs) SetArgs(args []string) {
	*m.argsCapture = args
}

func TestProcess_ReceiveOnlyOnce(t *testing.T) {
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

	// then
	a.Error(err)
	a.Nil(ch2)
	a.Contains(err.Error(), "already receiving")

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
