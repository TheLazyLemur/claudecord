package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSendUpdateTool_Name(t *testing.T) {
	// given
	// ... the SendUpdateTool builder

	// when
	// ... called
	tool := SendUpdateTool()

	// then
	// ... tool name is send_update
	assert.Equal(t, "send_update", tool.Name)
}

func TestReactEmojiTool_Name(t *testing.T) {
	// given
	// ... the ReactEmojiTool builder

	// when
	// ... called
	tool := ReactEmojiTool()

	// then
	// ... tool name is react_emoji
	assert.Equal(t, "react_emoji", tool.Name)
}

func TestReactEmojiTool_RequiresEmojiParam(t *testing.T) {
	// given
	// ... the ReactEmojiTool builder

	// when
	// ... called
	tool := ReactEmojiTool()

	// then
	// ... required fields include emoji
	required, _ := tool.InputSchema["required"].([]string)
	assert.Contains(t, required, "emoji")
}

func TestSendUpdateTool_RequiresMessageParam(t *testing.T) {
	// given
	// ... the SendUpdateTool builder

	// when
	// ... called
	tool := SendUpdateTool()

	// then
	// ... required fields include message
	required, _ := tool.InputSchema["required"].([]string)
	assert.Contains(t, required, "message")
}
