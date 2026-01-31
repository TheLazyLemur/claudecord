package core

// ToolDef defines a tool that can be used by both CLI (MCP) and API modes
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// DiscordTools returns tool definitions for Discord interaction
func DiscordTools() []ToolDef {
	return []ToolDef{
		{
			Name:        "react_emoji",
			Description: "Add emoji reaction to current Discord message. Call this first when you receive a message.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"emoji": map[string]any{
						"type":        "string",
						"description": "Unicode emoji character (e.g. 👀, 👍, 🚀)",
					},
				},
				"required": []string{"emoji"},
			},
		},
		{
			Name:        "send_update",
			Description: "Send a progress update message to a thread on the original Discord message. Use this to keep the user informed about what you're doing.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{
						"type":        "string",
						"description": "The update message to send",
					},
				},
				"required": []string{"message"},
			},
		},
	}
}

// FileTools returns tool definitions for file/shell operations (API mode only)
func FileTools() []ToolDef {
	return []ToolDef{
		{
			Name:        "Read",
			Description: "Read file contents",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{
						"type":        "string",
						"description": "Absolute path to the file",
					},
				},
				"required": []string{"file_path"},
			},
		},
		{
			Name:        "Bash",
			Description: "Execute a bash command",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command": map[string]any{
						"type":        "string",
						"description": "The command to execute",
					},
				},
				"required": []string{"command"},
			},
		},
	}
}

// ToMCP converts a ToolDef to MCP format for CLI mode
func (t ToolDef) ToMCP() map[string]any {
	return map[string]any{
		"name":        t.Name,
		"description": t.Description,
		"inputSchema": t.InputSchema,
	}
}

// SkillTools returns tool definitions for skill activation
func SkillTools() []ToolDef {
	return []ToolDef{
		{
			Name:        "Skill",
			Description: "Load a skill's full instructions. Call when task matches a skill description from available_skills.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Skill name from available_skills list",
					},
				},
				"required": []string{"name"},
			},
		},
		{
			Name:        "LoadSkillSupporting",
			Description: "Load a supporting file from a skill (scripts/, references/, assets/).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type":        "string",
						"description": "Skill name",
					},
					"path": map[string]any{
						"type":        "string",
						"description": "Relative path within skill (e.g. references/API.md, scripts/run.sh)",
					},
				},
				"required": []string{"name", "path"},
			},
		},
	}
}

// MCPTools returns MCP-formatted tools for CLI mode (Discord + Skill tools)
var MCPTools = func() []map[string]any {
	tools := append(DiscordTools(), SkillTools()...)
	result := make([]map[string]any, len(tools))
	for i, t := range tools {
		result[i] = t.ToMCP()
	}
	return result
}()
