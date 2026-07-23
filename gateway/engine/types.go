package engine

import (
	"encoding/json"
	"time"
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

// TokenUsage captures per-turn token consumption details.
type TokenUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
	Cached     int `json:"cached"`
}

// LoopResult is the outcome of a RunLoop invocation.
type LoopResult struct {
	Completed   bool
	Stopped     bool
	Rounds      int
	Usage       TokenUsage
	Content     string
	Error       string
}

// ToolDef is a lightweight tool definition.
type ToolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// MkTool creates a ToolDef.
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

// ProgressCallback is the callback type for emitting events during RunLoop.
type ProgressCallback func(eventType, turnID, itemID string, payload map[string]any)

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

// ParseJSONMap parses a JSON string into a map.
func ParseJSONMap(s string) (map[string]any, bool) {
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, false
	}
	return m, true
}

// MarshalJSON marshals a value to a JSON string.
func MarshalJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// NowRFC3339 returns the current time in RFC3339Nano format.
func NowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
