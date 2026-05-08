package dashboard

import (
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
	if s.lastSessionID != backend.SessionID() {
		s.lastSessionID = backend.SessionID()
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

	s.chatCallback(backend.SessionID(), content)
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

	active := true
	s.hub.Broadcast(Message{
		Type:      "session",
		Active:    &active,
		SessionID: backend.SessionID(),
	})
}
