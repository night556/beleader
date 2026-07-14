package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"beleader/runtime/engine"
)

func readStatusHandler(ctx context.Context, args string) *engine.ToolResult {
	threadDir, _ := ctx.Value(engine.CtxKeyThreadDir).(string)
	if threadDir == "" {
		return &engine.ToolResult{Error: "thread_dir not set in context"}
	}
	statusPath := filepath.Join(threadDir, "STATUS.md")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return &engine.ToolResult{Error: "STATUS.md not found: " + err.Error()}
	}
	return &engine.ToolResult{Content: string(data)}
}

func updateStatusHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct{ Content string `json:"content"` }
	json.Unmarshal([]byte(args), &p)
	if p.Content == "" {
		return &engine.ToolResult{Error: "content is required"}
	}
	threadDir, _ := ctx.Value(engine.CtxKeyThreadDir).(string)
	if threadDir == "" {
		return &engine.ToolResult{Error: "thread_dir not set in context"}
	}
	statusPath := filepath.Join(threadDir, "STATUS.md")
	if err := os.MkdirAll(filepath.Dir(statusPath), 0755); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	if err := os.WriteFile(statusPath, []byte(p.Content), 0644); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("STATUS.md updated (%d bytes)", len(p.Content))}
}

// RegisterStatusTools registers the status read/update tools.
func RegisterStatusTools(eng *engine.Engine) {
	eng.RegisterTool("read_status", readStatusHandler)
	eng.RegisterTool("update_status", updateStatusHandler)
}
