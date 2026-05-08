package core

import (
	"context"
	"log/slog"

	"github.com/pkg/errors"
)

// HandleInbound routes a single inbound message to the active backend, resetting
// the session if the SessionKey has changed since the last inbound.
//
// Concurrency: concurrent inbounds for the SAME session key proceed in parallel
// under RLock; a key mismatch upgrades to a write lock and rotates the session,
// blocking until in-flight messages drain (same as NewSession behaviour).
func (b *Bot) HandleInbound(in Inbound) error {
	if in.SessionKey == "" {
		return errors.New("inbound: empty SessionKey")
	}

	b.mu.RLock()
	matches := in.SessionKey == b.activeKey
	b.mu.RUnlock()

	if !matches {
		b.mu.Lock()
		if in.SessionKey != b.activeKey {
			if err := b.sessions.NewSession("", in.Capabilities); err != nil {
				b.mu.Unlock()
				return errors.Wrap(err, "rotating session on key change")
			}
			b.activeKey = in.SessionKey
			b.activeCaps = in.Capabilities
		}
		b.mu.Unlock()
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	if in.Reply != nil {
		_ = in.Reply.SendTyping()
	}

	backend, err := b.sessions.GetOrCreateSession(b.activeCaps)
	if err != nil {
		return errors.Wrap(err, "getting session")
	}

	slog.Info("dispatching inbound", "key", string(in.SessionKey), "session", backend.SessionID())

	ctx, cancel := context.WithTimeout(context.Background(), b.converseTimeout)
	defer cancel()
	response, err := backend.Converse(ctx, in, in.Reply, b.perms)
	if err != nil {
		return errors.Wrap(err, "converse")
	}
	if response != "" && in.Reply != nil {
		if err := in.Reply.PostResponse(response); err != nil {
			return errors.Wrap(err, "posting response")
		}
	}
	return nil
}
