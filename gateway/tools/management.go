package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"beleader/gateway/db"
	"beleader/gateway/engine"
)

// RegisterManagementTools registers agent/model/MCP management tool handlers.
func RegisterManagementTools(r *Router) {
	r.RegisterLocal("create_agent", createAgentHandler)
	r.RegisterLocal("list_agents", listAgentsHandler)
	r.RegisterLocal("update_agent", updateAgentHandler)
	r.RegisterLocal("delete_agent", deleteAgentHandler)
	r.RegisterLocal("create_model", createModelHandler)
	r.RegisterLocal("list_resources", listResourcesHandler)
	r.RegisterLocal("create_mcp_server", createMCPServerHandler)
	r.RegisterLocal("delete_mcp_server", deleteMCPServerHandler)
	r.RegisterLocal("list_mcp_servers", listMCPServersHandler)
}

func createAgentHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		Name           string   `json:"name"`
		SystemPrompt   string   `json:"system_prompt"`
		Desc           string   `json:"desc"`
		Tools          []string `json:"tools"`
		DefaultModelID string   `json:"default_model_id"`
		MCPServers     []string `json:"mcp_servers"`
		WorkerAgents   []string `json:"worker_agents"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Name == "" {
		return &engine.ToolResult{Error: "name is required"}
	}
	if p.SystemPrompt == "" {
		return &engine.ToolResult{Error: "system_prompt is required"}
	}
	toolsJSON := "[]"
	if len(p.Tools) > 0 {
		b, _ := json.Marshal(p.Tools)
		toolsJSON = string(b)
	}
	mcpJSON := "[]"
	if len(p.MCPServers) > 0 {
		b, _ := json.Marshal(p.MCPServers)
		mcpJSON = string(b)
	}
	workerJSON := "[]"
	if len(p.WorkerAgents) > 0 {
		b, _ := json.Marshal(p.WorkerAgents)
		workerJSON = string(b)
	}
	if err := globalDB.CreateAgent(p.Name, p.Desc, p.SystemPrompt, toolsJSON, p.DefaultModelID, mcpJSON, workerJSON); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Agent created: " + p.Name}
}

func listAgentsHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	agents, err := globalDB.ListAgents()
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	b, _ := json.MarshalIndent(agents, "", "  ")
	return &engine.ToolResult{Content: string(b)}
}

func updateAgentHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		ID           int64  `json:"id"`
		Name         string `json:"name"`
		SystemPrompt string `json:"system_prompt"`
		Desc         string `json:"desc"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.ID == 0 {
		return &engine.ToolResult{Error: "id is required"}
	}
	// Get existing to merge
	existing, err := globalDB.GetAgent(p.ID)
	if err != nil {
		return &engine.ToolResult{Error: fmt.Sprintf("agent %d not found", p.ID)}
	}
	name := p.Name
	if name == "" {
		name = existing.Name
	}
	sp := p.SystemPrompt
	if sp == "" {
		sp = existing.SystemPrompt
	}
	desc := p.Desc
	if desc == "" {
		desc = existing.Desc
	}
	if err := globalDB.UpdateAgent(p.ID, name, desc, sp, existing.Tools, existing.DefaultModelID, existing.MCPServers, existing.WorkerAgents); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Agent updated."}
}

func deleteAgentHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct{ ID int64 `json:"id"` }
	json.Unmarshal([]byte(args), &p)
	if p.ID == 0 {
		return &engine.ToolResult{Error: "id is required"}
	}
	if err := globalDB.DeleteAgent(p.ID); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Agent deleted."}
}

func createModelHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		ID              string `json:"id"`
		BaseURL         string `json:"base_url"`
		APIKey          string `json:"api_key"`
		Model           string `json:"model"`
		Vision          bool   `json:"vision"`
		ContextLimit    int    `json:"context_limit"`
		ReasoningEffort string `json:"reasoning_effort"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.ID == "" {
		return &engine.ToolResult{Error: "id is required"}
	}
	if p.BaseURL == "" {
		return &engine.ToolResult{Error: "base_url is required"}
	}
	if p.APIKey == "" {
		return &engine.ToolResult{Error: "api_key is required"}
	}
	if p.Model == "" {
		return &engine.ToolResult{Error: "model is required"}
	}
	if p.ContextLimit == 0 {
		p.ContextLimit = 128000
	}
	if p.ReasoningEffort == "" {
		p.ReasoningEffort = "high"
	}
	m := &db.ModelProfile{
		ModelID:         strings.TrimSpace(p.ID),
		BaseURL:         p.BaseURL,
		APIKey:          p.APIKey,
		Model:           p.Model,
		Vision:          p.Vision,
		ContextLimit:    p.ContextLimit,
		ReasoningEffort: p.ReasoningEffort,
	}
	if err := globalDB.CreateModel(m); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Model created: " + p.ID}
}

func listResourcesHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct{ Type string `json:"type"` }
	json.Unmarshal([]byte(args), &p)
	switch p.Type {
	case "models":
		models, err := globalDB.ListModels()
		if err != nil {
			return &engine.ToolResult{Error: err.Error()}
		}
		b, _ := json.MarshalIndent(models, "", "  ")
		return &engine.ToolResult{Content: string(b)}
	case "runtimes":
		agents, err := globalDB.ListToolAgents()
		if err != nil {
			return &engine.ToolResult{Error: err.Error()}
		}
		b, _ := json.MarshalIndent(agents, "", "  ")
		return &engine.ToolResult{Content: string(b)}
	default:
		return &engine.ToolResult{Error: "unknown resource type: " + p.Type + " (valid: models, runtimes)"}
	}
}

func createMCPServerHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		Name    string `json:"name"`
		Type    string `json:"type"`
		Command string `json:"command"`
		Args    string `json:"args"`
		Env     string `json:"env"`
		URL     string `json:"url"`
		Headers string `json:"headers"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Name == "" {
		return &engine.ToolResult{Error: "name is required"}
	}
	if p.Type != "stdio" && p.Type != "http" {
		return &engine.ToolResult{Error: "type must be 'stdio' or 'http'"}
	}
	s := &db.MCPServer{
		Name:    p.Name,
		Type:    p.Type,
		Enabled: true,
		Command: p.Command,
		Args:    defaultIfEmpty(p.Args, "[]"),
		Env:     defaultIfEmpty(p.Env, "{}"),
		URL:     p.URL,
		Headers: defaultIfEmpty(p.Headers, "{}"),
		Status:  "disconnected",
	}
	if thread != nil {
		s.PoolID = thread.PoolID
	}
	if err := globalDB.CreateMCPServer(s); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "MCP server created: " + p.Name}
}

func deleteMCPServerHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct{ ID int64 `json:"id"` }
	json.Unmarshal([]byte(args), &p)
	if p.ID == 0 {
		return &engine.ToolResult{Error: "id is required"}
	}
	if err := globalDB.DeleteMCPServer(p.ID); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "MCP server deleted."}
}

func listMCPServersHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	servers, err := globalDB.ListMCPServers()
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	b, _ := json.MarshalIndent(servers, "", "  ")
	return &engine.ToolResult{Content: string(b)}
}

func defaultIfEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
