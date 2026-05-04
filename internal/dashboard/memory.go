package dashboard

import (
	"log/slog"

	"github.com/TheLazyLemur/claudecord/internal/memory"
)

func (s *Server) handleListMemory(client *Client) {
	files, err := memory.List(s.memoryDir)
	if err != nil {
		slog.Error("list memory", "error", err)
		return
	}
	infos := make([]SkillFile, 0, len(files))
	for _, f := range files {
		infos = append(infos, SkillFile{Path: f})
	}
	client.Send(Message{Type: "memory_list", Files: infos})
}

func (s *Server) handleGetMemory(client *Client, path string) {
	content, err := memory.Read(s.memoryDir, path)
	if err != nil {
		slog.Error("read memory", "error", err, "path", path)
		client.Send(Message{Type: "memory_file", Path: path, Msg: err.Error()})
		return
	}
	client.Send(Message{Type: "memory_file", Path: path, Content: content})
}

func (s *Server) handleSaveMemory(client *Client, path, content string) {
	if err := memory.Write(s.memoryDir, path, content); err != nil {
		slog.Error("write memory", "error", err, "path", path)
		client.Send(Message{Type: "memory_file", Path: path, Msg: err.Error()})
		return
	}
	slog.Info("memory saved", "path", path)
	client.Send(Message{Type: "memory_file", Path: path, Content: content})
	s.handleListMemory(client)
}

func (s *Server) handleDeleteMemory(client *Client, path string) {
	if err := memory.Delete(s.memoryDir, path); err != nil {
		slog.Error("delete memory", "error", err, "path", path)
		client.Send(Message{Type: "memory_list", Msg: err.Error()})
		return
	}
	slog.Info("memory deleted", "path", path)
	s.handleListMemory(client)
}
