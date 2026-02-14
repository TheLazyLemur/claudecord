package cli

import (
	"encoding/json"

	"github.com/TheLazyLemur/claudecord/internal/core"
)

// Outbound messages

type UserMessage struct {
	Type      string           `json:"type"`
	SessionID string           `json:"session_id"`
	Message   UserMessageInner `json:"message"`
}

type UserMessageInner struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ControlRequest struct {
	Type      string              `json:"type"`
	RequestID string              `json:"request_id"`
	Request   ControlRequestInner `json:"request"`
}

type ControlRequestInner struct {
	Subtype       string   `json:"subtype"`
	SystemPrompt  string   `json:"systemPrompt,omitempty"`
	SDKMcpServers []string `json:"sdkMcpServers,omitempty"`
}

type ControlResponse struct {
	Type     string                `json:"type"`
	Response ControlResponseInner  `json:"response"`
}

type ControlResponseInner struct {
	Subtype   string `json:"subtype"`
	RequestID string `json:"request_id"`
	Response  any    `json:"response"`
}

type PermissionAllow struct {
	Behavior     string         `json:"behavior"`
	ToolUseID    string         `json:"toolUseID"`
	UpdatedInput core.ToolInput `json:"updatedInput"`
}

type PermissionDeny struct {
	Behavior  string `json:"behavior"`
	ToolUseID string `json:"toolUseID"`
	Message   string `json:"message"`
	Interrupt bool   `json:"interrupt"`
}

// MCP response wrappers

type MCPResponseWrapper struct {
	Type     string                    `json:"type"`
	Response MCPResponseWrapperInner   `json:"response"`
}

type MCPResponseWrapperInner struct {
	Subtype   string      `json:"subtype"`
	RequestID string      `json:"request_id"`
	Response  MCPResponse `json:"response"`
}

type MCPResponse struct {
	MCPResponse MCPJSONRPCResponse `json:"mcp_response"`
}

type MCPJSONRPCResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result"`
}

type MCPInitResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    MCPCapability  `json:"capabilities"`
	ServerInfo      MCPServerInfo  `json:"serverInfo"`
}

type MCPCapability struct {
	Tools map[string]any `json:"tools"`
}

type MCPServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type MCPToolListResult struct {
	Tools []core.MCPTool `json:"tools"`
}

type MCPToolCallResult struct {
	Content []MCPTextContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

type MCPTextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Inbound messages

type CLIMessage struct {
	Type      string          `json:"type"`
	Subtype   string          `json:"subtype,omitempty"`
	IsReplay  bool            `json:"isReplay,omitempty"`
	SessionID string          `json:"session_id,omitempty"`
	RequestID string          `json:"request_id,omitempty"`
	Message   json.RawMessage `json:"message,omitempty"`
	Request   json.RawMessage `json:"request,omitempty"`
	Response  json.RawMessage `json:"response,omitempty"`
}

type AssistantMessage struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type CanUseToolRequest struct {
	Subtype   string          `json:"subtype"`
	ToolName  string          `json:"tool_name"`
	ToolUseID string          `json:"tool_use_id"`
	Input     json.RawMessage `json:"input"`
}

type MCPMessageRequest struct {
	Subtype    string        `json:"subtype"`
	ServerName string        `json:"server_name"`
	Message    MCPRPCMessage `json:"message"`
}

type MCPRPCMessage struct {
	ID     any             `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type MCPToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}
