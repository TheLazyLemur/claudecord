# Claudecord Self-Improvement Plan

This document outlines a comprehensive plan to improve the existing Claudecord codebase without adding new features. The focus is on making current features more robust, maintainable, and efficient.

## Executive Summary

After reviewing the entire codebase, I've identified 8 key improvement areas:
1. **Error Handling & Resilience** - Better error recovery and user feedback
2. **Session Management** - More robust session lifecycle handling
3. **Message Processing** - Improved buffering, chunking, and delivery
4. **Permission System** - More granular and safer permission checks
5. **Code Quality** - Reduce duplication, improve testability
6. **Resource Management** - Better cleanup and resource lifecycle
7. **Observability** - Better logging and monitoring hooks
8. **Configuration** - Validation and safer defaults

---

## 1. Error Handling & Resilience

### Current Issues
- CLI process crashes can leave sessions in broken state
- Discord API errors (rate limits, network issues) aren't gracefully handled
- Tool execution errors don't provide actionable feedback to users
- No retry logic for transient failures

### Improvements

#### 1.1 CLI Process Health Monitoring
```go
// Add to cli/process.go
type ProcessHealth struct {
    lastPing time.Time
    mu       sync.RWMutex
}

func (p *Process) StartHealthCheck(interval time.Duration) {
    // Periodically check if process is responsive
    // Auto-restart on failure with backoff
}
```

#### 1.2 Discord API Resilience
```go
// Add to handler/discord.go
type ResilientDiscordClient struct {
    client DiscordSession
    retryPolicy RetryPolicy
}

func (c *ResilientDiscordClient) SendMessageWithRetry(channelID, content string) error {
    // Exponential backoff for rate limits
    // Circuit breaker pattern for repeated failures
}
```

#### 1.3 Tool Error Context
```go
// Enhance tool execution to return structured errors
type ToolError struct {
    ToolName string
    Input    map[string]any
    Cause    error
    Suggestion string // actionable suggestion for user
}
```

---

## 2. Session Management

### Current Issues
- Sessions can leak if Close() isn't called
- No session state visibility for users
- Passive and active sessions aren't coordinated
- Session recovery after crash not possible

### Improvements

#### 2.1 Session State Tracking
```go
// Add to core/session.go
type SessionState struct {
    ID        string
    CreatedAt time.Time
    LastActivity time.Time
    MessageCount int
    Status    SessionStatus // active, idle, error
}

type SessionManager struct {
    // ... existing fields
    stateMu sync.RWMutex
    state   SessionState
}
```

#### 2.2 Graceful Session Cleanup
```go
// Ensure all goroutines are stopped, channels closed
func (m *SessionManager) Close() error {
    // Use sync.WaitGroup to track in-flight operations
    // Context cancellation for graceful shutdown
}
```

#### 2.3 Session Coordination
```go
// Ensure passive and active sessions share state where appropriate
type SessionCoordinator struct {
    active  *SessionManager
    passive *SessionManager
}
```

---

## 3. Message Processing

### Current Issues
- Long messages chunked naively (may split words/URLs)
- No message delivery confirmation
- Passive buffer doesn't handle rapid message bursts well
- Thread creation can race with message delivery

### Improvements

#### 3.1 Smart Message Chunking
```go
// Add to core/responder.go
func SmartChunk(content string, maxLen int) []string {
    // Split at word boundaries
    // Preserve code blocks (don't split mid-block)
    // Handle URLs specially (don't break them)
}
```

#### 3.2 Message Delivery Tracking
```go
// Track which chunks were delivered successfully
type MessageDelivery struct {
    MessageID string
    Chunks    []ChunkStatus
}
```

#### 3.3 Improved Passive Buffer
```go
// Add burst detection and adaptive debouncing
func (d *DebouncedBuffer) Add(msg BufferedMessage) {
    // If messages arriving rapidly, extend debounce window
    // If channel is quiet, trigger immediately
}
```

---

## 4. Permission System

### Current Issues
- Path traversal check can be bypassed with symlinks
- No audit logging of permission decisions
- Binary allow/deny - no "allow once" or time-limited permissions
- Permission prompts can be missed in busy channels

### Improvements

#### 4.1 Symlink Resolution
```go
// In cli/permission.go
func (p *PermissionChecker) isAllowed(path string) bool {
    // Resolve symlinks before checking
    resolved, err := filepath.EvalSymlinks(path)
    if err != nil {
        return false // deny on error
    }
    // ... existing check
}
```

#### 4.2 Permission Audit Log
```go
type PermissionDecision struct {
    Timestamp time.Time
    ToolName  string
    Input     map[string]any
    Allowed   bool
    Reason    string
    UserID    string // if user-approved
}
```

#### 4.3 Ephemeral Permissions
```go
// Allow "approve for this session" or "approve for 5 minutes"
type EphemeralPermission struct {
    ToolName  string
    Pattern   string // e.g., path prefix
    ExpiresAt time.Time
}
```

---

## 5. Code Quality

### Current Issues
- **Tool execution logic is duplicated** between CLI (`internal/cli/backend.go:271-328`) and API (`internal/api/executors.go:14-76`) backends
- String constants scattered throughout code
- Some functions are too long (processResponses)
- Interface definitions mixed with implementations

### Improvements

#### 5.1 Unified Tool Execution (Confirmed Duplication)
```go
// Tool definitions (ToolDef) are correctly shared via core/tools.go
// BUT execution logic is duplicated in:
// - cli/backend.go handleMCPToolCall() 
// - api/executors.go executeToolByName()
// 
// Solution: Create internal/tools package
type Executor interface {
    Execute(ctx context.Context, input map[string]any, responder core.Responder, store skills.SkillStore) (string, bool)
}

var Registry = map[string]Executor{
    "react_emoji":      &ReactEmojiExecutor{},
    "send_update":      &SendUpdateExecutor{},
    "Skill":            &SkillExecutor{},
    "LoadSkillSupporting": &LoadSkillSupportingExecutor{},
    "Read":             &ReadExecutor{},
    "Bash":             &BashExecutor{},
}
```

#### 5.2 Extract Constants
```go
// internal/constants/constants.go
const (
    MaxDiscordMessageLen = 2000
    DefaultDebounceDelay = 30 * time.Second
    MaxFileReadSize      = 50000
    // ...
)
```

#### 5.3 Split Large Functions
```go
// cli/backend.go processResponses is too long
// Split into:
// - handleAssistantMessage
// - handleControlRequest  
// - handleToolUse
// - handleResult
```

#### 5.4 Separate Interfaces
```go
// internal/core/interfaces.go is good but could be better organized
// Group by domain:
// - Messaging interfaces
// - Session interfaces
// - Permission interfaces
```

---

## 6. Resource Management

### Current Issues
- CLI process may not be killed on panic
- Goroutine leaks in readLoop
- No resource quotas (could read giant files)
- HTTP server shutdown doesn't wait for requests

### Improvements

#### 6.1 Guaranteed Cleanup
```go
// Use defer and finalizers where appropriate
func NewProcess(...) (*Process, error) {
    p := &Process{}
    runtime.SetFinalizer(p, (*Process).cleanup)
    // ...
}
```

#### 6.2 Goroutine Lifecycle
```go
// Track goroutines with named contexts
type Process struct {
    readCtx    context.Context
    readCancel context.CancelFunc
    readWg     sync.WaitGroup
}
```

#### 6.3 Resource Quotas
```go
// Limit file sizes, command execution time
type Quotas struct {
    MaxFileSize      int64
    MaxCommandTime   time.Duration
    MaxOutputSize    int
}
```

#### 6.4 Graceful HTTP Shutdown
```go
// Already partially implemented, but ensure:
// - Drain active connections
// - Reject new connections with 503
// - Timeout for stubborn connections
```

---

## 7. Observability

### Current Issues
- Logging is inconsistent (some use slog, some don't)
- No metrics or health endpoints
- Hard to debug session issues in production
- No structured logging of key events

### Improvements

#### 7.1 Structured Logging
```go
// Consistent logging pattern
slog.Info("session_created",
    "session_id", sessionID,
    "user_id", userID,
    "work_dir", workDir,
)
```

#### 7.2 Health Endpoint
```go
// Add to handler/webhook.go
func (h *WebhookHandler) Health(w http.ResponseWriter, r *http.Request) {
    // Return:
    // - Discord connection status
    // - Active session count
    // - Last error timestamp
}
```

#### 7.3 Session Tracing
```go
// Add trace IDs to follow a request through the system
type ContextKey string
const TraceIDKey ContextKey = "trace_id"

func WithTraceID(ctx context.Context) context.Context {
    return context.WithValue(ctx, TraceIDKey, generateTraceID())
}
```

---

## 8. Configuration

### Current Issues
- Config validation happens at load time only
- No validation that allowed dirs exist
- Sensitive values (API keys) logged
- No config hot-reload

### Improvements

#### 8.1 Enhanced Validation
```go
func (c *Config) Validate() error {
    // Check allowed dirs exist and are readable
    // Validate Discord token format
    // Check API connectivity if in API mode
}
```

#### 8.2 Safe Logging
```go
func (c *Config) SafeString() string {
    // Return config with secrets redacted
    // Useful for debugging
}
```

#### 8.3 Config Documentation
```go
// Add comments to Config struct for auto-generated docs
type Config struct {
    // DiscordToken is the bot token from Discord Developer Portal
    DiscordToken string
    
    // AllowedDirs are paths the bot can read/write
    // Applied recursively
    AllowedDirs []string
}
```

---

## Implementation Priority

### Phase 1: Critical Stability (Week 1)
1. Error handling improvements (1.1, 1.2)
2. Session cleanup fixes (2.2)
3. Resource quotas (6.3)

### Phase 2: Reliability (Week 2)
4. Smart message chunking (3.1)
5. Permission audit logging (4.2)
6. Symlink resolution (4.1)

### Phase 3: Maintainability (Week 3)
7. **Unified tool execution (5.1)** - Fix confirmed duplication
8. Extract constants (5.2)
9. Structured logging (7.1)

### Phase 4: Polish (Week 4)
10. Health endpoints (7.2)
11. Config validation (8.1)
12. Code organization (5.3, 5.4)

---

## Success Metrics

- **Crash Recovery**: Bot should auto-recover from 95% of CLI process crashes
- **Message Delivery**: 99.9% of messages delivered without truncation errors
- **Permission Accuracy**: Zero path traversal vulnerabilities
- **Code Coverage**: Increase from current ~60% to 80%+
- **Mean Time to Debug**: Reduce production issue diagnosis time by 50%

---

## Notes

This plan focuses exclusively on improving existing functionality. Each improvement:
- Maintains backward compatibility
- Includes comprehensive tests
- Documents behavior changes
- Can be rolled back independently

The goal is to make Claudecord more robust, maintainable, and trustworthy without changing its fundamental behavior.

### Correction Note
Tool **definitions** (`ToolDef` structs in `internal/core/tools.go`) are correctly shared between CLI and API backends via `core.DiscordTools()`, `core.FileTools()`, and `core.SkillTools()`. 

However, the **tool execution logic** IS duplicated:
- `internal/cli/backend.go:271-328` (`handleMCPToolCall`)
- `internal/api/executors.go:14-76` (`executeToolByName`)

Both implement identical switch statements for `react_emoji`, `send_update`, `Skill`, and `LoadSkillSupporting`.
