package tools

import (
	"fmt"
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

type permResponder struct {
	approved bool
	err      error
	prompt   string
}

func (p *permResponder) SendTyping() error                  { return nil }
func (p *permResponder) PostResponse(string) error          { return nil }
func (p *permResponder) AddReaction(string) error           { return nil }
func (p *permResponder) SendUpdate(string) error            { return nil }
func (p *permResponder) AskPermission(prompt string) (bool, error) {
	p.prompt = prompt
	return p.approved, p.err
}

func TestCheckPermission_AutoAllow(t *testing.T) {
	a := assert.New(t)
	perms := &mockPerms{allow: true}
	resp := &permResponder{}

	allow, reason := CheckPermission("Bash", core.ToolInput{Command: "ls"}, perms, resp)

	a.True(allow)
	a.Empty(reason)
	a.Empty(resp.prompt) // should not have asked user
}

func TestCheckPermission_DenyThenUserApproves(t *testing.T) {
	a := assert.New(t)
	perms := &mockPerms{allow: false, reason: "not in allowed dirs"}
	resp := &permResponder{approved: true}

	allow, reason := CheckPermission("Bash", core.ToolInput{Command: "rm -rf /"}, perms, resp)

	a.True(allow)
	a.Empty(reason)
	a.Contains(resp.prompt, "**Bash**")
}

func TestCheckPermission_DenyThenUserDenies(t *testing.T) {
	a := assert.New(t)
	perms := &mockPerms{allow: false, reason: "not in allowed dirs"}
	resp := &permResponder{approved: false}

	allow, reason := CheckPermission("Bash", core.ToolInput{Command: "rm -rf /"}, perms, resp)

	a.False(allow)
	a.Equal("not in allowed dirs", reason)
}

func TestCheckPermission_AskPermissionError_StillDenies(t *testing.T) {
	a := assert.New(t)
	perms := &mockPerms{allow: false, reason: "blocked"}
	resp := &permResponder{approved: false, err: fmt.Errorf("discord timeout")}

	allow, reason := CheckPermission("Read", core.ToolInput{}, perms, resp)

	a.False(allow)
	a.Equal("blocked", reason)
}

func TestFormatPermissionPrompt_Command(t *testing.T) {
	a := assert.New(t)

	prompt := FormatPermissionPrompt("Bash", core.ToolInput{Command: "rm -rf /"})

	a.Contains(prompt, "**Bash**")
	a.Contains(prompt, "rm -rf /")
}

func TestFormatPermissionPrompt_FilePath(t *testing.T) {
	a := assert.New(t)

	prompt := FormatPermissionPrompt("Read", core.ToolInput{FilePath: "/etc/passwd"})

	a.Contains(prompt, "**Read**")
	a.Contains(prompt, "/etc/passwd")
}
