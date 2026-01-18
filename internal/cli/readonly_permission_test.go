package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadOnlyPermissionChecker_AllowsReadTools(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewReadOnlyPermissionChecker([]string{"/allowed"})

	// then - read tools allowed
	readTools := []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch"}
	for _, tool := range readTools {
		allow, _ := checker.Check(tool, map[string]any{"file_path": "/allowed/test.go"})
		a.True(allow, "should allow %s", tool)
	}
}

func TestReadOnlyPermissionChecker_DeniesWriteTools(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewReadOnlyPermissionChecker([]string{"/allowed"})

	// then - write tools denied
	writeTools := []string{"Write", "Edit", "Bash", "NotebookEdit"}
	for _, tool := range writeTools {
		allow, reason := checker.Check(tool, map[string]any{})
		a.False(allow, "should deny %s", tool)
		a.Contains(reason, "read-only")
	}
}

func TestReadOnlyPermissionChecker_EnforcesAllowedDirs(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewReadOnlyPermissionChecker([]string{"/allowed", "/also-allowed"})

	// then
	allow, _ := checker.Check("Read", map[string]any{"file_path": "/allowed/foo.go"})
	a.True(allow)

	allow, _ = checker.Check("Read", map[string]any{"file_path": "/also-allowed/bar.go"})
	a.True(allow)

	allow, reason := checker.Check("Read", map[string]any{"file_path": "/not-allowed/baz.go"})
	a.False(allow)
	a.Contains(reason, "outside allowed")
}
