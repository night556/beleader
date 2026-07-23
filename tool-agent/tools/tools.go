package tools

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

// ToolDef is the tool definition sent to Gateway during registration.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ToolResult is returned to Gateway after execution.
type ToolResult struct {
	Content string   `json:"content,omitempty"`
	Error   string   `json:"error,omitempty"`
	Images  []string `json:"images,omitempty"`
}

// ToolHandler executes a tool and returns a result.
type ToolHandler func(args string, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult

var handlers = map[string]ToolHandler{}
var toolDefs []ToolDef

func register(name, description string, params map[string]any, required []string, handler ToolHandler) {
	if params == nil {
		params = map[string]any{"type": "object"}
	} else {
		// Wrap in "type": "object" if not already
		if _, ok := params["type"]; !ok {
			wrapped := map[string]any{"type": "object"}
			for k, v := range params {
				wrapped[k] = v
			}
			params = wrapped
		}
	}
	if len(required) > 0 {
		params["required"] = required
	}
	handlers[name] = handler
	toolDefs = append(toolDefs, ToolDef{
		Name:        name,
		Description: description,
		Parameters:  params,
	})
}

// AllToolDefs returns all registered tool definitions.
func AllToolDefs() []json.RawMessage {
	defs := make([]json.RawMessage, 0, len(toolDefs))
	for _, td := range toolDefs {
		b, _ := json.Marshal(td)
		defs = append(defs, b)
	}
	return defs
}

// GetToolDefs returns tool definitions (for /tools endpoint).
func GetToolDefs() []ToolDef {
	return toolDefs
}

// ExecuteTool runs a tool by name. Returns a ToolResult.
// This is called by the API server.
func ExecuteTool(name, args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	handler, ok := handlers[name]
	if !ok {
		return &ToolResult{Error: fmt.Sprintf("unknown tool: %s", name)}
	}
	return handler(args, workspace, workspaceRoot, restrict, threadID)
}

// resolvePath joins a path with workspace, enforcing restrict if needed.
func resolvePath(p, workspace string, restrict bool) (string, error) {
	if filepath.IsAbs(p) {
		if restrict && workspace != "" {
			clean := filepath.Clean(p)
			cleanWs := filepath.Clean(workspace)
			if !strings.HasPrefix(clean, cleanWs+string(filepath.Separator)) && clean != cleanWs {
				return "", fmt.Errorf("access denied: path is outside workspace (%s)", workspace)
			}
		}
		return p, nil
	}
	if workspace != "" {
		return filepath.Join(workspace, p), nil
	}
	return p, nil
}

// platformString returns the platform string.
func platformString() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}
