package core

import (
	"context"
	"log/slog"
	"time"
)

const memoryFlushTimeout = 30 * time.Second

const memoryFlushPrompt = "This conversation is ending. " +
	"Use bash to run remember.sh for any durable facts you have established this session, " +
	"and note.sh for tactical context worth keeping. " +
	"Be brief — only commit things actually established this session. " +
	"Do not respond with any other text."

// NewMemoryFlusher returns a FlushFunc that fires one final agent turn
// against the outgoing backend, instructing the model to commit unsaved
// memory state via the memory skill scripts. The flush has a hard 30s
// timeout; errors are logged and swallowed so the session reset is never
// blocked.
func NewMemoryFlusher(perms PermissionChecker) FlushFunc {
	return func(ctx context.Context, current Backend) {
		timeoutCtx, cancel := context.WithTimeout(ctx, memoryFlushTimeout)
		defer cancel()

		responder := &noopResponder{}
		if _, err := current.Converse(timeoutCtx, memoryFlushPrompt, responder, perms); err != nil {
			slog.Warn("memory flush failed", "error", err)
		}
	}
}
