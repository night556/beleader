package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"beleader/runtime/engine"
)

func spawnWorkerHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Agent == "" {
		return &engine.ToolResult{Error: "agent is required"}
	}
	if p.Task == "" {
		return &engine.ToolResult{Error: "task is required"}
	}

	threadID, _ := ctx.Value(engine.CtxKeyThreadID).(string)
	workDir, _ := ctx.Value(engine.CtxKeyWorkDir).(string)

	eng := globalEngine
	if eng == nil {
		return &engine.ToolResult{Error: "spawn_worker: engine not available"}
	}
	parentThread := globalThreads[threadID]
	if parentThread == nil {
		return &engine.ToolResult{Error: "spawn_worker: parent thread not found"}
	}

	// Validate worker whitelist.
	workerAgents, _ := parentThread.Metadata["worker_agents"].([]any)
	if len(workerAgents) == 0 {
		return &engine.ToolResult{Error: "spawn_worker: this agent has no workers configured"}
	}
	allowed := false
	for _, wa := range workerAgents {
		if name, _ := wa.(string); name == p.Agent {
			allowed = true
			break
		}
	}
	if !allowed {
		return &engine.ToolResult{Error: "spawn_worker: agent '" + p.Agent + "' is not in the allowed worker list"}
	}

	// Fetch worker agent ID and config from Gateway.
	result, err := callGateway("GET", "/api/agents", nil)
	if err != nil {
		return &engine.ToolResult{Error: "spawn_worker: failed to fetch agents: " + err.Error()}
	}
	agents, _ := result["agents"].([]any)
	var workerAgentID float64
	var workerModelID string
	found := false
	for _, a := range agents {
		if ag, ok := a.(map[string]any); ok {
			if name, _ := ag["name"].(string); name == p.Agent {
				workerAgentID, _ = ag["id"].(float64)
				workerModelID, _ = ag["default_model_id"].(string)
				found = true
				break
			}
		}
	}
	if !found {
		return &engine.ToolResult{Error: "spawn_worker: agent '" + p.Agent + "' not found"}
	}

	if gatewayURL == "" {
		return &engine.ToolResult{Error: "spawn_worker: gateway URL not configured"}
	}

	// Build the chat request to create and run the worker thread.
	chatReq := map[string]any{
		"agent_id":         int64(workerAgentID),
		"message":          p.Task,
		"parent_thread_id": threadID,
		"workspace_dir":    workDir,
	}
	if workerModelID != "" {
		chatReq["model_id"] = workerModelID
	}
	body, _ := json.Marshal(chatReq)

	req, err := http.NewRequest("POST", gatewayURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return &engine.ToolResult{Error: "spawn_worker: create request: " + err.Error()}
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &engine.ToolResult{Error: "spawn_worker: gateway request failed: " + err.Error()}
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	var chatResp struct {
		ThreadID string `json:"thread_id"`
		Status   string `json:"status"`
	}
	json.Unmarshal(respBody, &chatResp)
	if chatResp.ThreadID == "" {
		return &engine.ToolResult{Error: "spawn_worker: no thread_id returned"}
	}

	return &engine.ToolResult{Content: fmt.Sprintf("Worker dispatched: %s", chatResp.ThreadID)}
}

// ── check_workers ──

func checkWorkersHandler(ctx context.Context, args string) *engine.ToolResult {
	threadID, _ := ctx.Value(engine.CtxKeyThreadID).(string)

	result, err := callGateway("GET", "/api/threads/"+threadID+"/workers", nil)
	if err != nil {
		return &engine.ToolResult{Error: "check_workers: " + err.Error()}
	}

	workers, _ := result["workers"].([]any)
	if len(workers) == 0 {
		return &engine.ToolResult{Content: "No workers."}
	}

	var running, completed int
	var runningNames, completedNames []string
	for _, w := range workers {
		if wm, ok := w.(map[string]any); ok {
			title, _ := wm["title"].(string)
			status, _ := wm["status"].(string)
			switch status {
			case "running":
				running++
				runningNames = append(runningNames, title)
			case "completed":
				completed++
				completedNames = append(completedNames, title)
			}
		}
	}

	var b bytes.Buffer
	if running > 0 {
		fmt.Fprintf(&b, "%d running: ", running)
		for i, n := range runningNames {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(n)
		}
	}
	if completed > 0 {
		if b.Len() > 0 {
			b.WriteString(" | ")
		}
		fmt.Fprintf(&b, "%d completed: ", completed)
		for i, n := range completedNames {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(n)
		}
	}
	return &engine.ToolResult{Content: b.String()}
}

// ── stop_worker ──

func stopWorkerHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		ThreadID string `json:"thread_id"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.ThreadID == "" {
		return &engine.ToolResult{Error: "thread_id is required"}
	}

	parentThreadID, _ := ctx.Value(engine.CtxKeyThreadID).(string)
	_, err := callGateway("POST", "/api/threads/"+parentThreadID+"/workers/"+p.ThreadID+"/stop", nil)
	if err != nil {
		return &engine.ToolResult{Error: "stop_worker: " + err.Error()}
	}
	return &engine.ToolResult{Content: "Worker stopped: " + p.ThreadID}
}

var (
	globalEngine  *engine.Engine
	globalThreads map[string]*engine.Thread
)

func SetWorkerGlobals(eng *engine.Engine, threads map[string]*engine.Thread) {
	globalEngine = eng
	globalThreads = threads
}
