package core

import (
	"os"
	"path/filepath"
	"strings"
)

// AgentsFileName is the conventional filename read for context injection.
const AgentsFileName = "AGENTS.md"

// LoadAgentsContext reads <workDir>/AGENTS.md and returns it wrapped in
// <agents_md> tags. Returns empty string if workDir is empty, the file is
// missing, or the file is empty.
func LoadAgentsContext(workDir string) string {
	if workDir == "" {
		return ""
	}
	body, err := os.ReadFile(filepath.Join(workDir, AgentsFileName))
	if err != nil {
		return ""
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	return "<agents_md>\n" + trimmed + "\n</agents_md>"
}

// BootstrapAgentsMd seeds <workDir>/AGENTS.md from defaultPath when the file
// is missing. Existing files are left untouched. A missing default or empty
// path is a silent no-op so dev environments without the bundled default
// still boot.
func BootstrapAgentsMd(workDir, defaultPath string) error {
	if workDir == "" || defaultPath == "" {
		return nil
	}
	target := filepath.Join(workDir, AgentsFileName)
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	body, err := os.ReadFile(defaultPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.WriteFile(target, body, 0o644)
}

// ReadAgentsMd returns the raw contents of <workDir>/AGENTS.md, or empty
// string if the file is missing. Errors other than not-found are returned.
func ReadAgentsMd(workDir string) (string, error) {
	if workDir == "" {
		return "", nil
	}
	body, err := os.ReadFile(filepath.Join(workDir, AgentsFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(body), nil
}

// WriteAgentsMd overwrites <workDir>/AGENTS.md with content.
func WriteAgentsMd(workDir, content string) error {
	return os.WriteFile(filepath.Join(workDir, AgentsFileName), []byte(content), 0o644)
}

// ResetAgentsMd overwrites <workDir>/AGENTS.md from defaultPath, even if it
// already exists. Errors if defaultPath is unreadable.
func ResetAgentsMd(workDir, defaultPath string) error {
	body, err := os.ReadFile(defaultPath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workDir, AgentsFileName), body, 0o644)
}

// AppendAgentsContext joins a base system prompt with an AGENTS.md block.
// Either side may be empty.
func AppendAgentsContext(base, agents string) string {
	switch {
	case agents == "":
		return base
	case base == "":
		return agents
	default:
		return base + "\n\n" + agents
	}
}
