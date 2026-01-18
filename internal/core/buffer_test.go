package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMessageBuffer_Add_StoresMessage(t *testing.T) {
	a := assert.New(t)

	// given
	buf := NewMessageBuffer()
	msg := BufferedMessage{
		ChannelID: "chan-1",
		MessageID: "msg-1",
		Content:   "hello",
		AuthorID:  "user-1",
	}

	// when
	buf.Add(msg)

	// then
	msgs := buf.Get("chan-1")
	a.Len(msgs, 1)
	a.Equal("hello", msgs[0].Content)
}

func TestMessageBuffer_Add_AccumulatesPerChannel(t *testing.T) {
	a := assert.New(t)

	// given
	buf := NewMessageBuffer()

	// when
	buf.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m1", Content: "first"})
	buf.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m2", Content: "second"})
	buf.Add(BufferedMessage{ChannelID: "chan-2", MessageID: "m3", Content: "other channel"})

	// then
	chan1Msgs := buf.Get("chan-1")
	a.Len(chan1Msgs, 2)
	a.Equal("first", chan1Msgs[0].Content)
	a.Equal("second", chan1Msgs[1].Content)

	chan2Msgs := buf.Get("chan-2")
	a.Len(chan2Msgs, 1)
	a.Equal("other channel", chan2Msgs[0].Content)
}

func TestMessageBuffer_Clear_RemovesChannelMessages(t *testing.T) {
	a := assert.New(t)

	// given
	buf := NewMessageBuffer()
	buf.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m1", Content: "msg"})
	buf.Add(BufferedMessage{ChannelID: "chan-2", MessageID: "m2", Content: "other"})

	// when
	buf.Clear("chan-1")

	// then
	a.Len(buf.Get("chan-1"), 0)
	a.Len(buf.Get("chan-2"), 1)
}

func TestMessageBuffer_Get_ReturnsEmptyForUnknownChannel(t *testing.T) {
	a := assert.New(t)

	// given
	buf := NewMessageBuffer()

	// when
	msgs := buf.Get("unknown")

	// then
	a.Len(msgs, 0)
}

func TestDebouncedBuffer_FiresAfterDelay(t *testing.T) {
	r := require.New(t)

	// given
	fired := make(chan string, 1)
	callback := func(channelID string, msgs []BufferedMessage) {
		fired <- channelID
	}
	db := NewDebouncedBuffer(50*time.Millisecond, callback)
	defer db.Stop()

	// when
	db.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m1", Content: "test"})

	// then
	select {
	case ch := <-fired:
		r.Equal("chan-1", ch)
	case <-time.After(200 * time.Millisecond):
		t.Fatal("callback not fired")
	}
}

func TestDebouncedBuffer_ResetsTimerOnNewMessage(t *testing.T) {
	a := assert.New(t)

	// given
	fired := make(chan []BufferedMessage, 1)
	callback := func(channelID string, msgs []BufferedMessage) {
		fired <- msgs
	}
	db := NewDebouncedBuffer(100*time.Millisecond, callback)
	defer db.Stop()

	// when
	db.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m1", Content: "first"})
	time.Sleep(50 * time.Millisecond)
	db.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m2", Content: "second"})

	// then - should fire after another 100ms with both messages
	select {
	case msgs := <-fired:
		a.Len(msgs, 2)
	case <-time.After(300 * time.Millisecond):
		t.Fatal("callback not fired")
	}
}

func TestDebouncedBuffer_IndependentChannelTimers(t *testing.T) {
	r := require.New(t)

	// given
	fired := make(chan string, 2)
	callback := func(channelID string, msgs []BufferedMessage) {
		fired <- channelID
	}
	db := NewDebouncedBuffer(50*time.Millisecond, callback)
	defer db.Stop()

	// when
	db.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m1", Content: "a"})
	time.Sleep(30 * time.Millisecond)
	db.Add(BufferedMessage{ChannelID: "chan-2", MessageID: "m2", Content: "b"})

	// then - chan-1 fires first, then chan-2
	first := <-fired
	r.Equal("chan-1", first)

	second := <-fired
	r.Equal("chan-2", second)
}

func TestDebouncedBuffer_ClearsAfterFiring(t *testing.T) {
	a := assert.New(t)

	// given
	firedCount := 0
	callback := func(channelID string, msgs []BufferedMessage) {
		firedCount++
		a.Len(msgs, 1)
	}
	db := NewDebouncedBuffer(30*time.Millisecond, callback)
	defer db.Stop()

	// when - add message, wait for fire, add another
	db.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m1", Content: "first"})
	time.Sleep(100 * time.Millisecond)
	db.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m2", Content: "second"})
	time.Sleep(100 * time.Millisecond)

	// then - should have fired twice, each time with 1 message
	a.Equal(2, firedCount)
}

func TestDebouncedBuffer_ClearChannel_CancelsTimer(t *testing.T) {
	// given
	fired := make(chan bool, 1)
	callback := func(channelID string, msgs []BufferedMessage) {
		fired <- true
	}
	db := NewDebouncedBuffer(50*time.Millisecond, callback)
	defer db.Stop()

	// when
	db.Add(BufferedMessage{ChannelID: "chan-1", MessageID: "m1", Content: "test"})
	db.ClearChannel("chan-1")

	// then - should not fire
	select {
	case <-fired:
		t.Fatal("callback should not have fired")
	case <-time.After(100 * time.Millisecond):
		// expected
	}
}
