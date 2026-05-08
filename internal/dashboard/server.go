package dashboard

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

// Server handles the dashboard HTTP and WS endpoints.
type Server struct {
	hub               *Hub
	upgrader          websocket.Upgrader
	sessionMgr        *core.SessionManager
	permChecker       core.PermissionChecker
	skillStore        skills.SkillStore
	skillsDir         string
	workDir           string
	agentsDefaultPath string
	memoryDir         string
	password          string
	chatCallback      func(sessionID, text string)

	mu            sync.Mutex
	sessions      map[string]time.Time // valid session tokens
	lastSessionID string               // protected by mu
}

// NewServer creates a dashboard server. chatCallback is required; it is invoked
// for each inbound chat message and receives the current session UUID and the
// message text.
func NewServer(hub *Hub, sessionMgr *core.SessionManager, permChecker core.PermissionChecker, skillStore skills.SkillStore, skillsDir, workDir, agentsDefaultPath, memoryDir, password string, chatCallback func(sessionID, text string)) *Server {
	return &Server{
		hub:               hub,
		sessionMgr:        sessionMgr,
		permChecker:       permChecker,
		skillStore:        skillStore,
		skillsDir:         skillsDir,
		workDir:           workDir,
		agentsDefaultPath: agentsDefaultPath,
		memoryDir:         memoryDir,
		password:          password,
		chatCallback:      chatCallback,
		sessions:          make(map[string]time.Time),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return false
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return u.Host == r.Host
			},
		},
	}
}

// SetChatCallback replaces the server's chat callback. The dashboard plugin
// calls this from Start so that it owns the wiring rather than the callsite.
func (s *Server) SetChatCallback(cb func(sessionID, text string)) {
	s.mu.Lock()
	s.chatCallback = cb
	s.mu.Unlock()
}

// Handler returns the HTTP handler for the dashboard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	if s.password == "" {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Dashboard disabled (no password set)", http.StatusForbidden)
		})
		return mux
	}

	mux.HandleFunc("/login", s.handleLogin)

	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/static/", s.requireAuth(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if !s.isAuthenticated(r) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		data, err := staticFiles.ReadFile("static/index.html")
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(data)
	})

	mux.HandleFunc("/api/qr", func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		data := s.hub.Sticky()
		if data == nil {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	})

	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.handleWS(w, r)
	})

	return mux
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("ws upgrade", "error", err)
		return
	}

	client := &Client{
		hub:  s.hub,
		conn: conn,
		send: make(chan []byte, 256),
	}

	s.hub.register <- client

	go client.writePump()
	go client.readPump(s.handleMessage)
}
