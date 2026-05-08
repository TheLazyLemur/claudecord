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

func fmtThread(i int) string { return "t-" + strconv.Itoa(i) }
