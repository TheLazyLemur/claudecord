package discord

import (
	"strconv"
	"testing"
)

func TestThreadRegistry_RemembersOwnership(t *testing.T) {
	// given
	// ... an empty registry
	r := newThreadRegistry()

	// when
	// ... a thread id is registered as bot-owned
	r.markOwned("thread-123")

	// then
	// ... ownership is reported true for that id and false for others
	if !r.owns("thread-123") {
		t.Fatalf("expected ownership of thread-123")
	}
	if r.owns("thread-999") {
		t.Fatalf("did not expect ownership of thread-999")
	}
}

func TestThreadRegistry_ConcurrentSafe(t *testing.T) {
	// given
	// ... a registry hit from multiple goroutines
	r := newThreadRegistry()
	done := make(chan struct{})

	// when
	// ... 100 concurrent writes complete
	for i := 0; i < 100; i++ {
		go func(i int) {
			r.markOwned(fmtThread(i))
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}

	// then
	// ... every id is reported owned
	for i := 0; i < 100; i++ {
		if !r.owns(fmtThread(i)) {
			t.Fatalf("missing %s", fmtThread(i))
		}
	}
}

func TestThreadRegistry_EvictsOldestPastBound(t *testing.T) {
	// given
	// ... a registry filled to its capacity plus one
	r := newThreadRegistry()
	first := fmtThread(0)
	for i := 0; i <= threadRegistryMax; i++ {
		r.markOwned(fmtThread(i))
	}

	// when
	// ... ownership of the very first id is queried
	owned := r.owns(first)

	// then
	// ... the first id was evicted to make room
	if owned {
		t.Fatalf("expected first thread to be evicted past the bound of %d", threadRegistryMax)
	}
}

func TestThreadRegistry_NewIdsStillOwned(t *testing.T) {
	// given
	// ... a registry filled to exactly the bound, then one more added
	r := newThreadRegistry()
	for i := 0; i <= threadRegistryMax; i++ {
		r.markOwned(fmtThread(i))
	}

	// when
	// ... the most-recent N ids are checked
	allOwned := true
	for i := 1; i <= threadRegistryMax; i++ {
		if !r.owns(fmtThread(i)) {
			allOwned = false
			break
		}
	}

	// then
	// ... all recent ids within the bound are still owned
	if !allOwned {
		t.Fatalf("expected the last %d ids to be owned", threadRegistryMax)
	}
}

func fmtThread(i int) string { return "t-" + strconv.Itoa(i) }
