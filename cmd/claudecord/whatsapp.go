package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/TheLazyLemur/claudecord/internal/channels/whatsapp"
	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/dashboard"
	"github.com/TheLazyLemur/claudecord/internal/handler"
	"github.com/mdp/qrterminal/v3"
	"github.com/pkg/errors"
	waow "go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

// startWhatsApp connects to WhatsApp, wires the plugin against bot, and
// returns a cleanup func that disconnects and stops the plugin.
func startWhatsApp(cfg *config.Config, hub *dashboard.Hub, bot *core.Bot) (func(), error) {
	container, err := sqlstore.New(context.Background(), "sqlite", "file:"+cfg.WhatsAppDBPath+"?_pragma=foreign_keys(1)", nil)
	if err != nil {
		return nil, errors.Wrap(err, "creating whatsapp store")
	}
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, errors.Wrap(err, "getting whatsapp device")
	}
	waClient := waow.NewClient(device, nil)
	waWrapper := handler.NewWhatsAppClientWrapper(waClient)

	plugin := whatsapp.New(whatsapp.Config{
		Messenger:      waWrapper,
		Downloader:     waWrapper,
		AllowedSenders: cfg.WhatsAppAllowedSenders,
		MediaDir:       cfg.WhatsAppMediaDir,
		NewSession: func() error {
			return bot.NewSession("")
		},
	})

	if err := plugin.Start(context.Background(), func(in core.Inbound) {
		if err := bot.HandleInbound(in); err != nil {
			slog.Error("handling whatsapp inbound", "error", err)
		}
	}); err != nil {
		return nil, errors.Wrap(err, "starting whatsapp plugin")
	}

	waClient.AddEventHandler(plugin.HandleEvent)

	if waClient.Store.ID == nil {
		qrChan, err := waClient.GetQRChannel(context.Background())
		if err != nil {
			_ = plugin.Stop()
			return nil, errors.Wrap(err, "getting whatsapp QR channel")
		}
		if err := waClient.Connect(); err != nil {
			_ = plugin.Stop()
			return nil, errors.Wrap(err, "connecting whatsapp")
		}
		go func() {
			for evt := range qrChan {
				if evt.Event == "code" {
					fmt.Println("Scan this QR code in WhatsApp > Linked Devices:")
					qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
					hub.BroadcastSticky(dashboard.Message{Type: "whatsapp_qr", Content: evt.Code})
				} else {
					slog.Info("whatsapp qr event", "event", evt.Event)
					hub.ClearSticky()
					hub.Broadcast(dashboard.Message{Type: "whatsapp_qr", Content: evt.Event})
				}
			}
		}()
	} else {
		if err := waClient.Connect(); err != nil {
			_ = plugin.Stop()
			return nil, errors.Wrap(err, "connecting whatsapp")
		}
	}
	slog.Info("whatsapp connected")

	cleanup := func() {
		waClient.Disconnect()
		_ = plugin.Stop()
	}
	return cleanup, nil
}
