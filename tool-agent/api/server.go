package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"beleader/tool-agent/tools"
)

// MCPManagerInterface allows server.go to call MCPManager without circular import.
type MCPManagerInterface interface {
	ConnectAll(configs []tools.MCPServerConfig) []json.RawMessage
}

type ExecuteRequest struct {
	ThreadID  string          `json:"thread_id"`
	Workspace string          `json:"workspace"`
	Tool      string          `json:"tool"`
	Args      json.RawMessage `json:"args"`
}

type Server struct {
	WorkspaceRoot     string
	RestrictWorkspace bool
}

func NewServer(workspaceRoot string, restrict bool) *Server {
	return &Server{
		WorkspaceRoot:     workspaceRoot,
		RestrictWorkspace: restrict,
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		w.WriteHeader(204)
		return
	}

	path := r.URL.Path
	switch {
	case r.Method == "POST" && path == "/execute":
		s.handleExecute(w, r)
	case r.Method == "POST" && path == "/workspace/init":
		s.handleWorkspaceInit(w, r)
	case r.Method == "POST" && path == "/workspace/cleanup":
		s.handleWorkspaceCleanup(w, r)
	case r.Method == "POST" && path == "/mcp/test":
		s.handleMCPTest(w, r)
	case r.Method == "GET" && path == "/tools":
		s.handleListTools(w, r)
	case r.Method == "GET" && path == "/health":
		w.WriteHeader(200)
		fmt.Fprint(w, "ok")
	default:
		w.WriteHeader(404)
		fmt.Fprint(w, "not found")
	}
}

func (s *Server) handleExecute(w http.ResponseWriter, r *http.Request) {
	var req ExecuteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, 400, err.Error())
		return
	}

	if req.Tool == "" {
		jsonError(w, 400, "tool is required")
		return
	}

	// Determine workspace
	workspace := req.Workspace
	if workspace == "" {
		workspace = filepath.Join(s.WorkspaceRoot, "threads", req.ThreadID, "workspace")
	}

	result := tools.ExecuteTool(req.Tool, string(req.Args), workspace, s.WorkspaceRoot, s.RestrictWorkspace, req.ThreadID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (s *Server) handleWorkspaceInit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	if req.ThreadID == "" {
		jsonError(w, 400, "thread_id is required")
		return
	}

	wsPath := filepath.Join(s.WorkspaceRoot, "threads", req.ThreadID, "workspace")
	if err := os.MkdirAll(wsPath, 0755); err != nil {
		jsonError(w, 500, err.Error())
		return
	}

	// Also create trash dir
	os.MkdirAll(filepath.Join(s.WorkspaceRoot, "threads", req.ThreadID, ".trash"), 0755)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"workspace": wsPath,
	})
}

func (s *Server) handleWorkspaceCleanup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ThreadID string `json:"thread_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, 400, err.Error())
		return
	}

	threadDir := filepath.Join(s.WorkspaceRoot, "threads", req.ThreadID)
	if err := os.RemoveAll(threadDir); err != nil {
		jsonError(w, 500, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"status": "cleaned"})
}

func (s *Server) handleListTools(w http.ResponseWriter, r *http.Request) {
	defs := tools.GetToolDefs()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"tools": defs})
}

func (s *Server) handleMCPTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, 400, err.Error())
		return
	}
	if req.URL == "" {
		jsonError(w, 400, "url is required")
		return
	}

	count, names, err := tools.TestMCPConnection(req.URL, req.Headers)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"success": false, "error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"success": true, "tool_count": count, "tools": names})
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ── Registration ──

type registerRequest struct {
	Name              string            `json:"name"`
	URL               string            `json:"url"`
	Token             string            `json:"token"`
	Pool              string            `json:"pool"`
	WorkspaceRoot     string            `json:"workspace_root"`
	RestrictWorkspace bool              `json:"restrict_workspace"`
	Env               map[string]string `json:"env"`
	ToolDefs          []json.RawMessage `json:"tool_defs"`
}

type heartbeatRequest struct {
	ID     int64  `json:"id"`
	Status string `json:"status"`
}

type registerResponse struct {
	ID         int64                    `json:"id"`
	MCPServers []tools.MCPServerConfig  `json:"mcp_servers"`
}

// doRegister sends a single registration request. Returns the assigned agent ID
// and any MCP server configs for the pool. Does NOT connect to MCP servers.
func doRegister(client *http.Client, gatewayURL string, req registerRequest) (*registerResponse, error) {
	body, _ := json.Marshal(req)
	resp, err := client.Post(gatewayURL+"/api/tool-agents/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("register failed: %s", resp.Status)
	}
	var result registerResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode register response: %w", err)
	}
	return &result, nil
}

// connectMCP connects to MCP servers, updates global tool defs, and re-registers
// the combined (built-in + MCP) tool list with the gateway.
func connectMCP(client *http.Client, gatewayURL string, req registerRequest, mcpMgr MCPManagerInterface, configs []tools.MCPServerConfig) {
	combinedDefs := mcpMgr.ConnectAll(configs)
	req.ToolDefs = combinedDefs
	body, _ := json.Marshal(req)
	resp, err := client.Post(gatewayURL+"/api/tool-agents/register", "application/json", strings.NewReader(string(body)))
	if err == nil {
		resp.Body.Close()
	}
}

// doHeartbeat sends a heartbeat and returns MCP version stamps.
func doHeartbeat(client *http.Client, gatewayURL string, id int64, status string) (map[string]string, error) {
	body, _ := json.Marshal(heartbeatRequest{ID: id, Status: status})
	req, _ := http.NewRequest("POST", gatewayURL+"/api/tool-agents/heartbeat", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		MCPVersions map[string]string `json:"mcp_versions"`
	}
	json.NewDecoder(resp.Body).Decode(&result)
	return result.MCPVersions, nil
}

// mapsEqual compares two string maps for equality.
func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// initialRegister retries registration up to maxAttempts times, then connects
// to MCP servers and re-registers the combined tool list.
func initialRegister(client *http.Client, gatewayURL string, req registerRequest, mcpMgr MCPManagerInterface, maxAttempts int, done chan struct{}) int64 {
	for i := 0; i < maxAttempts; i++ {
		result, err := doRegister(client, gatewayURL, req)
		if err == nil {
			log.Printf("[registration] registered as %q (id=%d) at %s", req.Pool, result.ID, req.URL)

			// Phase 2: connect MCP servers and report combined tools
			if mcpMgr != nil && len(result.MCPServers) > 0 {
				connectMCP(client, gatewayURL, req, mcpMgr, result.MCPServers)
			}
			return result.ID
		}
		log.Printf("[registration] attempt %d/%d failed: %v", i+1, maxAttempts, err)
		select {
		case <-done:
			return 0
		case <-time.After(time.Duration(i+1) * time.Second):
		}
	}
	log.Printf("[registration] all %d attempts failed, giving up", maxAttempts)
	return 0
}

// runHeartbeatLoop sends periodic heartbeats and reconnects MCP servers when
// their configuration changes on the gateway side.
func runHeartbeatLoop(client *http.Client, gatewayURL string, agentID int64, req registerRequest, mcpMgr MCPManagerInterface, done chan struct{}) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	lastVersions := map[string]string{}

	for {
		select {
		case <-ticker.C:
			versions, err := doHeartbeat(client, gatewayURL, agentID, "active")
			if err != nil {
				log.Printf("[registration] heartbeat failed: %v", err)
				continue
			}

			if mapsEqual(lastVersions, versions) {
				continue
			}

			log.Printf("[registration] MCP config changed, reconnecting")
			lastVersions = versions

			// Re-register to get fresh MCP configs, then reconnect
			result, err := doRegister(client, gatewayURL, req)
			if err != nil {
				log.Printf("[registration] re-register failed: %v", err)
				continue
			}
			if len(result.MCPServers) > 0 && mcpMgr != nil {
				connectMCP(client, gatewayURL, req, mcpMgr, result.MCPServers)
			}

		case <-done:
			log.Println("[registration] deregistering...")
			doHeartbeat(client, gatewayURL, agentID, "inactive")
			return
		}
	}
}

func StartRegistration(gatewayURL, token, poolName, myURL, workspaceRoot string, restrict bool, env map[string]string, toolDefs []json.RawMessage, mcpMgr MCPManagerInterface) chan struct{} {
	client := &http.Client{Timeout: 10 * time.Second}

	req := registerRequest{
		Name:              poolName + "-" + hostname(),
		URL:               myURL,
		Token:             token,
		Pool:              poolName,
		WorkspaceRoot:     workspaceRoot,
		RestrictWorkspace: restrict,
		Env:               env,
		ToolDefs:          toolDefs,
	}

	done := make(chan struct{})

	go func() {
		agentID := initialRegister(client, gatewayURL, req, mcpMgr, 10, done)
		if agentID == 0 {
			return
		}
		runHeartbeatLoop(client, gatewayURL, agentID, req, mcpMgr, done)
	}()

	return done
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
