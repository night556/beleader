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
	CtxKeyVisionEnabled     ctxKey = "vision_enabled"
	CtxKeyProgress          ctxKey = "progress"
	CtxKeyToolCallID        ctxKey = "tool_call_id"
	CtxKeyThreadID          ctxKey = "thread_id"
	CtxKeyTurnID            ctxKey = "turn_id"
	CtxKeyItemID            ctxKey = "item_id"
	CtxKeyWorkDir           ctxKey = "work_dir"
	CtxKeyThreadDir         ctxKey = "thread_dir"
	CtxKeyRestrictWorkspace ctxKey = "restrict_workspace"
)

// EmitEvent sends an event through the progress callback in ctx.
func EmitEvent(ctx context.Context, ev RuntimeEventRecord) {
	progress, _ := ctx.Value(CtxKeyProgress).(func(RuntimeEventRecord))
	if progress != nil {
		progress(ev)
	}
}

// SendProgress emits item.delta events for streaming command output.
func SendProgress(ctx context.Context, command, delta string) {
	itemID, _ := ctx.Value(CtxKeyItemID).(string)
	EmitEvent(ctx, RuntimeEventRecord{
		SchemaVersion: 1,
		Event:         EventItemDelta,
		ItemID:        itemID,
		Payload: map[string]any{
			"delta": delta,
			"kind":  ItemKindCommandExecution,
		},
	})
}

// SendCommandBegin emits item.started for a command_execution item.
func SendCommandBegin(ctx context.Context, command string) string {
	itemID, _ := ctx.Value(CtxKeyItemID).(string)
	turnID, _ := ctx.Value(CtxKeyTurnID).(string)
	metadata := map[string]any{
		"tool_name": "run_command",
		"command":   command,
	}
	ev := StartItem(itemID, turnID, ItemKindCommandExecution, command, metadata)
	// Add tool info to payload (CodeWhale includes tool details in item.started for tools).
	payload := ev.Payload
	payload["tool"] = map[string]any{
		"id":    itemID,
		"name":  "run_command",
		"input": map[string]any{"command": command},
	}
	ev.Payload = payload
	EmitEvent(ctx, ev)
	return itemID
}

// SendCommandEnd emits item.completed for a command_execution item.
func SendCommandEnd(ctx context.Context, command string, exitCode int) {
	itemID, _ := ctx.Value(CtxKeyItemID).(string)
	turnID, _ := ctx.Value(CtxKeyTurnID).(string)
	status := ItemStatusCompleted
	if exitCode != 0 {
		status = ItemStatusFailed
	}
	detail := fmt.Sprintf("Command '%s' exited with code %d", command, exitCode)
	metadata := map[string]any{
		"tool_name": "run_command",
		"command":   command,
		"exit_code": exitCode,
	}
	ev := RuntimeEventRecord{
		SchemaVersion: 1,
		Event:         EventItemCompleted,
		ItemID:        itemID,
		Payload: map[string]any{
			"item": TurnItemRecord{
				ID:       itemID,
				TurnID:   turnID,
				Kind:     ItemKindCommandExecution,
				Status:   status,
				Summary:  detail,
				Detail:   detail,
				Metadata: metadata,
			},
		},
	}
	if exitCode != 0 {
		ev.Event = EventItemFailed
	}
	EmitEvent(ctx, ev)
}

// BackgroundResult is the result of a completed background command.
type BackgroundResult struct {
	ID       string
	Command  string
	ExitCode int
	Output   string
	Error    string
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
// It receives a RuntimeEventRecord that will be written to SSE + events.jsonl.
type ProgressCallback func(RuntimeEventRecord)

// CompressPrompt is the system prompt for context compression.
const CompressPrompt = `You are compressing a conversation to save context space.

Summarize EXACTLY in this format:

## Files
- path/to/file — what was done

## Decisions
- decision and rationale

## Errors / Blockers
- exact error messages verbatim

## Current State
- what is in progress

## Next Steps
- immediate next actions

Rules:
- Preserve exact file paths and error messages verbatim
- Drop greetings, filler, and redundant tool output
- Be dense — every sentence should carry actionable information`
