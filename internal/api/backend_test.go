package api

import (
	"testing"

	"github.com/TheLazyLemur/claudecord/internal/tools"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildToolResultBlock_TextPath(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	block := buildToolResultBlock("call-1", "plain text result", false)

	r.NotNil(block.OfToolResult)
	tr := block.OfToolResult
	a.Equal("call-1", tr.ToolUseID)
	r.Len(tr.Content, 1)
	a.NotNil(tr.Content[0].OfText)
	a.Nil(tr.Content[0].OfImage)
	a.Equal("plain text result", tr.Content[0].OfText.Text)
}

func TestBuildToolResultBlock_ImageSentinelTurnsIntoImageBlock(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	sentinel := tools.ImageSentinel + "\timage/png\tBASE64DATA"
	block := buildToolResultBlock("call-2", sentinel, false)

	r.NotNil(block.OfToolResult)
	tr := block.OfToolResult
	a.Equal("call-2", tr.ToolUseID)
	r.Len(tr.Content, 1)
	r.NotNil(tr.Content[0].OfImage)
	a.Nil(tr.Content[0].OfText)

	src := tr.Content[0].OfImage.Source.OfBase64
	r.NotNil(src)
	a.Equal("BASE64DATA", src.Data)
	a.Equal(anthropic.Base64ImageSourceMediaType("image/png"), src.MediaType)
}

func TestBuildToolResultBlock_MalformedSentinelFallsBackToText(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// Missing third field; SplitN gives < 3 parts so we fall through to text.
	bad := tools.ImageSentinel + "\timage/png"
	block := buildToolResultBlock("call-3", bad, false)

	r.NotNil(block.OfToolResult)
	r.Len(block.OfToolResult.Content, 1)
	a.NotNil(block.OfToolResult.Content[0].OfText)
}

func TestBuildToolResultBlock_ErrorIsPlainText(t *testing.T) {
	r := require.New(t)
	a := assert.New(t)

	// Even if the result happens to start with the sentinel, an error result
	// is reported as text (so the model sees the error verbatim).
	block := buildToolResultBlock("call-4", tools.ImageSentinel+"\timage/png\tdata", true)

	r.NotNil(block.OfToolResult)
	r.Len(block.OfToolResult.Content, 1)
	a.NotNil(block.OfToolResult.Content[0].OfText)
	a.True(block.OfToolResult.IsError.Valid())
	a.True(block.OfToolResult.IsError.Value)
}
