package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
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
	recvChan    chan []byte // persistent receive channel
	recvOnce    sync.Once
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
	slog.Info("CLI initialize: sending init request")
	// Send initialize request
	reqID := fmt.Sprintf("init-%d", time.Now().UnixNano())
	initMsg := buildInitializeRequest(reqID)
	if _, err := p.stdin.Write(append(initMsg, '\n')); err != nil {
		return errors.Wrap(err, "sending initialize request")
	}
	slog.Info("CLI initialize: sent, waiting for control_response")

	// Read until we get control_response or system.init with session_id
	scanner := bufio.NewScanner(p.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024) // 1MB buffer for large responses
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return errors.Wrap(err, "reading response")
			}
			return errors.New("unexpected EOF during initialization")
		}

		line := scanner.Bytes()
		slog.Info("CLI initialize: got line", "len", len(line))

		// Check if it's control_response (init ack) or system.init
		var msg map[string]any
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		msgType, _ := msg["type"].(string)

		// Handle MCP setup messages during init
		if msgType == "control_request" {
			if err := p.handleInitMCP(msg); err != nil {
				slog.Error("MCP init error", "error", err)
			}
			continue
		}

		// If we get control_response for our init, we're ready
		if msgType == "control_response" {
			slog.Info("CLI initialize: got control_response, ready")
			p.stdout = &prefixReader{scanner: scanner}
			return nil
		}

		// Also check for system.init which has session_id
		if msgType == "system" {
			if subtype, _ := msg["subtype"].(string); subtype == "init" {
				if sid, _ := msg["session_id"].(string); sid != "" {
					slog.Info("CLI initialize: got session", "sessionID", sid)
					p.sessionID = sid
					p.stdout = &prefixReader{scanner: scanner}
					return nil
				}
			}
		}
	}

	return errors.New("timeout waiting for session initialization")
}

// handleInitMCP handles MCP setup messages during initialization
func (p *Process) handleInitMCP(msg map[string]any) error {
	requestID, _ := msg["request_id"].(string)
	request, _ := msg["request"].(map[string]any)
	subtype, _ := request["subtype"].(string)

	if subtype != "mcp_message" {
		return nil
	}

	message, _ := request["message"].(map[string]any)
	method, _ := message["method"].(string)
	jsonrpcID := message["id"]

	slog.Info("MCP init", "method", method)

	var result any
	switch method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "discord-tools", "version": "1.0.0"},
		}
	case "notifications/initialized":
		result = map[string]any{}
	case "tools/list":
		result = map[string]any{"tools": core.MCPTools}
	default:
		result = map[string]any{}
	}

	resp := map[string]any{
		"type": "control_response",
		"response": map[string]any{
			"subtype":    "success",
			"request_id": requestID,
			"response": map[string]any{
				"mcp_response": map[string]any{
					"jsonrpc": "2.0",
					"id":      jsonrpcID,
					"result":  result,
				},
			},
		},
	}
	data, _ := json.Marshal(resp)
	_, err := p.stdin.Write(append(data, '\n'))
	return err
}

// prefixReader allows continuing to read from scanner's buffer then underlying reader
type prefixReader struct {
	remaining []byte
	scanner   *bufio.Scanner
}

func (r *prefixReader) Read(p []byte) (int, error) {
	// Drain remaining from previous partial read first
	if len(r.remaining) > 0 {
		n := copy(p, r.remaining)
		r.remaining = r.remaining[n:]
		return n, nil
	}

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
	p.recvOnce.Do(func() {
		p.recvChan = make(chan []byte, 100) // buffered to avoid blocking
		go p.readLoop(p.recvChan)
	})
	return p.recvChan, nil
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

func buildInitializeRequest(requestID string) []byte {
	req := map[string]any{
		"type":       "control_request",
		"request_id": requestID,
		"request": map[string]any{
			"subtype":       "initialize",
			"systemPrompt":  "When you receive a message, first call react_emoji with '👀' to acknowledge. For longer tasks, use send_update to post progress updates to a thread.",
			"sdkMcpServers": []string{"discord-tools"},
		},
	}
	data, _ := json.Marshal(req)
	return data
}

