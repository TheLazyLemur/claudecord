package handler

import (
	"log/slog"
	"net/http"
)

type WebhookHandler struct{}

func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{}
}

func (h *WebhookHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	slog.Info("webhook received", "path", r.URL.Path)

	w.WriteHeader(http.StatusOK)
}
