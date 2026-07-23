package tools

import (
	"context"
	"encoding/json"

	"beleader/gateway/db"
	"beleader/gateway/engine"
)

// RegisterLocalTools registers all local (Gateway-side) tool handlers.
// These tools execute in the Gateway process and do not need a workspace.
func RegisterLocalTools(r *Router) {
	r.RegisterLocal("read_status", readStatusHandler)
	r.RegisterLocal("update_status", updateStatusHandler)
}

func readStatusHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	t, err := h_getThread(thread.ID)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	if t.StatusContent == "" {
		return &engine.ToolResult{Content: "No STATUS.md content found."}
	}
	return &engine.ToolResult{Content: t.StatusContent}
}

func updateStatusHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct{ Content string `json:"content"` }
	json.Unmarshal([]byte(args), &p)
	if p.Content == "" {
		return &engine.ToolResult{Error: "content is required"}
	}
	if err := h_updateThreadStatus(thread.ID, p.Content); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Status updated."}
}
