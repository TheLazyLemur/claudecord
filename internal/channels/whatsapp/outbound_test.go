package whatsapp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutbound_PostResponse_ChunksAtMaxLen(t *testing.T) {
	// given
	// ... a messenger mock and an outbound, and a message exactly maxMessageLen bytes
	msgr := &messengerMock{}
	out := NewOutbound(msgr, "chat-1@g.us")
	content := strings.Repeat("x", maxMessageLen)
	msgr.On("SendText", "chat-1@g.us", content).Return(nil)

	// when
	// ... PostResponse is called with exactly maxMessageLen bytes
	err := out.PostResponse(content)

	// then
	// ... exactly one send is made and no error is returned
	require.NoError(t, err)
	msgr.AssertNumberOfCalls(t, "SendText", 1)
}

func TestOutbound_PostResponse_SplitsWhenOverMaxLen(t *testing.T) {
	// given
	// ... a messenger mock and an outbound, and a message one byte over maxMessageLen
	msgr := &messengerMock{}
	out := NewOutbound(msgr, "chat-1@g.us")
	first := strings.Repeat("x", maxMessageLen)
	second := "y"
	msgr.On("SendText", "chat-1@g.us", first).Return(nil)
	msgr.On("SendText", "chat-1@g.us", second).Return(nil)

	// when
	// ... PostResponse is called with maxMessageLen+1 bytes
	err := out.PostResponse(first + second)

	// then
	// ... two sends are made, one for each chunk
	require.NoError(t, err)
	msgr.AssertNumberOfCalls(t, "SendText", 2)
	msgr.AssertCalled(t, "SendText", "chat-1@g.us", first)
	msgr.AssertCalled(t, "SendText", "chat-1@g.us", second)
}
