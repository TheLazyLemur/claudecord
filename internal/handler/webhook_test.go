package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWebhookHandler_POST_ReturnsOK(t *testing.T) {
	a := assert.New(t)

	h := NewWebhookHandler()
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(`{"test":"data"}`))
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	a.Equal(http.StatusOK, rec.Code)
}

func TestWebhookHandler_GET_ReturnsMethodNotAllowed(t *testing.T) {
	a := assert.New(t)

	h := NewWebhookHandler()
	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	a.Equal(http.StatusMethodNotAllowed, rec.Code)
}
