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
