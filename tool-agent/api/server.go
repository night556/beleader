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

func StartRegistration(gatewayURL, token, poolName, myURL, workspaceRoot string, restrict bool, env map[string]string, toolDefs []json.RawMessage) chan struct{} {
	client := &http.Client{Timeout: 10 * time.Second}

	register := func() (int64, error) {
		body, _ := json.Marshal(registerRequest{
			Name:              poolName + "-" + hostname(),
			URL:               myURL,
			Token:             token,
			Pool:              poolName,
			WorkspaceRoot:     workspaceRoot,
			RestrictWorkspace: restrict,
			Env:               env,
			ToolDefs:          toolDefs,
		})
		resp, err := client.Post(gatewayURL+"/api/tool-agents/register", "application/json", strings.NewReader(string(body)))
		if err != nil {
			return 0, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return 0, fmt.Errorf("register failed: %s", resp.Status)
		}
		var result map[string]any
		json.NewDecoder(resp.Body).Decode(&result)
		if id, ok := result["id"].(float64); ok {
			return int64(id), nil
		}
		return 0, nil
	}

	sendHeartbeat := func(id int64, status string) error {
		body, _ := json.Marshal(heartbeatRequest{ID: id, Status: status})
		req, _ := http.NewRequest("POST", gatewayURL+"/api/tool-agents/heartbeat", strings.NewReader(string(body)))
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}

	done := make(chan struct{})

	go func() {
		var agentID int64
		for i := 0; i < 10; i++ {
			id, err := register()
			if err == nil {
				agentID = id
				log.Printf("[registration] registered as %q (id=%d) at %s", poolName, agentID, myURL)
				break
			}
			log.Printf("[registration] attempt %d/10 failed: %v", i+1, err)
			select {
			case <-done:
				return
			case <-time.After(time.Duration(i+1) * time.Second):
			}
		}
		if agentID == 0 {
			log.Printf("[registration] all attempts failed, giving up")
			return
		}

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if err := sendHeartbeat(agentID, "active"); err != nil {
					log.Printf("[registration] heartbeat failed: %v", err)
				}
			case <-done:
				log.Println("[registration] deregistering...")
				sendHeartbeat(agentID, "inactive")
				return
			}
		}
	}()

	return done
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
