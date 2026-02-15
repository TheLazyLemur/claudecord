package dashboard

import (
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512 * 1024
)

// Message represents a WS message.
type Message struct {
	Type    string `json:"type"`
	Level   string `json:"level,omitempty"`
	Msg     string `json:"msg,omitempty"`
	Time    string `json:"time,omitempty"`
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
	Active  *bool  `json:"active,omitempty"`
	ID      string `json:"id,omitempty"`
	Prompt  string `json:"prompt,omitempty"`

	// Session info
	SessionID string `json:"sessionID,omitempty"`

	// Skills
	Skills []SkillInfo  `json:"skills,omitempty"`
	Name   string       `json:"name,omitempty"`
	Files  []SkillFile  `json:"files,omitempty"`
	Path   string       `json:"path,omitempty"`

	// Permission response
	Approved *bool `json:"approved,omitempty"`

	// New session
	WorkDir string `json:"workDir,omitempty"`
}

// SkillInfo for skill list.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// SkillFile for skill detail.
type SkillFile struct {
	Path    string `json:"path"`
	Size    int64  `json:"size,omitempty"`
	Content string `json:"content,omitempty"`
}

// Client represents a connected WS client.
type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
}

// Hub manages WS clients and broadcasts.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
	sticky     []byte // last sticky message, replayed to new clients
}

// NewHub creates a new Hub.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's event loop.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			sticky := h.sticky
			h.mu.Unlock()
			if sticky != nil {
				select {
				case client.send <- sticky:
				default:
				}
			}
			slog.Debug("ws client registered", "clients", len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
			slog.Debug("ws client unregistered", "clients", len(h.clients))

		case msg := <-h.broadcast:
			h.mu.RLock()
			var slow []*Client
			for client := range h.clients {
				select {
				case client.send <- msg:
				default:
					slow = append(slow, client)
				}
			}
			h.mu.RUnlock()

			if len(slow) > 0 {
				h.mu.Lock()
				for _, c := range slow {
					if _, ok := h.clients[c]; ok {
						delete(h.clients, c)
						close(c.send)
					}
				}
				h.mu.Unlock()
			}
		}
	}
}

// Broadcast sends a message to all clients.
func (h *Hub) Broadcast(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("marshal broadcast", "error", err)
		return
	}
	h.broadcast <- data
}

// BroadcastSticky caches the message and broadcasts it. Late-joining clients receive the cached copy.
func (h *Hub) BroadcastSticky(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("marshal broadcast", "error", err)
		return
	}
	h.mu.Lock()
	h.sticky = data
	h.mu.Unlock()
	h.broadcast <- data
}

// ClearSticky removes the cached sticky message.
func (h *Hub) ClearSticky() {
	h.mu.Lock()
	h.sticky = nil
	h.mu.Unlock()
}

// BroadcastRaw sends raw bytes to all clients.
func (h *Hub) BroadcastRaw(data []byte) {
	h.broadcast <- data
}

// readPump pumps messages from WS to hub.
func (c *Client) readPump(handler func(*Client, Message)) {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Error("ws read error", "error", err)
			}
			break
		}

		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			slog.Error("ws unmarshal", "error", err)
			continue
		}

		handler(c, msg)
	}
}

// writePump pumps messages from hub to WS.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case msg, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(msg)

			// Drain queued messages
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte("\n"))
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Send sends a message to this client.
func (c *Client) Send(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}
	select {
	case c.send <- data:
	default:
	}
}
