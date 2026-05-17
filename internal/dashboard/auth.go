package dashboard

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"
)

const sessionCookieName = "switchboard_session"

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		data, err := staticFiles.ReadFile("static/login.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		if r.FormValue("password") != s.password {
			http.Error(w, "invalid password", http.StatusUnauthorized)
			return
		}

		token := s.createSession()
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 7,
		})
		w.WriteHeader(http.StatusOK)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

func (s *Server) createSession() string {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)

	s.mu.Lock()
	s.sessions[token] = time.Now()
	s.mu.Unlock()

	return token
}

func (s *Server) isAuthenticated(r *http.Request) bool {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}

	s.mu.Lock()
	_, valid := s.sessions[cookie.Value]
	s.mu.Unlock()

	return valid
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
