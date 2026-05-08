package discord

import (
	"github.com/bwmarrin/discordgo"
	"github.com/pkg/errors"
)

// Connect opens a discordgo session with the message intents we need.
// Caller owns the returned session and must call Close.
func Connect(token string) (*discordgo.Session, error) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, errors.Wrap(err, "creating discord session")
	}
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent | discordgo.IntentDirectMessages
	if err := dg.Open(); err != nil {
		return nil, errors.Wrap(err, "opening discord session")
	}
	return dg, nil
}
