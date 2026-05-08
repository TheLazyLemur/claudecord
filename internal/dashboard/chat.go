package dashboard

import (
	"context"
	"log/slog"
)

func (s *Server) handleMessage(client *Client, msg Message) {
	switch msg.Type {
	case "chat":
		go s.handleChat(msg.Content)

	case "new_session":
		go s.handleNewSession(msg.WorkDir)

	case "get_skills":
		s.handleGetSkills(client)

	case "get_skill":
		s.handleGetSkill(client, msg.Name)

	case "save_skill":
		s.handleSaveSkill(client, msg)

	case "delete_skill_file":
		s.handleDeleteSkillFile(client, msg.Name, msg.Path)

	case "get_agents_md":
		s.handleGetAgentsMd(client)

	case "save_agents_md":
		s.handleSaveAgentsMd(client, msg.Content)

	case "reset_agents_md":
		s.handleResetAgentsMd(client)

	case "list_memory":
		s.handleListMemory(client)

	case "get_memory":
		s.handleGetMemory(client, msg.Path)

	case "save_memory":
		s.handleSaveMemory(client, msg.Path, msg.Content)

	case "delete_memory":
		s.handleDeleteMemory(client, msg.Path)
	}
}

// handleChat processes an inbound chat message. When a chatCallback is wired
// in, it delegates to the callback (plugin path). Otherwise falls back to the
// legacy direct-dispatch path.
//
// The server mutex protects only the shared responder field; Converse runs
// without the lock so a slow API call does not block other dashboard actions
// like /new_session.
func (s *Server) handleChat(content string) {
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

	s.mu.Lock()
	if s.responder == nil || s.responder.sessionID != backend.SessionID() {
		s.responder = NewWSResponder(s.hub, backend.SessionID())

		active := true
		s.hub.Broadcast(Message{
			Type:      "session",
			Active:    &active,
			SessionID: backend.SessionID(),
		})
	}
	s.mu.Unlock()

	s.hub.Broadcast(Message{
		Type:    "chat",
		Role:    "user",
		Content: content,
	})

	if s.chatCallback != nil {
		s.chatCallback(backend.SessionID(), content)
		return
	}

	// Legacy path: dispatch directly via the session manager.
	s.mu.Lock()
	responder := s.responder
	s.mu.Unlock()

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

	s.mu.Lock()
	s.responder = NewWSResponder(s.hub, backend.SessionID())
	s.mu.Unlock()

	active := true
	s.hub.Broadcast(Message{
		Type:      "session",
		Active:    &active,
		SessionID: backend.SessionID(),
	})
}
