package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"beleader/gateway/db"
	"beleader/gateway/engine"
)

// WorkerCallbacks holds callbacks for worker lifecycle.
type WorkerCallbacks struct {
	SpawnWorker     func(ctx context.Context, parentThread *db.Thread, agentName, task, poolName string) (string, error)
	ListWorkers     func(parentThreadID string) ([]db.Thread, error)
	InterveneWorker func(ctx context.Context, workerThreadID, message string) error
	TerminateWorker func(workerThreadID string) error
}

var workerCB *WorkerCallbacks

// SetWorkerCallbacks sets the worker lifecycle callbacks.
func SetWorkerCallbacks(cb *WorkerCallbacks) {
	workerCB = cb
}

// RegisterWorkerTools registers worker management tool handlers.
func RegisterWorkerTools(r *Router) {
	r.RegisterLocal("spawn_worker", spawnWorkerHandler)
	r.RegisterLocal("list_workers", listWorkersHandler)
	r.RegisterLocal("intervene_worker", interveneWorkerHandler)
	r.RegisterLocal("terminate_worker", terminateWorkerHandler)
}

func spawnWorkerHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		Agent string `json:"agent"`
		Task  string `json:"task"`
		Pool  string `json:"pool"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Agent == "" {
		return &engine.ToolResult{Error: "agent is required"}
	}
	if p.Task == "" {
		return &engine.ToolResult{Error: "task is required"}
	}

	// Validate worker whitelist
	var workerNames []string
	if thread != nil {
		agent, _ := globalDB.GetAgent(thread.AgentID)
		if agent != nil {
			json.Unmarshal([]byte(agent.WorkerAgents), &workerNames)
		}
	}
	if len(workerNames) > 0 {
		allowed := false
		for _, n := range workerNames {
			if n == p.Agent {
				allowed = true
				break
			}
		}
		if !allowed {
			return &engine.ToolResult{Error: fmt.Sprintf("agent '%s' is not in the allowed worker list", p.Agent)}
		}
	}

	if workerCB == nil || workerCB.SpawnWorker == nil {
		return &engine.ToolResult{Error: "worker spawning not configured"}
	}

	workerID, err := workerCB.SpawnWorker(ctx, thread, p.Agent, p.Task, p.Pool)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("Worker spawned: %s (thread %s)\nTask: %s", p.Agent, workerID, p.Task)}
}

func listWorkersHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	if workerCB == nil || workerCB.ListWorkers == nil {
		return &engine.ToolResult{Content: "[]"}
	}
	workers, err := workerCB.ListWorkers(thread.ID)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	var b strings.Builder
	b.WriteString("[")
	for i, w := range workers {
		if i > 0 {
			b.WriteString(",")
		}
		status := w.Status
		if status == "" {
			status = "unknown"
		}
		b.WriteString(fmt.Sprintf(`{"id":"%s","title":"%s","status":"%s"}`, w.ID, w.Title, status))
	}
	b.WriteString("]")
	return &engine.ToolResult{Content: b.String()}
}

func interveneWorkerHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		ThreadID string `json:"thread_id"`
		Message  string `json:"message"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.ThreadID == "" {
		return &engine.ToolResult{Error: "thread_id is required"}
	}
	if p.Message == "" {
		return &engine.ToolResult{Error: "message is required"}
	}
	if workerCB == nil || workerCB.InterveneWorker == nil {
		return &engine.ToolResult{Error: "worker intervention not configured"}
	}
	if err := workerCB.InterveneWorker(ctx, p.ThreadID, p.Message); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Message sent to worker."}
}

func terminateWorkerHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		ThreadID string `json:"thread_id"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.ThreadID == "" {
		return &engine.ToolResult{Error: "thread_id is required"}
	}
	if workerCB == nil || workerCB.TerminateWorker == nil {
		return &engine.ToolResult{Error: "worker termination not configured"}
	}
	if err := workerCB.TerminateWorker(p.ThreadID); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Worker terminated."}
}
