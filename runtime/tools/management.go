package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"beleader/runtime/engine"
)

var gatewayURL string

// SetGatewayURL sets the Gateway base URL for management tools.
func SetGatewayURL(url string) {
	gatewayURL = strings.TrimRight(url, "/")
}

func gwErr(msg string) *engine.ToolResult {
	return &engine.ToolResult{Error: msg}
}

// callGateway makes an HTTP request to the Gateway and returns the decoded response body.
func callGateway(method, path string, body any) (map[string]any, error) {
	if gatewayURL == "" {
		return nil, fmt.Errorf("gateway URL not configured — set GATEWAY_URL env var")
	}
	url := gatewayURL + path

	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gateway request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("gateway returned non-JSON (status %d): %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode >= 400 {
		errMsg, _ := result["error"].(string)
		if errMsg == "" {
			errMsg = resp.Status
		}
		return nil, fmt.Errorf("gateway error (%d): %s", resp.StatusCode, errMsg)
	}

	return result, nil
}

func callGatewayJSON(method, path string, body any) *engine.ToolResult {
	result, err := callGateway(method, path, body)
	if err != nil {
		return gwErr(err.Error())
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	return &engine.ToolResult{Content: string(b)}
}

// ── Agent management ──

func createAgentHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Name           string   `json:"name"`
		SystemPrompt   string   `json:"system_prompt"`
		Desc           string   `json:"desc"`
		Tools          []string `json:"tools"`
		DefaultModelID string   `json:"default_model_id"`
		MCPServers     []string `json:"mcp_servers"`
		WorkerAgents   []string `json:"worker_agents"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return gwErr("invalid args: " + err.Error())
	}
	if p.Name == "" {
		return gwErr("name is required")
	}
	if p.SystemPrompt == "" {
		return gwErr("system_prompt is required")
	}

	req := map[string]any{
		"name":          p.Name,
		"system_prompt": p.SystemPrompt,
	}
	if p.Desc != "" {
		req["desc"] = p.Desc
	}
	if len(p.Tools) > 0 {
		b, _ := json.Marshal(p.Tools)
		req["tools"] = string(b)
	}
	if p.DefaultModelID != "" {
		req["default_model_id"] = p.DefaultModelID
	}
	if len(p.MCPServers) > 0 {
		b, _ := json.Marshal(p.MCPServers)
		req["mcp_servers"] = string(b)
	}
	if len(p.WorkerAgents) > 0 {
		b, _ := json.Marshal(p.WorkerAgents)
		req["worker_agents"] = string(b)
	}

	return callGatewayJSON("POST", "/api/agents", req)
}

func updateAgentHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		ID             int64    `json:"id"`
		Name           string   `json:"name"`
		SystemPrompt   string   `json:"system_prompt"`
		Desc           string   `json:"desc"`
		Tools          []string `json:"tools"`
		DefaultModelID string   `json:"default_model_id"`
		MCPServers     []string `json:"mcp_servers"`
		WorkerAgents   []string `json:"worker_agents"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return gwErr("invalid args: " + err.Error())
	}
	if p.ID == 0 {
		return gwErr("id is required")
	}

	// Fetch existing agent to merge partial updates.
	existing, err := callGateway("GET", "/api/agents", nil)
	if err != nil {
		return gwErr("fetch existing agents: " + err.Error())
	}
	agents, _ := existing["agents"].([]any)
	var current map[string]any
	for _, a := range agents {
		if ag, ok := a.(map[string]any); ok {
			if id, _ := ag["id"].(float64); int64(id) == p.ID {
				current = ag
				break
			}
		}
	}
	if current == nil {
		return gwErr(fmt.Sprintf("agent %d not found", p.ID))
	}

	// Build merged request—only send fields that differ from current.
	req := map[string]any{
		"name":          pickStr(p.Name, currentStr(current, "name")),
		"system_prompt": pickStr(p.SystemPrompt, currentStr(current, "system_prompt")),
		"desc":          pickStr(p.Desc, currentStr(current, "desc")),
	}
	if p.Tools != nil {
		b, _ := json.Marshal(p.Tools)
		req["tools"] = string(b)
	}
	if p.DefaultModelID != "" {
		req["default_model_id"] = p.DefaultModelID
	} else if v := currentStr(current, "default_model_id"); v != "" {
		req["default_model_id"] = v
	}
	if p.MCPServers != nil {
		b, _ := json.Marshal(p.MCPServers)
		req["mcp_servers"] = string(b)
	}
	if p.WorkerAgents != nil {
		b, _ := json.Marshal(p.WorkerAgents)
		req["worker_agents"] = string(b)
	}

	return callGatewayJSON("PUT", "/api/agents/"+strconv.FormatInt(p.ID, 10), req)
}

func deleteAgentHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return gwErr("invalid args: " + err.Error())
	}
	if p.ID == 0 {
		return gwErr("id is required")
	}
	return callGatewayJSON("DELETE", "/api/agents/"+strconv.FormatInt(p.ID, 10), nil)
}

func listAgentsHandler(ctx context.Context, args string) *engine.ToolResult {
	return callGatewayJSON("GET", "/api/agents", nil)
}

// ── MCP Server management ──

func createMCPServerHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Command string `json:"command"`
		Args    string `json:"args"`
		Env     string `json:"env"`
		URL     string `json:"url"`
		Headers string `json:"headers"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return gwErr("invalid args: " + err.Error())
	}
	if p.Name == "" {
		return gwErr("name is required")
	}
	if p.Type == "" {
		return gwErr("type is required (stdio or http)")
	}
	if p.Type != "stdio" && p.Type != "http" {
		return gwErr("type must be stdio or http")
	}

	req := map[string]any{
		"name":    p.Name,
		"type":    p.Type,
		"enabled": true,
	}
	if p.Command != "" {
		req["command"] = p.Command
	}
	if p.Args != "" {
		req["args"] = p.Args
	}
	if p.Env != "" {
		req["env"] = p.Env
	}
	if p.URL != "" {
		req["url"] = p.URL
	}
	if p.Headers != "" {
		req["headers"] = p.Headers
	}

	return callGatewayJSON("POST", "/api/mcp/servers", req)
}

func deleteMCPServerHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return gwErr("invalid args: " + err.Error())
	}
	if p.ID == 0 {
		return gwErr("id is required")
	}
	return callGatewayJSON("DELETE", "/api/mcp/servers/"+strconv.FormatInt(p.ID, 10), nil)
}

func listMCPServersHandler(ctx context.Context, args string) *engine.ToolResult {
	return callGatewayJSON("GET", "/api/mcp/servers", nil)
}

// ── Model management ──

func createModelHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		ID              string `json:"id"`
		BaseURL         string `json:"base_url"`
		APIKey          string `json:"api_key"`
		Model           string `json:"model"`
		Vision          bool   `json:"vision"`
		ContextLimit    int    `json:"context_limit"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	if err := json.Unmarshal([]byte(args), &p); err != nil {
		return gwErr("invalid args: " + err.Error())
	}
	if p.ID == "" {
		return gwErr("id is required")
	}
	if p.BaseURL == "" {
		return gwErr("base_url is required")
	}
	if p.APIKey == "" {
		return gwErr("api_key is required")
	}
	if p.Model == "" {
		return gwErr("model name is required")
	}
	if p.ContextLimit <= 0 {
		p.ContextLimit = 128000
	}
	if p.ReasoningEffort == "" {
		p.ReasoningEffort = "high"
	}

	req := map[string]any{
		"id":               p.ID,
		"base_url":         p.BaseURL,
		"api_key":          p.APIKey,
		"model":            p.Model,
		"vision":           p.Vision,
		"context_limit":    p.ContextLimit,
		"reasoning_effort": p.ReasoningEffort,
	}

	return callGatewayJSON("POST", "/api/models", req)
}

// ── List resources ──

func listResourcesHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Type string `json:"type"`
	}
	json.Unmarshal([]byte(args), &p)

	var path string
	switch p.Type {
	case "runtimes":
		path = "/api/runtimes"
	case "models":
		path = "/api/models"
	case "knowledge":
		path = "/api/knowledge"
	default:
		return gwErr("type must be one of: runtimes, models, knowledge")
	}

	return callGatewayJSON("GET", path, nil)
}

// ── Helpers ──

func currentStr(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func pickStr(newVal, fallback string) string {
	if newVal != "" {
		return newVal
	}
	return fallback
}

// ── Tool definitions ──

func ManagementToolDefs() []engine.ToolDef {
	return []engine.ToolDef{
		engine.MkTool("create_agent",
			"Create a new AI agent with a system prompt and optional tools, MCP servers, and default model.",
			map[string]any{
				"name":             map[string]any{"type": "string", "description": "Agent name (unique)."},
				"system_prompt":    map[string]any{"type": "string", "description": "System prompt defining the agent's behavior."},
				"desc":             map[string]any{"type": "string", "description": "Short description of the agent."},
				"tools":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of tool names the agent may use."},
				"default_model_id": map[string]any{"type": "string", "description": "ID of the default model for this agent."},
				"mcp_servers":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of MCP server names to connect."},
				"worker_agents":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of agent names this agent can spawn as workers."},
			},
			[]string{"name", "system_prompt"},
		),
		engine.MkTool("update_agent",
			"Update an existing agent. Only the id is required — all other fields are optional and will merge with existing values.",
			map[string]any{
				"id":               map[string]any{"type": "integer", "description": "Numeric ID of the agent to update."},
				"name":             map[string]any{"type": "string", "description": "New name for the agent."},
				"system_prompt":    map[string]any{"type": "string", "description": "New system prompt."},
				"desc":             map[string]any{"type": "string", "description": "New description."},
				"tools":            map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "New tool list (replaces existing)."},
				"default_model_id": map[string]any{"type": "string", "description": "New default model ID."},
				"mcp_servers":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "New MCP server list (replaces existing)."},
				"worker_agents":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "New worker agent list (replaces existing)."},
			},
			[]string{"id"},
		),
		engine.MkTool("delete_agent",
			"Delete an agent by its numeric ID.",
			map[string]any{
				"id": map[string]any{"type": "integer", "description": "Numeric ID of the agent to delete."},
			},
			[]string{"id"},
		),
		engine.MkTool("list_agents",
			"List all configured AI agents with their IDs, names, and descriptions.",
			nil, nil,
		),
		engine.MkTool("create_mcp_server",
			"Create a new MCP server connection for extending AI capabilities with external tools.",
			map[string]any{
				"name":    map[string]any{"type": "string", "description": "Unique name for the MCP server."},
				"type":    map[string]any{"type": "string", "enum": []string{"stdio", "http"}, "description": "Connection type: stdio or http."},
				"command": map[string]any{"type": "string", "description": "Shell command to start the server (for stdio type)."},
				"args":    map[string]any{"type": "string", "description": "JSON array of command arguments (for stdio type)."},
				"env":     map[string]any{"type": "string", "description": "JSON object of environment variables (for stdio type)."},
				"url":     map[string]any{"type": "string", "description": "Server URL (for http type)."},
				"headers": map[string]any{"type": "string", "description": "JSON object of HTTP headers (for http type)."},
			},
			[]string{"name", "type"},
		),
		engine.MkTool("delete_mcp_server",
			"Delete an MCP server by its numeric ID.",
			map[string]any{
				"id": map[string]any{"type": "integer", "description": "Numeric ID of the MCP server to delete."},
			},
			[]string{"id"},
		),
		engine.MkTool("list_mcp_servers",
			"List all configured MCP servers with their IDs, names, types, and connection status.",
			nil, nil,
		),
		engine.MkTool("create_model",
			"Create a new LLM model profile for use by agents.",
			map[string]any{
				"id":               map[string]any{"type": "string", "description": "Unique model profile ID (e.g. 'openai-gpt4')."},
				"base_url":         map[string]any{"type": "string", "description": "API base URL (e.g. https://api.openai.com/v1)."},
				"api_key":          map[string]any{"type": "string", "description": "API key for authentication."},
				"model":            map[string]any{"type": "string", "description": "Model name (e.g. gpt-4o)."},
				"vision":           map[string]any{"type": "boolean", "description": "Whether the model supports image input."},
				"context_limit":    map[string]any{"type": "integer", "description": "Context window size in tokens. Default 128000."},
				"reasoning_effort": map[string]any{"type": "string", "description": "Reasoning effort: off, low, medium, high, max. Default 'high'."},
			},
			[]string{"id", "base_url", "api_key", "model"},
		),
		engine.MkTool("list_resources",
			"List system resources of a given type. Use to discover available runtimes, models, or knowledge entries.",
			map[string]any{
				"type": map[string]any{"type": "string", "enum": []string{"runtimes", "models", "knowledge"}, "description": "Resource type to list."},
			},
			[]string{"type"},
		),
	}
}

// RegisterManagementTools registers all management tool handlers on the engine.
func RegisterManagementTools(eng *engine.Engine) {
	eng.RegisterTool("create_agent", createAgentHandler)
	eng.RegisterTool("update_agent", updateAgentHandler)
	eng.RegisterTool("delete_agent", deleteAgentHandler)
	eng.RegisterTool("list_agents", listAgentsHandler)
	eng.RegisterTool("create_mcp_server", createMCPServerHandler)
	eng.RegisterTool("delete_mcp_server", deleteMCPServerHandler)
	eng.RegisterTool("list_mcp_servers", listMCPServersHandler)
	eng.RegisterTool("create_model", createModelHandler)
	eng.RegisterTool("list_resources", listResourcesHandler)
}
