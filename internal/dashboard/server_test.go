package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServer_NoPassword_Returns403(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "", "")
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestServer_LoginPage_Unauthenticated(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "", "testpass")
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Password")
}

func TestServer_Index_RedirectsToLogin(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "", "testpass")
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/login", rec.Header().Get("Location"))
}

func TestServer_Login_WrongPassword(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "", "testpass")
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=wrong"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestServer_Login_CorrectPassword_SetsCookie(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "", "testpass")
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=testpass"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	cookies := rec.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, sessionCookieName, cookies[0].Name)
	assert.NotEmpty(t, cookies[0].Value)
}

func TestServer_Index_WithValidSession(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "", "testpass")
	handler := s.Handler()

	// Login first
	loginReq := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("password=testpass"))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	cookie := loginRec.Result().Cookies()[0]

	// Access index with session cookie
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "Claudecord Dashboard")
}

func TestServer_WS_Unauthenticated(t *testing.T) {
	s := NewServer(nil, nil, nil, nil, "", "testpass")
	handler := s.Handler()

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}
