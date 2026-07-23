package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"beleader/gateway/db"
	"beleader/gateway/engine"

	"github.com/sashabaranov/go-openai"
)

// ── Global DB (set once at startup) ──

var globalDB *db.DB

func SetDB(database *db.DB) {
	globalDB = database
}

// ── HTTP helpers ──

var httpClient = &http.Client{Timeout: 300 * time.Second}

func newRequestWithContext(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequestWithContext(ctx, method, url, nil)
	}
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

// Router implements engine.ToolRouter.
type Router struct {
	DB            *db.DB
	LocalHandlers map[string]func(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult
	HTTPClients   map[int64]*AgentClient // poolID → client (round-robin pool)
	mu            sync.Mutex
	rrCounter     map[int64]int
}

func NewRouter(database *db.DB) *Router {
	return &Router{
		DB:            database,
		LocalHandlers: make(map[string]func(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult),
		HTTPClients:   make(map[int64]*AgentClient),
		rrCounter:     make(map[int64]int),
	}
}

func (r *Router) RegisterLocal(name string, handler func(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult) {
	r.LocalHandlers[name] = handler
}

func isLocal(name string) bool {
	switch name {
	case "web_search", "web_fetch", "run_http_request",
		"read_status", "update_status",
		"spawn_worker", "list_workers", "intervene_worker", "terminate_worker",
		"create_agent", "list_agents", "update_agent", "delete_agent",
		"create_model", "list_resources",
		"create_mcp_server", "delete_mcp_server", "list_mcp_servers":
		return true
	}
	return false
}

func (r *Router) Execute(ctx context.Context, thread *db.Thread, tc openai.ToolCall) *engine.ToolResult {
	if isLocal(tc.Function.Name) {
		if h, ok := r.LocalHandlers[tc.Function.Name]; ok {
			return h(ctx, thread, tc.Function.Arguments)
		}
		return &engine.ToolResult{Error: "local tool not implemented: " + tc.Function.Name}
	}

	// Remote: pick an agent from the thread's pool
	agents, err := r.DB.ListActiveToolAgentsByPool(thread.PoolID)
	if err != nil || len(agents) == 0 {
		return &engine.ToolResult{Error: "no tool agent available in pool"}
	}

	// Try agents round-robin, failover on error
	for i := 0; i < len(agents); i++ {
		idx := (r.rr(thread.PoolID) + i) % len(agents)
		agent := &agents[idx]
		client := NewAgentClient(agent.URL)
		result, err := client.Execute(ctx, thread.ID, thread.WorkspacePath, tc.Function.Name, tc.Function.Arguments)
		if err == nil {
			return result
		}
	}
	return &engine.ToolResult{Error: "all agents in pool failed"}
}

func (r *Router) rr(poolID int64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := r.rrCounter[poolID]
	r.rrCounter[poolID] = (n + 1) % 1000
	return n
}

// ── Agent HTTP Client ──

type AgentClient struct {
	BaseURL string
}

func NewAgentClient(baseURL string) *AgentClient {
	return &AgentClient{BaseURL: strings.TrimRight(baseURL, "/")}
}

func (c *AgentClient) Execute(ctx context.Context, threadID, workspace, toolName, args string) (*engine.ToolResult, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"thread_id":  threadID,
		"workspace":  workspace,
		"tool":       toolName,
		"args":       json.RawMessage(args),
	})

	req, err := newRequestWithContext(ctx, "POST", c.BaseURL+"/execute", reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("agent returned %d", resp.StatusCode)
	}

	var result engine.ToolResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *AgentClient) InitWorkspace(ctx context.Context, threadID string) (string, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"thread_id": threadID,
	})
	req, err := newRequestWithContext(ctx, "POST", c.BaseURL+"/workspace/init", reqBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("workspace init returned %d", resp.StatusCode)
	}

	var result struct {
		Workspace string `json:"workspace"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.Workspace, nil
}

func (c *AgentClient) CleanupWorkspace(ctx context.Context, threadID string) error {
	reqBody, _ := json.Marshal(map[string]any{
		"thread_id": threadID,
	})
	req, err := newRequestWithContext(ctx, "POST", c.BaseURL+"/workspace/cleanup", reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// ── Tool Definitions (local tools only, remote tools come from Pool.ToolDefs) ──

func LocalToolDefs() []engine.ToolDef {
	return []engine.ToolDef{
		engine.MkTool("web_search", "Search the web using Bing and return results.",
			map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			}, []string{"query"}),
		engine.MkTool("web_fetch", "Fetch content from a URL and return as text.",
			map[string]any{
				"url":   map[string]any{"type": "string", "description": "URL to fetch"},
				"json":  map[string]any{"type": "boolean", "description": "Parse response as JSON"},
			}, []string{"url"}),
		engine.MkTool("run_http_request", "Make an HTTP request with custom method, headers, and body.",
			map[string]any{
				"url":     map[string]any{"type": "string", "description": "URL to request"},
				"method":  map[string]any{"type": "string", "description": "HTTP method (GET, POST, etc.)"},
				"headers": map[string]any{"type": "object", "description": "Request headers"},
				"body":    map[string]any{"type": "string", "description": "Request body"},
			}, []string{"url"}),
		engine.MkTool("read_status", "Read the project STATUS.md content from the DB.",
			map[string]any{}, []string{}),
		engine.MkTool("update_status", "Update the project STATUS.md content in the DB. Use this ONLY for STATUS.md.",
			map[string]any{
				"content": map[string]any{"type": "string", "description": "The complete updated STATUS.md content"},
			}, []string{"content"}),
		engine.MkTool("spawn_worker", "Spawn a worker sub-agent to execute a task. Returns immediately with worker info.",
			map[string]any{
				"agent": map[string]any{"type": "string", "description": "Worker agent name"},
				"task":  map[string]any{"type": "string", "description": "Task description for the worker"},
				"pool":  map[string]any{"type": "string", "description": "Optional: pool to run worker on (defaults to parent's pool)"},
			}, []string{"agent", "task"}),
		engine.MkTool("list_workers", "List all worker threads for the current thread.",
			map[string]any{}, []string{}),
		engine.MkTool("intervene_worker", "Send a message to a running worker thread.",
			map[string]any{
				"thread_id": map[string]any{"type": "string", "description": "Worker thread ID"},
				"message":   map[string]any{"type": "string", "description": "Message to send"},
			}, []string{"thread_id", "message"}),
		engine.MkTool("terminate_worker", "Stop a worker thread.",
			map[string]any{
				"thread_id": map[string]any{"type": "string", "description": "Worker thread ID"},
			}, []string{"thread_id"}),
		engine.MkTool("create_agent", "Create a new agent template.",
			map[string]any{
				"name":          map[string]any{"type": "string", "description": "Agent name"},
				"system_prompt": map[string]any{"type": "string", "description": "System prompt"},
				"desc":          map[string]any{"type": "string", "description": "Short description"},
				"tools":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Tool names"},
				"default_model_id": map[string]any{"type": "string", "description": "Default model ID"},
				"mcp_servers":   map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "MCP server names"},
				"worker_agents": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Worker agent names"},
			}, []string{"name", "system_prompt"}),
		engine.MkTool("list_agents", "List all agent templates.", map[string]any{}, []string{}),
		engine.MkTool("update_agent", "Update an existing agent template.",
			map[string]any{
				"id":            map[string]any{"type": "integer", "description": "Agent ID"},
				"name":          map[string]any{"type": "string", "description": "Agent name"},
				"system_prompt": map[string]any{"type": "string", "description": "System prompt"},
				"desc":          map[string]any{"type": "string", "description": "Short description"},
			}, []string{"id"}),
		engine.MkTool("delete_agent", "Delete an agent template.",
			map[string]any{
				"id": map[string]any{"type": "integer", "description": "Agent ID"},
			}, []string{"id"}),
		engine.MkTool("create_model", "Create a new LLM model profile.",
			map[string]any{
				"id":              map[string]any{"type": "string", "description": "Unique model ID"},
				"base_url":        map[string]any{"type": "string", "description": "API base URL"},
				"api_key":         map[string]any{"type": "string", "description": "API key"},
				"model":           map[string]any{"type": "string", "description": "Model name"},
				"vision":          map[string]any{"type": "boolean", "description": "Supports image input"},
				"context_limit":   map[string]any{"type": "integer", "description": "Context window size"},
				"reasoning_effort": map[string]any{"type": "string", "description": "Reasoning effort: off, low, medium, high, max"},
			}, []string{"id", "base_url", "api_key", "model"}),
		engine.MkTool("list_resources", "List system resources of a given type.",
			map[string]any{
				"type": map[string]any{"type": "string", "description": "Resource type: runtimes, models, knowledge"},
			}, []string{"type"}),
		engine.MkTool("create_mcp_server", "Create a new MCP server connection.",
			map[string]any{
				"name":    map[string]any{"type": "string", "description": "Server name"},
				"type":    map[string]any{"type": "string", "description": "Connection type: stdio or http"},
				"command": map[string]any{"type": "string", "description": "Command (for stdio)"},
				"args":    map[string]any{"type": "string", "description": "JSON array of args (for stdio)"},
				"env":     map[string]any{"type": "string", "description": "JSON object of env vars (for stdio)"},
				"url":     map[string]any{"type": "string", "description": "URL (for http)"},
				"headers": map[string]any{"type": "string", "description": "JSON object of headers (for http)"},
			}, []string{"name", "type"}),
		engine.MkTool("delete_mcp_server", "Delete an MCP server by its numeric ID.",
			map[string]any{
				"id": map[string]any{"type": "integer", "description": "MCP server ID"},
			}, []string{"id"}),
		engine.MkTool("list_mcp_servers", "List all configured MCP servers.", map[string]any{}, []string{}),
	}
}

// ── DB helpers for local tools ──

func h_getThread(id string) (*db.Thread, error) {
	return globalDB.GetThread(id)
}

func h_updateThreadStatus(id, content string) error {
	return globalDB.UpdateThread(id, map[string]any{"status_content": content})
}
