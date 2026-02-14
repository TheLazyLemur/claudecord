package core

// ToolInput represents typed tool input fields shared across all tools.
// Replaces map[string]any for compile-time safety.
type ToolInput struct {
	FilePath  string            `json:"file_path,omitempty"`
	Path      string            `json:"path,omitempty"`
	Directory string            `json:"directory,omitempty"`
	Command   string            `json:"command,omitempty"`
	URL       string            `json:"url,omitempty"`
	Method    string            `json:"method,omitempty"`
	Body      string            `json:"body,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Emoji     string            `json:"emoji,omitempty"`
	Message   string            `json:"message,omitempty"`
	Query     string            `json:"query,omitempty"`
	Name      string            `json:"name,omitempty"`
}
