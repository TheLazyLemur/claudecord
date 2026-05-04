package permission

import (
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestAutoApprove_WriteInAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{FilePath: "/home/user/projects/file.txt"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestAutoApprove_BashInAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{Command: "go build ./..."}

	// when
	allow, reason := checker.Check("Bash", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestAutoApprove_EditInAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{FilePath: "/home/user/projects/main.go"}

	// when
	allow, reason := checker.Check("Edit", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestAutoApprove_FetchPOSTApproves(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{URL: "https://example.com", Method: "POST"}

	// when
	allow, reason := checker.Check("Fetch", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestAutoApprove_DeniesPathOutsideAllowedDirs(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{FilePath: "/etc/passwd"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.False(allow)
	a.Contains(reason, "outside allowed directories")
}

func TestAutoApprove_DeniesPathTraversal(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{FilePath: "/home/user/projects/../../../etc/passwd"}

	// when
	allow, reason := checker.Check("Edit", input)

	// then
	a.False(allow)
	a.Contains(reason, "outside allowed directories")
}

func TestAutoApprove_ToolWithNoPathsApproves(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{Query: "golang http client"}

	// when
	allow, reason := checker.Check("WebSearch", input)

	// then
	a.True(allow)
	a.Empty(reason)
}

func TestAutoApprove_RejectsPrefixTrick(t *testing.T) {
	a := assert.New(t)

	// given
	// ... allowed dir is /home/user/projects, sibling /home/user/projects-evil must not match
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{FilePath: "/home/user/projects-evil/file.go"}

	// when
	allow, reason := checker.Check("Write", input)

	// then
	a.False(allow)
	a.Contains(reason, "outside allowed directories")
}

func TestAutoApprove_CleansTrailingSlashOnAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	// ... allowed dir is given with a trailing slash
	checker := NewAutoApprovePermissionChecker([]string{"/home/user/projects/"})
	input := core.ToolInput{FilePath: "/home/user/projects/file.go"}

	// when
	allow, _ := checker.Check("Write", input)

	// then
	a.True(allow)
}

func TestAutoApprove_AllPathFields(t *testing.T) {
	a := assert.New(t)

	// given
	// ... a tool input populates FilePath, Path, and Directory at once
	checker := NewAutoApprovePermissionChecker([]string{"/home"})
	input := core.ToolInput{
		FilePath:  "/home/a.go",
		Path:      "/home/b.go",
		Directory: "/home/c",
	}

	// when
	allow, _ := checker.Check("Write", input)

	// then
	a.True(allow)
}
