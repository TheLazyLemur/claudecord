# Message Queue Implementation Plan

## Overview

Implement a message queue system for API mode that allows messages to be queued and processed sequentially per channel. This enables better handling of concurrent messages and lays the groundwork for future steering capabilities.

## Goals

1. **Non-blocking message handling**: Messages are queued immediately without blocking the Discord handler
2. **Per-channel isolation**: Each channel has its own queue, allowing parallel processing across channels
3. **Sequential processing within channel**: Messages in the same channel are processed in order to maintain conversation context
4. **Clean architecture**: Queue system is decoupled from Discord handler and Bot logic

## Out of Scope (Future Considerations)

- **Steering**: Real-time message injection during active conversations is NOT included in this plan
- **Queue persistence**: Messages are not persisted to disk (in-memory only)
- **Priority messages**: All messages are FIFO
- **Backpressure handling**: Simple buffered channels (no complex flow control)

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│  Discord Handler │────▶│  Message Queue   │────▶│  Worker Pool    │
│  (producers)     │     │  (per channel)   │     │  (consumers)    │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                              │                           │
                              ▼                           ▼
                        ┌─────────────┐            ┌─────────────┐
                        │  Channel A  │◄───────────│  Worker 1   │
                        │  Channel B  │◄───────────│  Worker 2   │
                        │  Channel C  │◄───────────│  Worker N   │
                        └─────────────┘            └─────────────┘
```

### Key Components

1. **MessageQueue**: Global queue manager that handles queues for all channels
2. **ChannelQueue**: Per-channel queue with buffered channel and worker goroutine
3. **QueuedMessage**: Message structure containing all necessary context
4. **Modified Bot**: Removed mutex lock, processes messages from queue
5. **Modified Handler**: Routes messages to queue instead of direct handling

## Implementation Details

### 1. New Files

#### `internal/core/queue.go`

```go
package core

import (
    "context"
    "sync"
    "time"
)

// QueuedMessage represents a message waiting to be processed
type QueuedMessage struct {
    ID        string
    ChannelID string
    UserID    string
    Content   string
    Responder Responder
    Timestamp time.Time
}

// ChannelQueue manages messages for a single channel
type ChannelQueue struct {
    mu     sync.Mutex
    queue  chan QueuedMessage
    active bool
}

func NewChannelQueue(bufferSize int) *ChannelQueue {
    return &ChannelQueue{
        queue:  make(chan QueuedMessage, bufferSize),
        active: true,
    }
}

func (q *ChannelQueue) Enqueue(msg QueuedMessage) bool {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    if !q.active {
        return false
    }
    
    select {
    case q.queue <- msg:
        return true
    default:
        return false // Queue full
    }
}

func (q *ChannelQueue) Close() {
    q.mu.Lock()
    defer q.mu.Unlock()
    
    if q.active {
        q.active = false
        close(q.queue)
    }
}

// MessageQueue manages all channel queues
type MessageQueue struct {
    mu     sync.RWMutex
    queues map[string]*ChannelQueue
    bufferSize int
}

func NewMessageQueue(bufferSize int) *MessageQueue {
    return &MessageQueue{
        queues:     make(map[string]*ChannelQueue),
        bufferSize: bufferSize,
    }
}

func (mq *MessageQueue) GetOrCreateQueue(channelID string) *ChannelQueue {
    mq.mu.RLock()
    if q, ok := mq.queues[channelID]; ok {
        mq.mu.RUnlock()
        return q
    }
    mq.mu.RUnlock()
    
    mq.mu.Lock()
    defer mq.mu.Unlock()
    
    // Double-check
    if q, ok := mq.queues[channelID]; ok {
        return q
    }
    
    q := NewChannelQueue(mq.bufferSize)
    mq.queues[channelID] = q
    return q
}

func (mq *MessageQueue) Enqueue(msg QueuedMessage) bool {
    q := mq.GetOrCreateQueue(msg.ChannelID)
    return q.Enqueue(msg)
}

func (mq *MessageQueue) CloseAll() {
    mq.mu.Lock()
    defer mq.mu.Unlock()
    
    for _, q := range mq.queues {
        q.Close()
    }
}
```

#### `internal/core/queue_worker.go`

```go
package core

import (
    "context"
    "log/slog"
)

// QueueWorker processes messages from a channel queue
type QueueWorker struct {
    queue   *ChannelQueue
    bot     *Bot
    channelID string
}

func NewQueueWorker(queue *ChannelQueue, bot *Bot, channelID string) *QueueWorker {
    return &QueueWorker{
        queue:     queue,
        bot:       bot,
        channelID: channelID,
    }
}

func (w *QueueWorker) Start(ctx context.Context) {
    go w.run(ctx)
}

func (w *QueueWorker) run(ctx context.Context) {
    slog.Info("queue worker started", "channel", w.channelID)
    defer slog.Info("queue worker stopped", "channel", w.channelID)
    
    for {
        select {
        case <-ctx.Done():
            return
        case msg, ok := <-w.queue.queue:
            if !ok {
                return // Queue closed
            }
            
            slog.Info("processing queued message", 
                "channel", w.channelID,
                "msg_id", msg.ID,
                "queued_for", time.Since(msg.Timestamp))
            
            if err := w.processMessage(ctx, msg); err != nil {
                slog.Error("processing message", "error", err)
            }
        }
    }
}

func (w *QueueWorker) processMessage(ctx context.Context, msg QueuedMessage) error {
    // Get or create session for this channel
    backend, err := w.bot.sessions.GetOrCreateSession()
    if err != nil {
        return err
    }
    
    msg.Responder.SendTyping()
    
    _, err = backend.Converse(ctx, msg.Content, msg.Responder, w.bot.perms)
    return err
}
```

### 2. Modified Files

#### `internal/core/bot.go`

**Changes:**
- Remove `sync.Mutex` from Bot struct
- Remove mutex lock/unlock from `HandleMessage`
- `HandleMessage` becomes a thin wrapper that delegates to session

```go
// Before:
type Bot struct {
    sessions *SessionManager
    perms    PermissionChecker
    mu       sync.Mutex  // REMOVE
}

func (b *Bot) HandleMessage(responder Responder, userMessage string) error {
    b.mu.Lock()          // REMOVE
    defer b.mu.Unlock()  // REMOVE
    // ... rest
}

// After:
type Bot struct {
    sessions *SessionManager
    perms    PermissionChecker
}

func (b *Bot) HandleMessage(ctx context.Context, msg QueuedMessage) error {
    // No mutex - queue handles serialization per channel
    backend, err := b.sessions.GetOrCreateSession()
    if err != nil {
        return err
    }
    
    msg.Responder.SendTyping()
    _, err = backend.Converse(ctx, msg.Content, msg.Responder, b.perms)
    return err
}
```

#### `internal/handler/discord.go`

**Changes:**
- Add `messageQueue` field to Handler
- Modify `OnMessageCreate` to queue messages instead of direct handling
- Start queue workers for each channel on first message

```go
// Before:
type Handler struct {
    bot           BotInterface
    botID         string
    allowedUsers  []string
    passiveBot    PassiveBotInterface
    buffer        *core.DebouncedBuffer
    discordClient core.DiscordClient
}

func (h *Handler) OnMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
    // ... validation ...
    
    if ok {
        responder := core.NewDiscordResponder(...)
        if err := h.bot.HandleMessage(responder, msg); err != nil {
            slog.Error("handling message", "error", err)
        }
    }
}

// After:
type Handler struct {
    bot           BotInterface
    botID         string
    allowedUsers  []string
    passiveBot    PassiveBotInterface
    buffer        *core.DebouncedBuffer
    discordClient core.DiscordClient
    messageQueue  *core.MessageQueue
    workers       map[string]*core.QueueWorker
    workerMu      sync.Mutex
}

func (h *Handler) OnMessageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
    // ... validation ...
    
    if ok {
        responder := core.NewDiscordResponder(...)
        
        // Ensure worker exists for this channel
        h.ensureWorker(m.ChannelID)
        
        // Queue the message
        queued := h.messageQueue.Enqueue(core.QueuedMessage{
            ID:        m.Message.ID,
            ChannelID: m.ChannelID,
            UserID:    m.Author.ID,
            Content:   msg,
            Responder: responder,
            Timestamp: time.Now(),
        })
        
        if !queued {
            slog.Error("queue full, message dropped", "channel", m.ChannelID)
            responder.PostResponse("❌ Queue full, please try again")
        }
    }
}

func (h *Handler) ensureWorker(channelID string) {
    h.workerMu.Lock()
    defer h.workerMu.Unlock()
    
    if _, ok := h.workers[channelID]; ok {
        return
    }
    
    queue := h.messageQueue.GetOrCreateQueue(channelID)
    worker := core.NewQueueWorker(queue, h.bot, channelID)
    worker.Start(context.Background())
    
    h.workers[channelID] = worker
}
```

#### `cmd/claudecord/main.go`

**Changes:**
- Initialize MessageQueue
- Pass queue to Handler
- Clean up queues on shutdown

```go
func run() error {
    // ... existing setup ...
    
    // Initialize message queue
    messageQueue := core.NewMessageQueue(100) // 100 messages buffer per channel
    
    // Create handler with queue
    h := handler.NewHandler(bot, dg.State.User.ID, cfg.AllowedUsers, 
        discordClient, passiveBot, messageQueue)
    
    // ... rest of setup ...
    
    // Cleanup on shutdown
    defer func() {
        messageQueue.CloseAll()
        sessionMgr.Close()
    }()
    
    // ... wait for interrupt ...
}
```

## Testing Strategy

### Unit Tests

1. **ChannelQueue Tests** (`internal/core/queue_test.go`):
   - Test enqueue/dequeue
   - Test buffer full behavior
   - Test close behavior
   - Test thread safety

2. **MessageQueue Tests**:
   - Test GetOrCreateQueue
   - Test multiple channels isolation
   - Test CloseAll

3. **QueueWorker Tests**:
   - Mock Bot and test message processing
   - Test graceful shutdown
   - Test error handling

### Integration Tests

1. **Handler Integration**:
   - Test message queuing in handler
   - Test worker creation
   - Test queue full error path

## Migration Path

1. **Phase 1**: Implement queue system alongside existing code
2. **Phase 2**: Switch handler to use queue (feature flag)
3. **Phase 3**: Remove old direct handling code
4. **Phase 4**: Remove CLI mode support (future)

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Queue overflow | Configurable buffer size, error message to user |
| Memory growth | Per-channel queues, cleanup on channel inactivity |
| Worker goroutine leaks | Proper context cancellation, worker tracking |
| Order preservation | Single worker per channel guarantees FIFO |
| Error visibility | Log errors, send error messages to Discord |

## Future Enhancements (Post-MVP)

1. **Steering**: Inject messages during active conversations
2. **Priority Queue**: Allow certain commands to skip line
3. **Queue Persistence**: Save queue to disk for crash recovery
4. **Metrics**: Track queue depth, processing time, drop rate
5. **Auto-scaling**: Dynamic worker pools based on load

## Timeline

- **Day 1**: Implement core queue types
- **Day 2**: Implement worker and modify Bot
- **Day 3**: Modify Handler and main
- **Day 4**: Write tests
- **Day 5**: Integration testing and bug fixes

