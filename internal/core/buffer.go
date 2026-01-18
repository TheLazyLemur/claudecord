package core

import (
	"sync"
	"time"
)

type BufferedMessage struct {
	ChannelID string
	MessageID string
	Content   string
	AuthorID  string
}

type MessageBuffer struct {
	mu       sync.RWMutex
	messages map[string][]BufferedMessage
}

func NewMessageBuffer() *MessageBuffer {
	return &MessageBuffer{
		messages: make(map[string][]BufferedMessage),
	}
}

func (b *MessageBuffer) Add(msg BufferedMessage) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages[msg.ChannelID] = append(b.messages[msg.ChannelID], msg)
}

func (b *MessageBuffer) Get(channelID string) []BufferedMessage {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.messages[channelID]
}

func (b *MessageBuffer) Clear(channelID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.messages, channelID)
}

type BufferCallback func(channelID string, msgs []BufferedMessage)

type DebouncedBuffer struct {
	buffer   *MessageBuffer
	delay    time.Duration
	callback BufferCallback
	timers   map[string]*time.Timer
	mu       sync.Mutex
	stopped  bool
}

func NewDebouncedBuffer(delay time.Duration, callback BufferCallback) *DebouncedBuffer {
	return &DebouncedBuffer{
		buffer:   NewMessageBuffer(),
		delay:    delay,
		callback: callback,
		timers:   make(map[string]*time.Timer),
	}
}

func (d *DebouncedBuffer) Add(msg BufferedMessage) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.stopped {
		return
	}

	d.buffer.Add(msg)
	channelID := msg.ChannelID

	if timer, exists := d.timers[channelID]; exists {
		timer.Stop()
	}

	d.timers[channelID] = time.AfterFunc(d.delay, func() {
		d.fire(channelID)
	})
}

func (d *DebouncedBuffer) fire(channelID string) {
	d.mu.Lock()
	msgs := d.buffer.Get(channelID)
	if len(msgs) == 0 {
		d.mu.Unlock()
		return
	}
	msgsCopy := make([]BufferedMessage, len(msgs))
	copy(msgsCopy, msgs)
	d.buffer.Clear(channelID)
	delete(d.timers, channelID)
	d.mu.Unlock()

	d.callback(channelID, msgsCopy)
}

func (d *DebouncedBuffer) ClearChannel(channelID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if timer, exists := d.timers[channelID]; exists {
		timer.Stop()
		delete(d.timers, channelID)
	}
	d.buffer.Clear(channelID)
}

func (d *DebouncedBuffer) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.stopped = true
	for _, timer := range d.timers {
		timer.Stop()
	}
	d.timers = make(map[string]*time.Timer)
}
