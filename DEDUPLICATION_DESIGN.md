# Tool Execution Deduplication Design

## Current State Analysis

### CLI Backend (`internal/cli/backend.go`)
- Receives tool calls via MCP protocol over stdio
- Returns results wrapped in MCP JSON-RPC format
- Has helper methods: `sendMCPResult()`, `sendMCPToolError()`
- Tools: react_emoji, send_update, Skill, LoadSkillSupporting

### API Backend (`internal/api/executors.go` + `internal/api/backend.go`)
- Receives tool calls via Anthropic SDK `ToolUseBlock`
- Returns results as `anthropic.ToolResultBlockParam`
- Has function-based execution: `executeToolByName()` returns `(string, bool)`
- Tools: react_emoji, send_update, Skill, LoadSkillSupporting, Read, Bash

### Key Differences

| Aspect | CLI | API |
|--------|-----|-----|
| Input format | `map[string]any` from JSON | `map[string]any` from `json.Unmarshal(tu.Input)` |
| Output format | `map[string]any` MCP wrapper | `(string, bool)` tuple |
| Error handling | `sendMCPToolError()` | Return error string + `isError=true` |
| Success handling | `sendMCPResult()` with content array | Return result string + `isError=false` |
| File/Shell tools | Handled by CLI process itself | Executed directly in `executors.go` |

## Deduplication Strategy

### Option 1: Core Tool Interface with Adapters (Recommended)

Create a unified tool execution layer in `internal/core/tools.go`:

```go
// internal/core/tools.go additions

// ToolResult is the unified result format
type ToolResult struct {
    Content string
    IsError bool
}

// ToolExecutor executes a tool with unified interface
type ToolExecutor interface {
    Execute(input map[string]any, responder Responder, store skills.SkillStore) ToolResult
}

// Registry of shared tool executors
var SharedToolExecutors = map[string]ToolExecutor{
    "react_emoji":       &reactEmojiExecutor{},
    "send_update":       &sendUpdateExecutor{},
    "Skill":             &skillExecutor{},
    "LoadSkillSupporting": &loadSkillSupportingExecutor{},
}

// Example executor implementation
type reactEmojiExecutor struct{}

func (e *reactEmojiExecutor) Execute(input map[string]any, responder Responder, store skills.SkillStore) ToolResult {
    emoji, ok := input["emoji"].(string)
    if !ok || emoji == "" {
        return ToolResult{Content: "missing emoji argument", IsError: true}
    }
    slog.Info("AddReaction", "emoji", emoji)
    if err := responder.AddReaction(emoji); err != nil {
        slog.Error("AddReaction failed", "error", err)
        return ToolResult{Content: err.Error(), IsError: true}
    }
    return ToolResult{Content: "reaction added", IsError: false}
}
```

**CLI Adapter:**
```go
// internal/cli/backend.go
func (b *Backend) handleMCPToolCall(requestID string, jsonrpcID any, params map[string]any, responder Responder, store skills.SkillStore) error {
    toolName, _ := params["name"].(string)
    args, _ := params["arguments"].(map[string]any)

    // Check shared executors first
    if executor, ok := core.SharedToolExecutors[toolName]; ok {
        result := executor.Execute(args, responder, store)
        if result.IsError {
            return b.sendMCPToolError(requestID, jsonrpcID, result.Content)
        }
        return b.sendMCPResult(requestID, jsonrpcID, map[string]any{
            "content": []map[string]any{{"type": "text", "text": result.Content}},
        })
    }

    return b.sendMCPToolError(requestID, jsonrpcID, "unknown tool: "+toolName)
}
```

**API Adapter:**
```go
// internal/api/backend.go
func (b *Backend) executeTools(ctx context.Context, toolUses []anthropic.ToolUseBlock, responder core.Responder, perms core.PermissionChecker, store skills.SkillStore) ([]anthropic.ContentBlockParamUnion, error) {
    var results []anthropic.ContentBlockParamUnion

    for _, tu := range toolUses {
        // ... permission check ...

        var result string
        var isError bool

        // Check shared executors first
        if executor, ok := core.SharedToolExecutors[tu.Name]; ok {
            r := executor.Execute(input, responder, store)
            result = r.Content
            isError = r.IsError
        } else {
            // Fall back to api-specific executors (Read, Bash)
            result, isError = executeToolByName(tu.Name, input, responder, store)
        }

        results = append(results, anthropic.NewToolResultBlock(tu.ID, result, isError))
    }

    return results, nil
}
```

### Option 2: Functional Approach with Result Wrappers

Instead of interface, use functions with result wrapper:

```go
// internal/core/toolfuncs.go

type ToolFunc func(input map[string]any, responder Responder, store skills.SkillStore) (content string, isError bool)

var SharedToolFuncs = map[string]ToolFunc{
    "react_emoji": func(input map[string]any, responder Responder, store skills.SkillStore) (string, bool) {
        emoji, ok := input["emoji"].(string)
        if !ok || emoji == "" {
            return "missing emoji argument", true
        }
        if err := responder.AddReaction(emoji); err != nil {
            return err.Error(), true
        }
        return "reaction added", false
    },
    // ... other tools
}
```

### Option 3: Code Generation (Overkill)

Generate both CLI and API adapters from a single source of truth. Probably overkill for 4 shared tools.

## Recommendation: Option 1 (Interface-based)

**Pros:**
- Clean separation of concerns
- Easy to test (mock Responder, SkillStore)
- Can add stateful executors later if needed
- Type-safe

**Cons:**
- Slightly more boilerplate than functional approach

**Migration Path:**
1. Create `internal/core/tool_executors.go` with interface and shared implementations
2. Refactor CLI backend to use shared executors
3. Refactor API backend to use shared executors
4. Keep API-specific tools (Read, Bash) in `internal/api/executors.go`
5. Add comprehensive tests for shared executors

## Backward Compatibility

**100% backward compatible** - this is purely internal refactoring:
- No changes to external interfaces (Backend, Responder, etc.)
- No changes to tool definitions (ToolDef in tools.go)
- No changes to behavior - same inputs produce same outputs
- Both backends continue to work independently

## Testing Strategy

```go
// internal/core/tool_executors_test.go
func TestReactEmojiExecutor(t *testing.T) {
    mockResponder := &mockResponder{}
    executor := &reactEmojiExecutor{}
    
    result := executor.Execute(map[string]any{"emoji": "👍"}, mockResponder, nil)
    
    assert.False(t, result.IsError)
    assert.Equal(t, "reaction added", result.Content)
    assert.Equal(t, "👍", mockResponder.lastReaction)
}
```

## File Structure After Refactoring

```
internal/
├── core/
│   ├── tools.go              # ToolDef, MCPTools (existing)
│   ├── tool_executors.go     # NEW: Shared executor interface + implementations
│   └── tool_executors_test.go # NEW: Tests for shared executors
├── cli/
│   └── backend.go            # Uses core.SharedToolExecutors
└── api/
    ├── backend.go            # Uses core.SharedToolExecutors
    └── executors.go          # API-specific tools only (Read, Bash)
```

## Open Questions

1. Should Read and Bash eventually be shared too? (Currently CLI handles them internally)
2. Should executors have access to context.Context for cancellation/timeouts?
3. Should we pass a logger interface instead of using global slog?
