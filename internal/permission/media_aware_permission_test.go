package permission

import (
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/assert"
)

type denyAllChecker struct{}

func (denyAllChecker) Check(string, core.ToolInput) (bool, string) {
	return false, "wrapped denied"
}

type allowAllChecker struct{}

func (allowAllChecker) Check(string, core.ToolInput) (bool, string) {
	return true, ""
}

func TestMediaAware_ReadUnderMediaDir_Allowed(t *testing.T) {
	a := assert.New(t)

	mac := NewMediaAwarePermissionChecker(denyAllChecker{}, "/var/wa-media")
	allow, _ := mac.Check("Read", core.ToolInput{FilePath: "/var/wa-media/photo.png"})
	a.True(allow)
}

func TestMediaAware_ReadOutsideMediaDir_DelegatesToInner(t *testing.T) {
	a := assert.New(t)

	mac := NewMediaAwarePermissionChecker(denyAllChecker{}, "/var/wa-media")
	allow, reason := mac.Check("Read", core.ToolInput{FilePath: "/etc/hosts"})
	a.False(allow)
	a.Equal("wrapped denied", reason)
}

func TestMediaAware_NonReadAlwaysDelegates(t *testing.T) {
	a := assert.New(t)

	mac := NewMediaAwarePermissionChecker(denyAllChecker{}, "/var/wa-media")
	// Even a Bash with a path under mediaDir should still hit the inner checker.
	allow, reason := mac.Check("Bash", core.ToolInput{FilePath: "/var/wa-media/photo.png"})
	a.False(allow)
	a.Equal("wrapped denied", reason)
}

func TestMediaAware_EmptyMediaDir_DoesNotShortCircuit(t *testing.T) {
	a := assert.New(t)

	mac := NewMediaAwarePermissionChecker(denyAllChecker{}, "")
	allow, _ := mac.Check("Read", core.ToolInput{FilePath: "/var/wa-media/photo.png"})
	a.False(allow)
}

func TestMediaAware_DelegatesAllowedReadOutsideMediaDir(t *testing.T) {
	a := assert.New(t)

	mac := NewMediaAwarePermissionChecker(allowAllChecker{}, "/var/wa-media")
	allow, _ := mac.Check("Read", core.ToolInput{FilePath: "/home/code/file.go"})
	a.True(allow)
}

func TestMediaAware_PathTraversalDoesNotEscapeMediaDir(t *testing.T) {
	a := assert.New(t)

	mac := NewMediaAwarePermissionChecker(denyAllChecker{}, "/var/wa-media")
	allow, _ := mac.Check("Read", core.ToolInput{FilePath: "/var/wa-media/../etc/passwd"})
	a.False(allow)
}
