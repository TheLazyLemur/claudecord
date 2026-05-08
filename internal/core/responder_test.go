package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChunkMessage_Short(t *testing.T) {
	a := assert.New(t)
	a.Equal([]string{"hello"}, ChunkMessage("hello", 10))
}

func TestChunkMessage_ExactLimit(t *testing.T) {
	a := assert.New(t)
	a.Equal([]string{"12345"}, ChunkMessage("12345", 5))
}

func TestChunkMessage_MultipleChunks(t *testing.T) {
	a := assert.New(t)
	a.Equal([]string{"abc", "def", "ghi", "j"}, ChunkMessage("abcdefghij", 3))
}

func TestChunkMessage_Empty(t *testing.T) {
	a := assert.New(t)
	a.Empty(ChunkMessage("", 10))
}

func TestChunkMessage_DiscordLimit(t *testing.T) {
	a := assert.New(t)
	long := strings.Repeat("x", 4500)
	chunks := ChunkMessage(long, MaxDiscordMessageLen)
	a.Len(chunks, 3)
	a.Len(chunks[0], 2000)
	a.Len(chunks[1], 2000)
	a.Len(chunks[2], 500)
}
