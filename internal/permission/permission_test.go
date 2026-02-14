package permission

import (
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/assert"
)

func TestPermissionChecker_ReadAllowedInAllowedDir(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})
	input := core.ToolInput{FilePath: "/home/user/projects/myapp/main.go"}

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
	input := core.ToolInput{FilePath: "/etc/passwd"}

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
	input := core.ToolInput{FilePath: "/home/user/projects/deep/nested/file.go"}

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
	input := core.ToolInput{FilePath: "/home/user/projects/../../../etc/passwd"}

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
	input := core.ToolInput{FilePath: "/home/user/projects/test.txt"}

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
	input := core.ToolInput{Command: "ls -la"}

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
	input := core.ToolInput{FilePath: "/home/user/projects/main.go"}

	// when
	allow, reason := checker.Check("Edit", input)

	// then
	a.False(allow)
	a.Contains(reason, "requires approval")
}

func TestPermissionChecker_FetchGETAutoApproves(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})

	// when/then - explicit GET
	allow, reason := checker.Check("Fetch", core.ToolInput{URL: "https://example.com", Method: "GET"})
	a.True(allow)
	a.Empty(reason)

	// when/then - missing method defaults to GET
	allow, reason = checker.Check("Fetch", core.ToolInput{URL: "https://example.com"})
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_FetchMutatingMethodsRequireApproval(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})

	methods := []string{"POST", "PATCH", "DELETE"}
	for _, method := range methods {
		// when
		allow, reason := checker.Check("Fetch", core.ToolInput{URL: "https://example.com", Method: method})

		// then
		a.False(allow, "method %s should require approval", method)
		a.Contains(reason, "requires approval", "method %s", method)
	}
}

func TestPermissionChecker_SkillToolsAutoApprove(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})

	// when/then - Skill auto-approves
	allow, reason := checker.Check("Skill", core.ToolInput{Name: "test-skill"})
	a.True(allow)
	a.Empty(reason)

	// when/then - LoadSkillSupporting auto-approves
	allow, reason = checker.Check("LoadSkillSupporting", core.ToolInput{Name: "test-skill", Path: "scripts/run.sh"})
	a.True(allow)
	a.Empty(reason)
}

func TestPermissionChecker_WebSearchAutoApproves(t *testing.T) {
	a := assert.New(t)

	// given
	checker := NewPermissionChecker([]string{"/home/user/projects"})

	// when
	allow, reason := checker.Check("WebSearch", core.ToolInput{Query: "golang http client"})

	// then
	a.True(allow)
	a.Empty(reason)
}
