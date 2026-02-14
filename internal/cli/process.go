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

// CLIProcess is the low-level interface for CLI communication
type CLIProcess interface {
	Send(msg []byte) error
	Receive() (<-chan []byte, error)
	Close() error
	SessionID() string
}

var _ CLIProcess = (*Process)(nil)

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
	return newProcessWithSpawner(spawner, initTimeout, "")
}

// NewProcessWithSystemPrompt spawns the claude CLI with a custom system prompt.
func NewProcessWithSystemPrompt(workingDir, resumeSessionID string, initTimeout time.Duration, systemPrompt string) (*Process, error) {
	spawner := NewRealProcessSpawner(workingDir, resumeSessionID)
	return newProcessWithSpawner(spawner, initTimeout, systemPrompt)
}

func newProcessWithSpawner(spawner ProcessSpawner, initTimeout time.Duration, systemPrompt string) (*Process, error) {
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

	if err := proc.initialize(initTimeout, systemPrompt); err != nil {
		spawner.Kill()
		return nil, errors.Wrap(err, "initializing session")
	}

	return proc, nil
}

func (p *Process) initialize(timeout time.Duration, systemPrompt string) error {
	slog.Info("CLI initialize: sending init request")
	reqID := fmt.Sprintf("init-%d", time.Now().UnixNano())
	initMsg := buildInitializeRequest(reqID, systemPrompt)
	if _, err := p.stdin.Write(append(initMsg, '\n')); err != nil {
		return errors.Wrap(err, "sending initialize request")
	}
	slog.Info("CLI initialize: sent, waiting for control_response")

	scanner := bufio.NewScanner(p.stdout)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
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

		var msg CLIMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}

		// Handle MCP setup messages during init
		if msg.Type == "control_request" {
			if err := p.handleInitMCP(msg); err != nil {
				slog.Error("MCP init error", "error", err)
			}
			continue
		}

		// If we get control_response for our init, we're ready
		if msg.Type == "control_response" {
			slog.Info("CLI initialize: got control_response, ready")
			p.stdout = &prefixReader{scanner: scanner}
			return nil
		}

		// Also check for system.init which has session_id
		if msg.Type == "system" {
			if msg.Subtype == "init" {
				if msg.SessionID != "" {
					slog.Info("CLI initialize: got session", "sessionID", msg.SessionID)
					p.sessionID = msg.SessionID
					p.stdout = &prefixReader{scanner: scanner}
					return nil
				}
			}
		}
	}

	return errors.New("timeout waiting for session initialization")
}

// handleInitMCP handles MCP setup messages during initialization
func (p *Process) handleInitMCP(msg CLIMessage) error {
	var req MCPMessageRequest
	if err := json.Unmarshal(msg.Request, &req); err != nil {
		return nil
	}

	if req.Subtype != "mcp_message" {
		return nil
	}

	slog.Info("MCP init", "method", req.Message.Method)

	var result any
	switch req.Message.Method {
	case "initialize":
		result = MCPInitResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    MCPCapability{Tools: map[string]any{}},
			ServerInfo:      MCPServerInfo{Name: "discord-tools", Version: "1.0.0"},
		}
	case "notifications/initialized":
		result = struct{}{}
	case "tools/list":
		result = MCPToolListResult{Tools: core.MCPTools}
	default:
		result = struct{}{}
	}

	resp := MCPResponseWrapper{
		Type: "control_response",
		Response: MCPResponseWrapperInner{
			Subtype:   "success",
			RequestID: msg.RequestID,
			Response: MCPResponse{
				MCPResponse: MCPJSONRPCResponse{
					JSONRPC: "2.0",
					ID:      req.Message.ID,
					Result:  result,
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
	if len(r.remaining) > 0 {
		n := copy(p, r.remaining)
		r.remaining = r.remaining[n:]
		return n, nil
	}

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
		p.recvChan = make(chan []byte, 100)
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

func buildInitializeRequest(requestID, systemPrompt string) []byte {
	prompt := systemPrompt
	if prompt == "" {
		prompt = "When you receive a message, first call react_emoji with '👀' to acknowledge. For longer tasks, use send_update to post progress updates to a thread."
	}
	req := ControlRequest{
		Type:      "control_request",
		RequestID: requestID,
		Request: ControlRequestInner{
			Subtype:       "initialize",
			SystemPrompt:  prompt,
			SDKMcpServers: []string{"discord-tools"},
		},
	}
	data, _ := json.Marshal(req)
	return data
}
