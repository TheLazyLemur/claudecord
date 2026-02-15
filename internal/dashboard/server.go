package dashboard

import (
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/TheLazyLemur/claudecord/internal/core"
	"github.com/TheLazyLemur/claudecord/internal/skills"
	"github.com/gorilla/websocket"
)

//go:embed static/*
var staticFiles embed.FS

const sessionCookieName = "claudecord_session"

// Server handles the dashboard HTTP and WS endpoints.
type Server struct {
	hub            *Hub
	upgrader       websocket.Upgrader
	sessionMgr     *core.SessionManager
	permChecker    core.PermissionChecker
	skillStore     skills.SkillStore
	skillsDir      string
	password       string

	mu        sync.Mutex
	responder *WSResponder
	sessions  map[string]time.Time // valid session tokens
}

// NewServer creates a dashboard server.
func NewServer(hub *Hub, sessionMgr *core.SessionManager, permChecker core.PermissionChecker, skillStore skills.SkillStore, skillsDir, password string) *Server {
	return &Server{
		hub:         hub,
		sessionMgr:  sessionMgr,
		permChecker: permChecker,
		skillStore:  skillStore,
		skillsDir:   skillsDir,
		password:    password,
		sessions:    make(map[string]time.Time),
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

// Handler returns the HTTP handler for the dashboard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// If no password set, return 403 for all dashboard routes
	if s.password == "" {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Dashboard disabled (no password set)", http.StatusForbidden)
		})
		return mux
	}

	// Login routes (no auth required)
	mux.HandleFunc("/login", s.handleLogin)

	// Static files (protected)
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/static/", s.requireAuth(http.StripPrefix("/static/", http.FileServer(http.FS(staticFS)))))

	// Index (protected)
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

	// QR code (protected) — returns current QR or 204
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

	// WebSocket (protected)
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		s.handleWS(w, r)
	})

	return mux
}

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

		// Generate session token
		token := s.createSession()
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   86400 * 7, // 7 days
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

func (s *Server) handleMessage(client *Client, msg Message) {
	switch msg.Type {
	case "chat":
		go s.handleChat(msg.Content)

	case "new_session":
		go s.handleNewSession(msg.WorkDir)

	case "permission_response":
		s.handlePermissionResponse(msg.ID, msg.Approved)

	case "get_skills":
		s.handleGetSkills(client)

	case "get_skill":
		s.handleGetSkill(client, msg.Name)

	case "save_skill":
		s.handleSaveSkill(client, msg)

	case "delete_skill_file":
		s.handleDeleteSkillFile(client, msg.Name, msg.Path)
	}
}

func (s *Server) handleChat(content string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	backend, err := s.sessionMgr.GetOrCreateSession()
	if err != nil {
		slog.Error("get session", "error", err)
		s.hub.Broadcast(Message{
			Type:    "chat",
			Role:    "assistant",
			Content: "Error: " + err.Error(),
		})
		return
	}

	// Create/update responder for this session
	if s.responder == nil || s.responder.sessionID != backend.SessionID() {
		s.responder = NewWSResponder(s.hub, backend.SessionID())

		active := true
		s.hub.Broadcast(Message{
			Type:      "session",
			Active:    &active,
			SessionID: backend.SessionID(),
		})
	}

	responder := s.responder

	// Broadcast user message
	s.hub.Broadcast(Message{
		Type:    "chat",
		Role:    "user",
		Content: content,
	})

	// Converse — lock held to prevent handleNewSession from closing backend mid-conversation
	ctx := context.Background()
	response, err := backend.Converse(ctx, content, responder, s.permChecker)
	if err != nil {
		slog.Error("converse", "error", err)
		s.hub.Broadcast(Message{
			Type:    "chat",
			Role:    "assistant",
			Content: "Error: " + err.Error(),
		})
		return
	}

	if response != "" {
		s.hub.Broadcast(Message{
			Type:      "chat",
			Role:      "assistant",
			Content:   response,
			SessionID: backend.SessionID(),
		})
	}
}

func (s *Server) handleNewSession(workDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.sessionMgr.NewSession(workDir); err != nil {
		slog.Error("create session", "error", err)
		s.hub.Broadcast(Message{
			Type:    "chat",
			Role:    "assistant",
			Content: "Error creating session: " + err.Error(),
		})
		return
	}

	backend, _ := s.sessionMgr.GetOrCreateSession()
	s.responder = NewWSResponder(s.hub, backend.SessionID())

	active := true
	s.hub.Broadcast(Message{
		Type:      "session",
		Active:    &active,
		SessionID: backend.SessionID(),
	})
}

func (s *Server) handlePermissionResponse(id string, approved *bool) {
	s.mu.Lock()
	responder := s.responder
	s.mu.Unlock()

	if responder != nil && approved != nil {
		responder.HandlePermissionResponse(id, *approved)
	}
}

func (s *Server) handleGetSkills(client *Client) {
	skillList, err := s.skillStore.List()
	if err != nil {
		slog.Error("list skills", "error", err)
		return
	}

	var infos []SkillInfo
	for _, sk := range skillList {
		infos = append(infos, SkillInfo{
			Name:        sk.Name,
			Description: sk.Description,
		})
	}

	client.Send(Message{
		Type:   "skills",
		Skills: infos,
	})
}

func (s *Server) handleGetSkill(client *Client, name string) {
	skill, err := s.skillStore.Load(name)
	if err != nil {
		slog.Error("load skill", "error", err, "name", name)
		return
	}

	// Get supporting files
	skillDir := filepath.Join(s.skillsDir, name)
	var files []SkillFile

	// Walk subdirectories
	for _, subdir := range []string{"scripts", "references", "assets"} {
		dir := filepath.Join(skillDir, subdir)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, SkillFile{
				Path: filepath.Join(subdir, e.Name()),
				Size: info.Size(),
			})
		}
	}

	// Full skill content with frontmatter
	content := formatSkillContent(skill)

	client.Send(Message{
		Type:    "skill_detail",
		Name:    name,
		Content: content,
		Files:   files,
	})
}

func formatSkillContent(skill *skills.Skill) string {
	return "---\nname: " + skill.Name + "\ndescription: " + skill.Description + "\n---\n" + skill.Instructions
}

func (s *Server) handleSaveSkill(client *Client, msg Message) {
	if msg.Name == "" || msg.Content == "" {
		return
	}

	// Validate name
	if strings.Contains(msg.Name, "..") || strings.Contains(msg.Name, "/") {
		slog.Error("invalid skill name", "name", msg.Name)
		return
	}

	skillDir := filepath.Join(s.skillsDir, msg.Name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		slog.Error("create skill dir", "error", err)
		return
	}

	// Write SKILL.md
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(msg.Content), 0644); err != nil {
		slog.Error("write skill", "error", err)
		return
	}

	// Write supporting files
	for _, f := range msg.Files {
		if err := validateRelativePath(f.Path); err != nil {
			continue
		}
		filePath := filepath.Join(skillDir, f.Path)
		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			continue
		}
		if err := os.WriteFile(filePath, []byte(f.Content), 0644); err != nil {
			slog.Error("write skill file", "error", err, "path", f.Path)
		}
	}

	slog.Info("skill saved", "name", msg.Name)

	// Refresh skill list
	s.handleGetSkills(client)
}

func (s *Server) handleDeleteSkillFile(client *Client, name, path string) {
	if name == "" || path == "" {
		return
	}

	if err := validateRelativePath(path); err != nil {
		slog.Error("invalid path", "error", err)
		return
	}

	filePath := filepath.Join(s.skillsDir, name, path)

	// Ensure within skill dir
	absPath, _ := filepath.Abs(filePath)
	skillDir, _ := filepath.Abs(filepath.Join(s.skillsDir, name))
	if !strings.HasPrefix(absPath, skillDir+string(filepath.Separator)) {
		slog.Error("path escape attempt", "path", path)
		return
	}

	if err := os.Remove(filePath); err != nil {
		slog.Error("delete file", "error", err)
		return
	}

	slog.Info("skill file deleted", "name", name, "path", path)

	// Refresh skill detail
	s.handleGetSkill(client, name)
}

func validateRelativePath(p string) error {
	if filepath.IsAbs(p) {
		return os.ErrInvalid
	}
	if strings.Contains(p, "..") {
		return os.ErrInvalid
	}
	return nil
}
