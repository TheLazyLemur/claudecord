package dashboard

import (
	"log/slog"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

func (s *Server) handleGetAgentsMd(client *Client) {
	content, err := core.ReadAgentsMd(s.workDir)
	if err != nil {
		slog.Error("read AGENTS.md", "error", err)
		client.Send(Message{Type: "agents_md", Content: "", Msg: err.Error()})
		return
	}
	client.Send(Message{Type: "agents_md", Content: content})
}

func (s *Server) handleSaveAgentsMd(client *Client, content string) {
	if err := core.WriteAgentsMd(s.workDir, content); err != nil {
		slog.Error("write AGENTS.md", "error", err)
		client.Send(Message{Type: "agents_md", Content: content, Msg: err.Error()})
		return
	}
	slog.Info("AGENTS.md saved")
	client.Send(Message{Type: "agents_md", Content: content})
}

func (s *Server) handleResetAgentsMd(client *Client) {
	if err := core.ResetAgentsMd(s.workDir, s.agentsDefaultPath); err != nil {
		slog.Error("reset AGENTS.md", "error", err)
		client.Send(Message{Type: "agents_md", Msg: err.Error()})
		return
	}
	slog.Info("AGENTS.md reset to default")
	s.handleGetAgentsMd(client)
}
