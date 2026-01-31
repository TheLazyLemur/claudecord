package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPermissionChecker_ReadAllowedInAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/myapp/main.go"}

	// when
	allow, reason := checker.Check("Read", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_ReadDeniedOutsideAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/etc/passwd"}

	// when
	allow, reason := checker.Check("Read", input)

	// then
	a.False(allow)
	a.Contains(reason, "/etc/passwd")
	a.Contains(reason, "outside allowed directories")
}

func TestPermissionChecker_ReadAllowsSubdirectories(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/deep/nested/file.go"}

	// when
	allow, reason := checker.Check("Read", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_ReadDeniesPathTraversal(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/../../../etc/passwd"}

	// when
	allow, reason := checker.Check("Read", input)

	// then
	a.False(allow)
	a.Contains(reason, "outside allowed directories")
}

func TestPermissionChecker_NonReadToolRequiresApproval(t *testing.T) {
	a := assert.New(t)

	// given - Write tool requires approval even in allowed dir
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/test.txt"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.False(allow)
	a.Contains(reason, "requires approval")
}

func TestPermissionChecker_BashRequiresApproval(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"command": "ls -la"}

	// when
	allow, reason := checker.Check("Bash", input)

	// then
	a.False(allow)
	a.Contains(reason, "requires approval")
}

func TestPermissionChecker_EditRequiresApproval(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := map[string]any{"file_path": "/home/user/projects/main.go"}

	// when
	allow, reason := checker.Check("Edit", input)

	// then
	a.False(allow)
	a.Contains(reason, "requires approval")
}
