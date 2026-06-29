package tools

import (
	"context"
	"encoding/json"
	"sync"

	"beleader/backend/session"

	"github.com/sashabaranov/go-openai"
)

// ToolEntry describes a tool in the global registry.
type ToolEntry struct {
	Name        string
	Definition  openai.Tool
	Handler    func(ctx context.Context, args string) *session.ToolResult
	Description string // for UI tool picker
}

// ExposedTool is a lightweight tool summary for UI and prompt listing.
type ExposedTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// Registry is the global tool registry singleton.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*ToolEntry
}

// NewRegistry creates a new tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]*ToolEntry),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(name string, def openai.Tool, handler func(ctx context.Context, args string) *session.ToolResult, desc string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = &ToolEntry{
		Name:        name,
		Definition:  def,
		Handler:    handler,
		Description: desc,
	}
}

// GetTool returns a tool entry by name.
func (r *Registry) GetTool(name string) (*ToolEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	return t, ok
}

// GetToolDef returns a tool definition by name.
func (r *Registry) GetToolDef(name string) (openai.Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.tools[name]
	if !ok {
		return openai.Tool{}, false
	}
	return t.Definition, true
}

// ListExposed returns lightweight tool summaries for UI and prompt listing.
func (r *Registry) ListExposed() []ExposedTool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []ExposedTool
	for _, t := range r.tools {
		et := ExposedTool{Name: t.Name, Description: t.Description}
		if t.Definition.Function != nil && t.Definition.Function.Parameters != nil {
			if raw, err := json.Marshal(t.Definition.Function.Parameters); err == nil {
				et.Parameters = raw
			}
		}
		list = append(list, et)
	}
	return list
}

// ListTools returns all tool entries — for the agent editor tool picker.
func (r *Registry) ListTools() []ToolEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []ToolEntry
	for _, t := range r.tools {
		list = append(list, *t)
	}
	return list
}

// BuildToolList builds a []openai.Tool list for LLM usage from tool names.
func (r *Registry) BuildToolList(names []string) []openai.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var list []openai.Tool
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			list = append(list, t.Definition)
		}
	}
	return list
}

// RegisterTo registers handlers for the given tool names on a session Manager.
func (r *Registry) RegisterTo(mgr *session.Manager, names []string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			mgr.RegisterTool(name, t.Handler)
		}
	}
}

// RegisterAllTo registers all tools in the registry to a session Manager.
func (r *Registry) RegisterAllTo(mgr *session.Manager) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for name, t := range r.tools {
		mgr.RegisterTool(name, t.Handler)
	}
}

// Global registry singleton.
var Global = NewRegistry()

// RegisterBuiltin registers a builtin tool. Used during init.
func RegisterBuiltin(name string, def openai.Tool, handler func(ctx context.Context, args string) *session.ToolResult, desc string) {
	Global.Register(name, def, handler, desc)
}
