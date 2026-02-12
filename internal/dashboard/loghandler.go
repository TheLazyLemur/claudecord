package dashboard

import (
	"context"
	"log/slog"
	"time"
)

// BroadcastHandler wraps a slog.Handler and broadcasts logs to a Hub.
type BroadcastHandler struct {
	inner slog.Handler
	hub   *Hub
}

// NewBroadcastHandler creates a handler that broadcasts to hub and delegates to inner.
func NewBroadcastHandler(hub *Hub, inner slog.Handler) *BroadcastHandler {
	return &BroadcastHandler{
		inner: inner,
		hub:   hub,
	}
}

// Enabled reports whether the handler handles records at the given level.
func (h *BroadcastHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle broadcasts the log record and delegates to inner handler.
func (h *BroadcastHandler) Handle(ctx context.Context, r slog.Record) error {
	// Broadcast to WS clients
	h.hub.Broadcast(Message{
		Type:  "log",
		Level: r.Level.String(),
		Msg:   r.Message,
		Time:  r.Time.Format(time.RFC3339),
	})

	// Delegate to inner handler
	return h.inner.Handle(ctx, r)
}

// WithAttrs returns a new handler with the given attributes.
func (h *BroadcastHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BroadcastHandler{
		inner: h.inner.WithAttrs(attrs),
		hub:   h.hub,
	}
}

// WithGroup returns a new handler with the given group.
func (h *BroadcastHandler) WithGroup(name string) slog.Handler {
	return &BroadcastHandler{
		inner: h.inner.WithGroup(name),
		hub:   h.hub,
	}
}
