package discord

import "sync"

// threadRegistryMax is the maximum number of thread IDs kept in memory.
// When exceeded, the oldest entry is evicted.
const threadRegistryMax = 1000

// threadRegistry tracks Discord thread IDs created by us so the plugin can
// distinguish a continuation from a foreign-thread message.
// It is bounded to threadRegistryMax entries; oldest is evicted on overflow.
type threadRegistry struct {
	mu    sync.RWMutex
	owned map[string]struct{}
	order []string
}

func newThreadRegistry() *threadRegistry {
	return &threadRegistry{owned: make(map[string]struct{})}
}

func (r *threadRegistry) markOwned(threadID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.owned[threadID]; !exists {
		r.order = append(r.order, threadID)
		r.owned[threadID] = struct{}{}
		if len(r.order) > threadRegistryMax {
			oldest := r.order[0]
			r.order = r.order[1:]
			delete(r.owned, oldest)
		}
	}
}

func (r *threadRegistry) owns(threadID string) bool {
	r.mu.RLock()
	_, ok := r.owned[threadID]
	r.mu.RUnlock()
	return ok
}
