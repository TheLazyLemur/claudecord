package discord

import (
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/pkg/errors"
)

// discordSession is the slice of *discordgo.Session that outbound needs.
// Defined as an interface so tests can mock it.
type discordSession interface {
	ChannelMessageSend(channelID, content string) error
	ChannelTyping(channelID string) error
	MessageReactionAdd(channelID, messageID, emoji string) error
}

type outbound struct {
	s         discordSession
	threadID  string
	messageID string
	maxLen    int
}

func newOutbound(s discordSession, threadID, messageID string, maxLen int) *outbound {
	return &outbound{s: s, threadID: threadID, messageID: messageID, maxLen: maxLen}
}

func (o *outbound) SendTyping() error {
	return errors.Wrap(o.s.ChannelTyping(o.threadID), "discord typing")
}

func (o *outbound) PostResponse(content string) error {
	for _, chunk := range core.ChunkMessage(content, o.maxLen) {
		if err := o.s.ChannelMessageSend(o.threadID, chunk); err != nil {
			return errors.Wrap(err, "discord send")
		}
	}
	return nil
}

func (o *outbound) AddReaction(emoji string) error {
	return errors.Wrap(o.s.MessageReactionAdd(o.threadID, o.messageID, emoji), "discord react")
}

func (o *outbound) SendUpdate(message string) error {
	return errors.Wrap(o.s.ChannelMessageSend(o.threadID, message), "discord update")
}
