package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/channels/dashboard"
	"github.com/TheLazyLemur/claudecord/internal/config"
	"github.com/TheLazyLemur/claudecord/internal/core"
	dash "github.com/TheLazyLemur/claudecord/internal/dashboard"
	"github.com/TheLazyLemur/claudecord/internal/handler"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/pkg/errors"
)

// startHTTPServer mounts the webhook handler and the dashboard on a single
// http.Server and starts listening. Returns a cleanup that performs a graceful
// shutdown.
func startHTTPServer(
	cfg *config.Config,
	hub *dash.Hub,
	bot *core.Bot,
	sessionMgr *core.SessionManager,
	perms core.PermissionChecker,
	skillStore skills.SkillStore,
	skillsDir string,
) (func(), error) {
	plug := dashboard.New(dashboard.Config{Hub: hub})
	if err := plug.Start(context.Background(), func(in core.Inbound) {
		if err := bot.HandleInbound(in); err != nil {
			slog.Error("dashboard inbound", "error", err)
		}
	}); err != nil {
		return nil, errors.Wrap(err, "start dashboard plugin")
	}

	dashboardServer := dash.NewServer(hub, sessionMgr, perms, skillStore, skillsDir, cfg.ClaudeCWD, cfg.AgentsDefaultPath, cfg.MemoryDir, cfg.DashboardPassword, plug.HandleChat)

	mux := http.NewServeMux()
	mux.Handle("/webhook", handler.NewWebhookHandler())
	mux.Handle("/", dashboardServer.Handler())
	srv := &http.Server{Addr: ":" + cfg.WebhookPort, Handler: mux}

	go func() {
		slog.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
		}
	}()

	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}, nil
}
