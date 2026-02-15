package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChatTools_ReturnsSendUpdateOnly(t *testing.T) {
	a := assert.New(t)

	tools := ChatTools()

	a.Len(tools, 1)
	a.Equal("send_update", tools[0].Name)
}

func TestChatTools_ExcludesReactEmoji(t *testing.T) {
	a := assert.New(t)

	tools := ChatTools()

	for _, t := range tools {
		a.NotEqual("react_emoji", t.Name)
	}
}

func TestDiscordTools_IncludesReactEmoji(t *testing.T) {
	a := assert.New(t)

	tools := DiscordTools()

	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	a.Contains(names, "react_emoji")
	a.Contains(names, "send_update")
}
