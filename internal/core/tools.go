package core

// ToolDef defines a tool sent to the upstream Anthropic-shaped API.
type ToolDef struct {
	Name        string
	Description string
	InputSchema map[string]any
}

// objSchema builds the standard {"type":"object","properties":...,"required":...}
// JSON-schema wrapper so each tool definition stays readable.
func objSchema(props map[string]any, required ...string) map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

// strProp builds a {"type":"string","description":...} property.
func strProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

// ReactEmojiTool is the optional tool registered when the active channel
// reports Capabilities.Reactions == true.
func ReactEmojiTool() ToolDef {
	return ToolDef{
		Name:        "react_emoji",
		Description: "Add an emoji reaction to the message you are responding to.",
		InputSchema: objSchema(map[string]any{
			"emoji": strProp("Unicode emoji character (e.g. 👀, 👍, 🚀)"),
		}, "emoji"),
	}
}

// SendUpdateTool is registered for every channel.
func SendUpdateTool() ToolDef {
	return ToolDef{
		Name:        "send_update",
		Description: "Send a progress update message in the current conversation. Use to keep the user informed about what you're doing.",
		InputSchema: objSchema(map[string]any{
			"message": strProp("The update message to send"),
		}, "message"),
	}
}

// FileTools returns tool definitions for file/shell operations (API mode only)
func FileTools() []ToolDef {
	return []ToolDef{
		{
			Name:        "Read",
			Description: "Read file contents",
			InputSchema: objSchema(map[string]any{
				"file_path": strProp("Absolute path to the file"),
			}, "file_path"),
		},
		{
			Name:        "Bash",
			Description: "Execute a bash command",
			InputSchema: objSchema(map[string]any{
				"command": strProp("The command to execute"),
			}, "command"),
		},
		{
			Name:        "Fetch",
			Description: "Make HTTP request. GET auto-approved, POST/PATCH/DELETE require approval.",
			InputSchema: objSchema(map[string]any{
				"url": strProp("URL to fetch"),
				"method": map[string]any{
					"type":        "string",
					"enum":        []string{"GET", "POST", "PATCH", "DELETE"},
					"default":     "GET",
					"description": "HTTP method",
				},
				"body":    strProp("Request body (for POST/PATCH)"),
				"headers": map[string]any{"type": "object", "description": "Request headers"},
			}, "url"),
		},
		{
			Name:        "WebSearch",
			Description: "Search the web. Returns titles, links, snippets. Use 3-5 keywords.",
			InputSchema: objSchema(map[string]any{
				"query": strProp("Search query (3-5 keywords)"),
			}, "query"),
		},
	}
}

// SkillTools returns tool definitions for skill activation
func SkillTools() []ToolDef {
	return []ToolDef{
		{
			Name:        "Skill",
			Description: "Load a skill's full instructions. Call when task matches a skill description from available_skills.",
			InputSchema: objSchema(map[string]any{
				"name": strProp("Skill name from available_skills list"),
			}, "name"),
		},
		{
			Name:        "LoadSkillSupporting",
			Description: "Load a supporting file from a skill (scripts/, references/, assets/).",
			InputSchema: objSchema(map[string]any{
				"name": strProp("Skill name"),
				"path": strProp("Relative path within skill (e.g. references/API.md, scripts/run.sh)"),
			}, "name", "path"),
		},
	}
}
