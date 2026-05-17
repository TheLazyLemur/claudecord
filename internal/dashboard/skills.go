package dashboard

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheLazyLemur/switchboard/internal/skills"
)

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

	skillDir := filepath.Join(s.skillsDir, name)
	var files []SkillFile

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

	if strings.Contains(msg.Name, "..") || strings.Contains(msg.Name, "/") {
		slog.Error("invalid skill name", "name", msg.Name)
		return
	}

	skillDir := filepath.Join(s.skillsDir, msg.Name)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		slog.Error("create skill dir", "error", err)
		return
	}

	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte(msg.Content), 0644); err != nil {
		slog.Error("write skill", "error", err)
		return
	}

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
