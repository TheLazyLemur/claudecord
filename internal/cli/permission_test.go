package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermissionChecker_AllowsPathWithinAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/myapp/main.go"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_DeniesPathOutsideAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/etc/passwd"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.False(allow)
	a.Contains(reason, "/etc/passwd")
	a.Contains(reason, "outside allowed directories")
}

func TestPermissionChecker_AllowsWhenNoPathFieldsPresent(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"query": "search term"}

	// when
	allow, reason := checker.Check("WebSearch", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_ChecksMultiplePathFields(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{
		"file_path": "/home/user/projects/src/main.go",
		"path":      "/etc/shadow",
	}

	// when
	allow, reason := checker.Check("SomeTool", input)

	// then
	a.False(allow)
	a.Contains(reason, "/etc/shadow")
}

func TestPermissionChecker_AllowsMultipleAllowedDirs(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects", "/tmp"})
	input := map[string]any{"file_path": "/tmp/test.txt"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_RecursiveSubdirectories(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/deep/nested/path/file.go"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_DeniesPathTraversal(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/../../../etc/passwd"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.False(allow)
	a.Contains(reason, "outside allowed directories")
}

func TestPermissionChecker_HandlesDirectoryField(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"directory": "/etc"}

	// when
	allow, reason := checker.Check("Bash", input)

	// then
	a.False(allow)
	a.Contains(reason, "/etc")
}

func TestPermissionChecker_AllowsEmptyAllowedDirs(t *testing.T) {
	a := assert.New(t)

	// given - no allowed dirs means deny all paths
	checker := NewPermissionChecker([]string{})
	input := map[string]any{"file_path": "/any/path"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.False(allow)
	a.Contains(reason, "outside allowed directories")
}

func TestPermissionChecker_AllowsExactMatchDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"directory": "/home/user/projects"}

	// when
	allow, reason := checker.Check("Bash", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_HandlesBashCommandField(t *testing.T) {
	a := assert.New(t)

	// given - Bash tool uses "command" field, not path-based
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"command": "ls -la"}

	// when
	allow, reason := checker.Check("Bash", input)

	// then
	a.True(allow)
	a.Empty(reason)
}
