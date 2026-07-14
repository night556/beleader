package engine

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolResult is returned by tool handlers.
type ToolResult struct {
	Content        string   `json:"content,omitempty"`
	Error          string   `json:"error,omitempty"`
	Images         []string `json:"images,omitempty"`
	ImageLabel     string   `json:"-"`
	Width          int      `json:"-"`
	Height         int      `json:"-"`
	ShouldContinue *bool    `json:"should_continue,omitempty"`
}

// LoopResult is the outcome of a RunLoop invocation.
type LoopResult struct {
	Completed bool
	Paused    bool
	Stopped   bool
	Rounds    int
	Content   string
	Error     string
}

type ctxKey string

const (
	CtxKeyVisionEnabled ctxKey = "vision_enabled"
	CtxKeyProgress      ctxKey = "progress"
	CtxKeyToolCallID    ctxKey = "tool_call_id"
	CtxKeyThreadID      ctxKey = "thread_id"
	CtxKeyWorkDir       ctxKey = "work_dir"
	CtxKeyThreadDir     ctxKey = "thread_dir"
)

// SendProgress sends a tool_progress event if a progress callback is in the context.
// In the new EventFrame model, this emits exec_command_output_delta events.
func SendProgress(ctx context.Context, command, delta string) {
	progress, _ := ctx.Value(CtxKeyProgress).(func(EventFrame))
	if progress == nil {
		return
	}
	tcID, _ := ctx.Value(CtxKeyToolCallID).(string)
	tid, _ := ctx.Value(CtxKeyThreadID).(string)
	progress(EventFrame{
		Event:      EventExecCommandOutputDelta,
		Command:    command,
		Delta:      delta,
		ToolName:   "run_command",
		ResponseID: tcID,
		TurnID:     tid,
	})
}

// Engine manages tool handlers and runs the LLM loop.
type Engine struct {
	ToolHandlers map[string]func(ctx context.Context, args string) *ToolResult
}

// NewEngine creates a new Engine.
func NewEngine() *Engine {
	return &Engine{
		ToolHandlers: make(map[string]func(ctx context.Context, args string) *ToolResult),
	}
}

// RegisterTool registers a tool handler.
func (e *Engine) RegisterTool(name string, handler func(ctx context.Context, args string) *ToolResult) {
	e.ToolHandlers[name] = handler
}

func (e *Engine) executeTool(ctx context.Context, tc ToolCall) *ToolResult {
	handler, ok := e.ToolHandlers[tc.Function.Name]
	if !ok {
		return &ToolResult{Error: fmt.Sprintf("unknown tool: %s", tc.Function.Name)}
	}
	return handler(ctx, tc.Function.Arguments)
}

// MkTool creates a ToolDef. Used by the tools package.
func MkTool(name, description string, properties map[string]any, required []string) ToolDef {
	params := map[string]any{"type": "object"}
	if properties != nil {
		params["properties"] = properties
	}
	if len(required) > 0 {
		params["required"] = required
	}
	return ToolDef{Name: name, Description: description, Parameters: params}
}

// ValidateRequired checks that required fields are present in args JSON.
func ValidateRequired(args string, required []string) *ToolResult {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return &ToolResult{Error: "invalid args: " + err.Error()}
	}
	for _, k := range required {
		v, ok := m[k]
		if !ok || v == nil {
			return &ToolResult{Error: fmt.Sprintf("%s is required", k)}
		}
		if s, ok := v.(string); ok && s == "" {
			return &ToolResult{Error: fmt.Sprintf("%s is required", k)}
		}
	}
	return nil
}

// InterveneMsg is a message injected into a running loop.
type InterveneMsg struct {
	Message string   `json:"message"`
	Images  []string `json:"images"`
}

// ProgressCallback is the callback type for emitting events during RunLoop.
// It receives an EventFrame that will be written to SSE + events.jsonl.
type ProgressCallback func(EventFrame)

// CompressPrompt is the system prompt for context compression.
const CompressPrompt = `You are compressing a conversation to save context space. Your output will replace all previous messages. The assistant must be able to continue working from your summary alone.

Summarize:
- What the user asked for and what the assistant did about it
- Key information from tool outputs (file paths, errors, search results, code snippets that matter)
- What the assistant was doing right before compression

Rules:
- Preserve actionable information (file paths, error messages, relevant code)
- Discard redundant tool output, boilerplate, and noise
- Do NOT create a task plan — just record what happened
- Be dense. Every sentence should carry information needed to continue.`
