package tools

import (
	"context"
	"encoding/json"

	"beleader/runtime/engine"
	"beleader/runtime/llm"
)

func spawnWorkerHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		SystemPrompt string   `json:"system_prompt"`
		Task         string   `json:"task"`
		Tools        []string `json:"tools"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.SystemPrompt == "" {
		return &engine.ToolResult{Error: "system_prompt is required"}
	}
	if p.Task == "" {
		return &engine.ToolResult{Error: "task is required"}
	}

	// Get parent thread info from context.
	threadID, _ := ctx.Value(engine.CtxKeyThreadID).(string)
	workDir, _ := ctx.Value(engine.CtxKeyWorkDir).(string)
	threadDir, _ := ctx.Value(engine.CtxKeyThreadDir).(string)
	visionEnabled, _ := ctx.Value(engine.CtxKeyVisionEnabled).(bool)

	// Need access to the engine and parent thread.
	// We use a global reference set during registration.
	eng := globalEngine
	if eng == nil {
		return &engine.ToolResult{Error: "spawn_worker: engine not available"}
	}
	parentThread := globalThreads[threadID]
	if parentThread == nil {
		return &engine.ToolResult{Error: "spawn_worker: parent thread not found"}
	}

	// Build a tool list for the worker.
	allDefs := baseToolDefs()
	var subToolDefs []engine.ToolDef
	if len(p.Tools) > 0 {
		nameSet := make(map[string]bool, len(p.Tools))
		for _, t := range p.Tools {
			nameSet[t] = true
		}
		for _, d := range allDefs {
			if nameSet[d.Name] {
				subToolDefs = append(subToolDefs, d)
			}
		}
	} else {
		subToolDefs = allDefs
	}

	n2 := len(p.Task)
	if n2 > 8 {
		n2 = 8
	}
	subThread := &engine.Thread{
		ID:            threadID + "_sub_" + p.Task[:n2],
		SystemPrompt:  p.SystemPrompt,
		Model:         parentThread.Model,
		ToolDefs:      subToolDefs,
		WorkspaceDir:  workDir,
		DataDir:       threadDir,
		MaxContextPct: parentThread.MaxContextPct,
	}

	llmClient := llm.New(subThread.Model.BaseURL, subThread.Model.APIKey, subThread.Model.Model)
	toolList := engine.ToolDefsToOpenAI(subToolDefs)

	// Use the emit from parent context so sub-agent progress is visible.
	// But for spawn_worker, we consume the events internally and only return the final result.
	// We create a no-op emit for the sub-agent so it doesn't interfere with parent SSE.
	noopEmit := func(ev engine.RuntimeEventRecord) {}

	// Run the sub-agent.
	result, _ := eng.RunLoop(ctx, subThread, "sub_"+engine.NewTurnID(), p.SystemPrompt, p.Task, toolList, llmClient, subThread.Model.ContextLimit, visionEnabled, make(chan struct{}), make(chan engine.InterveneMsg, 1), noopEmit)

	if result != nil {
		if result.Error != "" {
			return &engine.ToolResult{Error: "worker error: " + result.Error}
		}
		return &engine.ToolResult{Content: result.Content}
	}
	return &engine.ToolResult{Content: "Worker completed."}
}

var (
	globalEngine  *engine.Engine
	globalThreads map[string]*engine.Thread
)

// SetWorkerGlobals sets the engine and thread map for spawn_worker to access.
func SetWorkerGlobals(eng *engine.Engine, threads map[string]*engine.Thread) {
	globalEngine = eng
	globalThreads = threads
}

// baseToolDefs returns all builtin tool defs for worker use.
func baseToolDefs() []engine.ToolDef {
	return []engine.ToolDef{
		engine.MkTool("read_file", "Read a file from the filesystem.", map[string]any{
			"path":   map[string]any{"type": "string", "description": "Absolute path to the file."},
			"offset": map[string]any{"type": "integer", "description": "Line number to start reading from."},
			"limit":  map[string]any{"type": "integer", "description": "Maximum lines to read."},
		}, []string{"path"}),
		engine.MkTool("read_dir", "List contents of a directory.", map[string]any{
			"path": map[string]any{"type": "string", "description": "Directory path."},
		}, []string{"path"}),
		engine.MkTool("write_file", "Write content to a file.", map[string]any{
			"path":    map[string]any{"type": "string", "description": "File path."},
			"content": map[string]any{"type": "string", "description": "File content."},
		}, []string{"path", "content"}),
		engine.MkTool("edit_file", "Replace a single occurrence of old_string with new_string.", map[string]any{
			"path":       map[string]any{"type": "string", "description": "File path."},
			"old_string": map[string]any{"type": "string", "description": "Exact text to replace."},
			"new_string": map[string]any{"type": "string", "description": "Replacement text."},
		}, []string{"path", "old_string", "new_string"}),
		engine.MkTool("delete_file", "Move files to .trash directory.", map[string]any{
			"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Paths to delete."},
		}, []string{"paths"}),
		engine.MkTool("search_content", "Search for a pattern in file contents.", map[string]any{
			"pattern":       map[string]any{"type": "string", "description": "Regex pattern."},
			"file_pattern":  map[string]any{"type": "string", "description": "File glob."},
			"path":          map[string]any{"type": "string", "description": "Path to a file."},
			"context_lines": map[string]any{"type": "integer", "description": "Context lines."},
			"offset":        map[string]any{"type": "integer", "description": "Result offset."},
			"limit":         map[string]any{"type": "integer", "description": "Max results."},
		}, []string{"pattern"}),
		engine.MkTool("search_files", "Find files by glob pattern.", map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern."},
			"path":    map[string]any{"type": "string", "description": "Directory to search."},
		}, []string{"pattern"}),
		engine.MkTool("run_command", "Run a shell command.", map[string]any{
			"command":    map[string]any{"type": "string", "description": "Command to execute."},
			"background": map[string]any{"type": "boolean", "description": "Run in background."},
			"timeout":    map[string]any{"type": "integer", "description": "Timeout in seconds."},
		}, []string{"command"}),
		engine.MkTool("web_search", "Search the web.", map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query."},
		}, []string{"query"}),
		engine.MkTool("web_fetch", "Fetch a URL.", map[string]any{
			"url": map[string]any{"type": "string", "description": "URL to fetch."},
		}, []string{"url"}),
		engine.MkTool("run_http_request", "Make an HTTP request.", map[string]any{
			"url":     map[string]any{"type": "string", "description": "URL."},
			"method":  map[string]any{"type": "string", "description": "HTTP method."},
			"headers": map[string]any{"type": "object", "description": "Headers."},
			"body":    map[string]any{"type": "string", "description": "Request body."},
		}, []string{"url"}),
	}
}
