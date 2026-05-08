package discord

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
)

type discordSessionMock struct{ mock.Mock }

func (m *discordSessionMock) ChannelMessageSend(channelID, content string) error {
	args := m.Called(channelID, content)
	return args.Error(0)
}
func (m *discordSessionMock) ChannelTyping(channelID string) error {
	return m.Called(channelID).Error(0)
}
func (m *discordSessionMock) MessageReactionAdd(channelID, messageID, emoji string) error {
	return m.Called(channelID, messageID, emoji).Error(0)
}

const maxLen = 2000

func TestOutbound_PostResponse_ChunksLongMessages(t *testing.T) {
	// given
	// ... an outbound bound to a thread and a >2000-char payload
	s := &discordSessionMock{}
	o := newOutbound(s, "thread-1", "msg-1", maxLen)
	long := strings.Repeat("x", maxLen+50)
	s.On("ChannelMessageSend", "thread-1", mock.Anything).Return(nil).Twice()

	// when
	// ... PostResponse is called
	err := o.PostResponse(long)

	// then
	// ... the payload was sent in 2 chunks to the thread
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.AssertNumberOfCalls(t, "ChannelMessageSend", 2)
}

func TestOutbound_AddReaction_TargetsOriginalMessage(t *testing.T) {
	// given
	// ... an outbound with a known message id
	s := &discordSessionMock{}
	o := newOutbound(s, "thread-1", "msg-42", maxLen)
	s.On("MessageReactionAdd", "thread-1", "msg-42", "👀").Return(nil).Once()

	// when
	// ... AddReaction is called
	err := o.AddReaction("👀")

	// then
	// ... the reaction was added to the right message
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.AssertExpectations(t)
}

func TestOutbound_SendUpdate_PostsInSameThread(t *testing.T) {
	// given
	// ... an outbound bound to a thread
	s := &discordSessionMock{}
	o := newOutbound(s, "thread-1", "msg-1", maxLen)
	s.On("ChannelMessageSend", "thread-1", "doing the thing").Return(nil).Once()

	// when
	// ... SendUpdate is called
	err := o.SendUpdate("doing the thing")

	// then
	// ... the update was posted to the thread
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.AssertExpectations(t)
}
