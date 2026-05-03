package tools

import (
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/stretchr/testify/assert"
)

type mockPerms struct {
	allow  bool
	reason string
}

func (m *mockPerms) Check(string, core.ToolInput) (bool, string) {
	return m.allow, m.reason
}

func TestCheckPermission_Allow(t *testing.T) {
	a := assert.New(t)
	perms := &mockPerms{allow: true}

	allow, reason := CheckPermission("Bash", core.ToolInput{Command: "ls"}, perms)

	a.True(allow)
	a.Empty(reason)
}

func TestCheckPermission_Deny(t *testing.T) {
	a := assert.New(t)
	perms := &mockPerms{allow: false, reason: "not in allowed dirs"}

	allow, reason := CheckPermission("Bash", core.ToolInput{Command: "rm -rf /"}, perms)

	a.False(allow)
	a.Equal("not in allowed dirs", reason)
}
