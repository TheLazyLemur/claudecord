package discord

import "sync"

// threadRegistry tracks Discord thread IDs created by us so the plugin can
// distinguish a continuation from a foreign-thread message.
type threadRegistry struct {
	mu    sync.RWMutex
	owned map[string]struct{}
}

func newThreadRegistry() *threadRegistry {
	return &threadRegistry{owned: make(map[string]struct{})}
}

func (r *threadRegistry) markOwned(threadID string) {
	r.mu.Lock()
	r.owned[threadID] = struct{}{}
	r.mu.Unlock()
}

func (r *threadRegistry) owns(threadID string) bool {
	r.mu.RLock()
	_, ok := r.owned[threadID]
	r.mu.RUnlock()
	return ok
}
