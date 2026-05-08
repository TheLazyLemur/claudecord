package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

type recordingBackend struct {
	mockBackend
	gotPrompt    string
	gotPerms     PermissionChecker
	gotResponder Outbound
	converseErr  error
}

func (r *recordingBackend) Converse(ctx context.Context, in Inbound, responder Outbound, perms PermissionChecker) (string, error) {
	r.gotPrompt = in.Text
	r.gotPerms = perms
	r.gotResponder = responder
	return "", r.converseErr
}

type alwaysAllow struct{}

func (alwaysAllow) Check(toolName string, input ToolInput) (bool, string) {
	return true, ""
}

func TestMemoryFlusher_CallsConverseWithFlushPrompt(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a flusher and a backend
	backend := &recordingBackend{}
	flush := NewMemoryFlusher(alwaysAllow{})

	// when
	// ... the flusher runs
	flush(context.Background(), backend)

	// then
	// ... the backend was asked to converse with a prompt mentioning the memory scripts
	a.Contains(backend.gotPrompt, "remember.sh")
	a.Contains(backend.gotPrompt, "note.sh")
	a.NotNil(backend.gotPerms)
	a.NotNil(backend.gotResponder)
}

func TestMemoryFlusher_PassesThroughGivenPerms(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a flusher with a specific permissions checker
	backend := &recordingBackend{}
	perms := alwaysAllow{}
	flush := NewMemoryFlusher(perms)

	// when
	// ... the flusher runs
	flush(context.Background(), backend)

	// then
	// ... the same perms instance is passed to Converse
	a.Equal(perms, backend.gotPerms)
}

func TestMemoryFlusher_SwallowsConverseErrors(t *testing.T) {
	// given
	// ... a backend that returns an error
	backend := &recordingBackend{converseErr: errors.New("boom")}
	flush := NewMemoryFlusher(alwaysAllow{})

	// when
	// ... the flusher runs
	flush(context.Background(), backend)

	// then
	// ... no panic; reaching this line is the assertion
}

func TestMemoryFlusher_UsesNoopResponder(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a flusher
	backend := &recordingBackend{}
	flush := NewMemoryFlusher(alwaysAllow{})

	// when
	// ... the flusher runs
	flush(context.Background(), backend)

	// then
	// ... the responder discards output (no-op type)
	a.Equal("*core.noopResponder", fmt.Sprintf("%T", backend.gotResponder))
}
