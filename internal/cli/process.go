package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/pkg/errors"
)

var _ core.CLIProcess = (*Process)(nil)

// ProcessSpawner abstracts process creation for testing
type ProcessSpawner interface {
	Start() error
	StdinPipe() (io.WriteCloser, error)
	StdoutPipe() (io.Reader, error)
	Kill() error
	Wait() error
}

// RealProcessSpawner wraps exec.Cmd
type RealProcessSpawner struct {
	cmd *exec.Cmd
}

func NewRealProcessSpawner(workingDir, resumeSessionID string) *RealProcessSpawner {
	args := []string{
		"-p",
		"--verbose",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--permission-prompt-tool", "stdio",
	}
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}

	cmd := exec.Command("claude", args...)
	cmd.Dir = workingDir
	return &RealProcessSpawner{cmd: cmd}
}

func (r *RealProcessSpawner) Start() error {
	return r.cmd.Start()
}

func (r *RealProcessSpawner) StdinPipe() (io.WriteCloser, error) {
	return r.cmd.StdinPipe()
}

func (r *RealProcessSpawner) StdoutPipe() (io.Reader, error) {
	return r.cmd.StdoutPipe()
}

func (r *RealProcessSpawner) Kill() error {
	if r.cmd.Process != nil {
		return r.cmd.Process.Kill()
	}
	return nil
}

func (r *RealProcessSpawner) Wait() error {
	return r.cmd.Wait()
}

type Process struct {
	stdin       io.WriteCloser
	stdout      io.Reader
	stdoutClose io.Closer // optional, for closing pipe readers
	spawner     ProcessSpawner
	sessionID   string
	done        chan struct{}
	closeOnce   sync.Once
	receiving   bool
	mu          sync.Mutex
}

// NewProcess spawns the claude CLI and performs the initialize handshake.
func NewProcess(workingDir, resumeSessionID string, initTimeout time.Duration) (*Process, error) {
	spawner := NewRealProcessSpawner(workingDir, resumeSessionID)
	return newProcessWithSpawner(spawner, workingDir, resumeSessionID, initTimeout)
}

func newProcessWithSpawner(spawner ProcessSpawner, workingDir, resumeSessionID string, initTimeout time.Duration) (*Process, error) {
	stdin, err := spawner.StdinPipe()
	if err != nil {
		return nil, errors.Wrap(err, "getting stdin pipe")
	}

	stdout, err := spawner.StdoutPipe()
	if err != nil {
		return nil, errors.Wrap(err, "getting stdout pipe")
	}

	if err := spawner.Start(); err != nil {
		return nil, errors.Wrap(err, "starting claude process")
	}

	proc := &Process{
		stdin:   stdin,
		stdout:  stdout,
		spawner: spawner,
		done:    make(chan struct{}),
	}

	if err := proc.initialize(initTimeout); err != nil {
		spawner.Kill()
		return nil, errors.Wrap(err, "initializing session")
	}

	return proc, nil
}

func newProcessWithSpawnerAndArgs(spawner interface {
	ProcessSpawner
	SetArgs([]string)
}, workingDir, resumeSessionID string, initTimeout time.Duration) (*Process, error) {
	args := []string{
		"-p",
		"--verbose",
		"--input-format", "stream-json",
		"--output-format", "stream-json",
		"--permission-prompt-tool", "stdio",
	}
	if resumeSessionID != "" {
		args = append(args, "--resume", resumeSessionID)
	}
	spawner.SetArgs(args)

	return newProcessWithSpawner(spawner, workingDir, resumeSessionID, initTimeout)
}

func (p *Process) initialize(timeout time.Duration) error {
	// Send initialize request
	reqID := fmt.Sprintf("init-%d", time.Now().UnixNano())
	initMsg := buildInitializeRequest(reqID)
	if _, err := p.stdin.Write(append(initMsg, '\n')); err != nil {
		return errors.Wrap(err, "sending initialize request")
	}

	// Read responses until we get system.init with session_id
	scanner := bufio.NewScanner(p.stdout)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return errors.Wrap(err, "reading response")
			}
			return errors.New("unexpected EOF during initialization")
		}

		line := scanner.Bytes()
		sessionID, err := extractSessionID(line)
		if err != nil {
			return errors.Wrap(err, "parsing message")
		}
		if sessionID != "" {
			p.sessionID = sessionID
			// Replace stdout with a new reader that includes remaining buffered data
			p.stdout = &prefixReader{
				remaining: nil,
				reader:    p.stdout,
				scanner:   scanner,
			}
			return nil
		}
	}

	return errors.New("timeout waiting for session initialization")
}

// prefixReader allows continuing to read from scanner's buffer then underlying reader
type prefixReader struct {
	remaining []byte
	reader    io.Reader
	scanner   *bufio.Scanner
}

func (r *prefixReader) Read(p []byte) (int, error) {
	// The scanner may have buffered data, continue using it
	if r.scanner.Scan() {
		line := append(r.scanner.Bytes(), '\n')
		n := copy(p, line)
		if n < len(line) {
			r.remaining = line[n:]
		}
		return n, nil
	}
	if err := r.scanner.Err(); err != nil {
		return 0, err
	}
	return 0, io.EOF
}

func (p *Process) Send(msg []byte) error {
	select {
	case <-p.done:
		return errors.New("process closed")
	default:
	}

	_, err := p.stdin.Write(append(msg, '\n'))
	return errors.Wrap(err, "writing to stdin")
}

func (p *Process) Receive() (<-chan []byte, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.receiving {
		return nil, errors.New("already receiving")
	}
	p.receiving = true

	ch := make(chan []byte)
	go p.readLoop(ch)
	return ch, nil
}

func (p *Process) readLoop(ch chan<- []byte) {
	defer close(ch)
	scanner := bufio.NewScanner(p.stdout)

	for {
		select {
		case <-p.done:
			return
		default:
		}

		if !scanner.Scan() {
			return
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		msg := make([]byte, len(line))
		copy(msg, line)

		select {
		case ch <- msg:
		case <-p.done:
			return
		}
	}
}

func (p *Process) Close() error {
	var err error
	p.closeOnce.Do(func() {
		close(p.done)
		if p.stdin != nil {
			p.stdin.Close()
		}
		if p.stdoutClose != nil {
			p.stdoutClose.Close()
		}
		if p.spawner != nil {
			err = p.spawner.Kill()
			p.spawner.Wait()
		}
	})
	return err
}

func (p *Process) SessionID() string {
	return p.sessionID
}

func extractSessionID(msg []byte) (string, error) {
	var m struct {
		Type      string `json:"type"`
		Subtype   string `json:"subtype"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(msg, &m); err != nil {
		return "", err
	}
	if m.Type == "system" && m.Subtype == "init" {
		return m.SessionID, nil
	}
	return "", nil
}

func buildInitializeRequest(requestID string) []byte {
	req := map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request": map[string]any{
			"subtype": "initialize",
		},
	}
	data, _ := json.Marshal(req)
	return data
}

func buildUserMessage(content, sessionID string) []byte {
	msg := map[string]any{
		"type":       "user",
		"session_id": sessionID,
		"message": map[string]any{
			"role":    "user",
			"content": content,
		},
	}
	data, _ := json.Marshal(msg)
	return data
}
