package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"io/fs"

	"beleader/backend/config"
	"beleader/backend/db"
	"beleader/backend/llm"
	"beleader/backend/mcp"
	"beleader/backend/session"
	"beleader/backend/tools"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

type Handler struct {
	DB              *db.DB
	WebFS           fs.FS           // Set by main.go: embedded web/ filesystem
	LLM             *llm.Client
	SessionMgr      *session.Manager
	Config          *config.Config
	SSE             *SSEBroker
	MCPMgr          *mcp.Manager
	hcSlots         chan struct{}
	hcSessions      sync.Map
	pauseChs        map[string]chan struct{}
	interveneChs    map[string]chan session.InterveneMsg
	cancelFuncs     map[string]context.CancelFunc
	pauseMu         sync.Mutex
	clients         map[string]*llm.Client
	clientsMu       sync.RWMutex
	observers       []SessionObserver
}

func (h *Handler) RegisterObserver(o SessionObserver) {
	h.observers = append(h.observers, o)
}

func (h *Handler) Notify(e SessionEvent) {
	for _, o := range h.observers {
		o.OnSessionEvent(e)
	}
}

func NewHandler(database *db.DB, llmClient *llm.Client, cfg *config.Config) *Handler {
	h := &Handler{
		DB:           database,
		LLM:          llmClient,
		Config:       cfg,
		SSE:          NewSSEBroker(),
		hcSlots:      make(chan struct{}, cfg.HC.Max),
		pauseChs:     make(map[string]chan struct{}),
		interveneChs: make(map[string]chan session.InterveneMsg),
		cancelFuncs:  make(map[string]context.CancelFunc),
	}
	h.SessionMgr = session.NewManager(database, llmClient, session.Config{
		WorkDir:       cfg.WorkDir,
		StatePath:     "",
		MaxContextPct: cfg.Thresholds.MaxContextPct,
	})
	h.clients = make(map[string]*llm.Client)
	h.RegisterObserver(h.SSE)


	// Reset stale agent & session statuses from previous run
	h.DB.ResetAllAgentStatuses()
	h.DB.ResetAllSessionStatuses()

	return h
}

func (h *Handler) SetStaticFS(f fs.FS) {
	h.WebFS = f
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// Serve embedded web files with SPA fallback (index.html)
	if h.WebFS != nil {
		webFS := h.WebFS
		fileServer := http.FileServer(http.FS(webFS))
		r.NoRoute(func(c *gin.Context) {
			path := c.Request.URL.Path
			_, err := fs.Stat(webFS, strings.TrimPrefix(path, "/"))
			if err == nil {
				c.Request.URL.Path = path
				fileServer.ServeHTTP(c.Writer, c.Request)
			} else {
				c.Request.URL.Path = "/"
				fileServer.ServeHTTP(c.Writer, c.Request)
			}
		})
	}

	api := r.Group("/api")
	{
		api.POST("/chat", h.handleChat)
		api.GET("/sse", h.handleSSE)
		api.GET("/projects", h.handleListProjects)
		api.POST("/projects", h.handleCreateProject)
		api.GET("/projects/:id", h.handleGetProject)
		api.DELETE("/projects/:id", h.handleDeleteProject)
		api.POST("/projects/:id/pause", h.handlePause)
		api.POST("/projects/:id/resume", h.handleResume)
		api.POST("/projects/:id/intervene", h.handleIntervene)
		api.GET("/settings", h.handleGetSettings)
		api.PUT("/settings", h.handleUpdateSettings)
		api.GET("/messages", h.handleGetMessages)
		api.GET("/messages/bookmarked", h.handleGetBookmarkedMessages)
		api.PUT("/messages/:id/bookmark", h.handleBookmarkMessage)
		api.POST("/sessions/:id/clear", h.handleClearContext)
		api.POST("/sessions/:id/stop", h.handleStop)
		api.POST("/sessions/:id/model", h.handleSwitchModel)
		api.GET("/sessions/:id/model", h.handleGetSessionModel)
		api.GET("/tools", h.handleListTools)
		api.GET("/agents", h.handleListAgents)
		api.POST("/agents", h.handleCreateAgent)
		api.PUT("/agents/desc", h.handleUpdateAgentDesc)
		api.PUT("/agents/:id", h.handleUpdateAgent)
		api.DELETE("/agents/:id", h.handleDeleteAgent)
		api.GET("/sessions/:id/tokens", h.handleGetSessionTokens)
		api.GET("/knowledge", h.handleListKnowledge)
		api.GET("/knowledge/search", h.handleSearchKnowledge)
		api.PUT("/knowledge/:id", h.handleUpdateKnowledge)
		api.DELETE("/knowledge/:id", h.handleDeleteKnowledge)

		api.GET("/mcp/servers", h.handleListMCPServers)
		api.POST("/mcp/servers", h.handleCreateMCPServer)
		api.PUT("/mcp/servers/:id", h.handleUpdateMCPServer)
		api.DELETE("/mcp/servers/:id", h.handleDeleteMCPServer)
		api.POST("/mcp/servers/:id/test", h.handleTestMCPServer)
		api.POST("/mcp/servers/:id/connect", h.handleConnectMCPServer)
		api.POST("/mcp/servers/:id/disconnect", h.handleDisconnectMCPServer)

	}
}

func (h *Handler) handleChat(c *gin.Context) {
	var req struct {
		Message string   `json:"message"`
		Images  []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Message == "" && len(req.Images) == 0) {
		c.JSON(400, gin.H{"error": "message required"})
		return
	}

	mainSess, err := h.DB.GetSession("main")
	if err != nil {
		h.DB.CreateSession("main", "running")
	} else if mainSess.Status != "running" {
		h.DB.UpdateSessionStatus("main", "running")
	}

	tools.BrowserHeadless = h.Config.Browser.Headless
	tools.BrowserProfileDir = h.Config.BrowserProfileDir()
	// Main is the platform controller: read-only file tools, web, project/agent/knowledge/MCP management.
	tools.RegisterReadFile(h.SessionMgr)
	tools.RegisterReadDir(h.SessionMgr)
	tools.RegisterSearchContent(h.SessionMgr)
	tools.RegisterSearchFiles(h.SessionMgr)
	tools.RegisterRunCommand(h.SessionMgr, h.Config.WorkDir)
	tools.RegisterWebTools(h.SessionMgr)
	h.SessionMgr.RegisterTool("create_project", func(ctx context.Context, args string) *session.ToolResult {
		var p struct{ Title string `json:"title"`; Prompt string `json:"prompt"` }
		json.Unmarshal([]byte(args), &p)
		refID, _, err := h.CreateProject(p.Title, p.Prompt)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: fmt.Sprintf("Project created. ref_id=%s", refID)}
	})
	tools.RegisterProjectManagementTools(
		h.SessionMgr,
		h.listProjectsForLLM,
		h.getProjectStatusForLLM,
		h.searchConversationForLLM,
	)
	h.RegisterDeleteProjectTool()
	h.RegisterInterveneProjectTool()
	h.RegisterAgentTools()
	h.RegisterMCPTools()

	if len(req.Images) > 0 {
		multiContent := buildTextAndImageMultiContent(req.Message, req.Images)
		multiJSON, _ := json.Marshal(multiContent)
		h.DB.InsertMessage(&db.Message{SessionID: "main", Role: "user", Content: req.Message, MultiContent: string(multiJSON)})
	} else {
		h.DB.InsertMessage(&db.Message{SessionID: "main", Role: "user", Content: req.Message})
	}

	// If main session is already running, inject via interveneCh instead of killing it
	h.pauseMu.Lock()
	_, sessionAlive := h.pauseChs["main"]
	interveneCh, hasCh := h.interveneChs["main"]
	h.pauseMu.Unlock()

	if sessionAlive && hasCh {
		msg := session.InterveneMsg{Message: req.Message, Images: req.Images}
		select {
		case interveneCh <- msg:
		default:
		}
		h.Notify(SessionEvent{
			Type: "intervention_sent",
			Data: gin.H{"ref_id": "main", "session_id": "main", "message": req.Message, "status": "injected"},
		})
		c.JSON(200, gin.H{"status": "ok"})
		return
	}

	go h.runSession("main", "", h.Config.WorkDir, "", RunSessionOpts{AgentType: "main"})

	c.JSON(200, gin.H{"status": "processing"})
}

func (h *Handler) cancelSession(sessionID string) {
	h.pauseMu.Lock()
	if cancel, ok := h.cancelFuncs[sessionID]; ok {
		cancel()
		delete(h.cancelFuncs, sessionID)
	}
	h.pauseMu.Unlock()
}

func (h *Handler) getCoordinatorSessionID(ref *db.ProjectRef) string {
	for _, a := range ref.Agents {
		if a.Role == "coordinator" {
			return a.SessionID
		}
	}
	return ""
}

func (h *Handler) getWorkerBySessionID(sessionID string) (*db.ProjectAgent, error) {
	return h.DB.GetProjectAgent(sessionID)
}

func (h *Handler) getWorkerByName(refID, workerName string) (*db.ProjectAgent, error) {
	ref, err := h.DB.GetProject(refID)
	if err != nil {
		return nil, err
	}
	for _, a := range ref.Agents {
		if a.Name == workerName && a.Role == "worker" {
			return &a, nil
		}
	}
	return nil, fmt.Errorf("worker '%s' not found in project", workerName)
}

// spawnWorker creates a Worker session and starts its RunLoop.
func (h *Handler) spawnWorker(coordinatorSessionID, refID, agentName, name, task string, enableBrowser bool) (string, error) {
	// Check if a worker with this name already exists
	if existing, _ := h.getWorkerByName(refID, name); existing != nil {
		if existing.Status == "running" {
			return "", fmt.Errorf("Worker '%s' is currently running. Options: (1) Wait for it to finish. (2) Use intervene_worker to check progress or add follow-up work. (3) If you need a parallel worker for a different task, spawn with a different name (e.g. '%s-2', '%s-backend').", name, name, name)
		}
		return "", fmt.Errorf("Worker '%s' already exists and is idle. Options: (1) Use intervene_worker to give it a new task (it keeps its context). (2) If you need a fresh worker, use delete_worker first then spawn again. (3) Spawn with a different name for a separate role.", name)
	}

	agentType := "worker"
	var toolNames []string

	agent, err := h.DB.GetAgentByName(agentName)
	if err != nil {
		return "", fmt.Errorf("no agent template named '%s'. Use list_agents to see available templates.", agentName)
	}
	customPrompt := agent.Content
	json.Unmarshal([]byte(agent.Tools), &toolNames)

	workerSessionID := uuid.New().String()
	workDir := h.Config.ProjectDir(refID)

	h.DB.CreateSession(workerSessionID, "running")
	h.acquireHC(workerSessionID)
	h.DB.AddProjectAgent(refID, name, workerSessionID, agentType, customPrompt, agentName, enableBrowser)

	h.Notify(SessionEvent{
		Type: "worker_spawned",
		Data: gin.H{
			"ref_id":     refID,
			"session_id": workerSessionID,
			"name":       name,
			"role":       agentType,
			"status":     "running",
		},
	})

	go h.runSession(workerSessionID, refID, workDir, task, RunSessionOpts{
		AgentType:     agentType,
		RoleLabel:     name,
		CustomPrompt:  customPrompt,
		EnableBrowser: enableBrowser,
		ToolNames:     toolNames,
	})

	return fmt.Sprintf("Worker '%s' spawned. session_id=%s", name, workerSessionID), nil
}

// terminateWorker removes a Worker from the project.
func (h *Handler) terminateWorker(refID, workerName string) (string, error) {
	agent, err := h.getWorkerByName(refID, workerName)
	if err != nil {
		return "", err
	}

	h.cancelSession(agent.SessionID)
	h.DB.UpdateSessionStatus(agent.SessionID, "idle")
	h.DB.UpdateProjectAgentStatus(agent.Name, agent.SessionID, "idle")
	h.releaseHC(agent.SessionID)

	h.Notify(SessionEvent{
		Type: "worker_terminated",
		Data: gin.H{"ref_id": refID, "worker_name": workerName, "session_id": agent.SessionID},
	})

	return fmt.Sprintf("Worker '%s' terminated.", workerName), nil
}

// deleteWorker removes a Worker and all traces (session, messages, agent record).
func (h *Handler) deleteWorker(refID, workerName string) (string, error) {
	agent, err := h.getWorkerByName(refID, workerName)
	if err != nil {
		return "", err
	}

	h.cancelSession(agent.SessionID)
	h.releaseHC(agent.SessionID)
	h.DB.RemoveProjectAgent(agent.SessionID)
	h.DB.DeleteMessages(agent.SessionID)
	h.DB.DeleteSession(agent.SessionID)

	h.Notify(SessionEvent{
		Type: "worker_deleted",
		Data: gin.H{"ref_id": refID, "worker_name": workerName, "session_id": agent.SessionID},
	})

	return fmt.Sprintf("Worker '%s' deleted.", workerName), nil
}

// interveneWorker sends a message to a Worker, restarting it if idle.
func (h *Handler) interveneWorker(refID, workerName, message string) (string, error) {
	agent, err := h.getWorkerByName(refID, workerName)
	if err != nil {
		return "", err
	}
	sid := agent.SessionID

	h.pauseMu.Lock()
	_, running := h.pauseChs[sid]
	interveneCh, hasCh := h.interveneChs[sid]
	h.pauseMu.Unlock()

	msg := session.InterveneMsg{Message: message}

	if running && hasCh {
		select {
		case interveneCh <- msg:
		default:
		}
		h.Notify(SessionEvent{
			Type: "worker_intervened",
			Data: gin.H{"ref_id": refID, "session_id": sid, "worker_name": workerName, "message": message, "status": "injected"},
		})
		return fmt.Sprintf("Message sent to Worker '%s'.", workerName), nil
	}

	// Worker idle 闁?restart RunLoop
	h.DB.InsertMessage(&db.Message{SessionID: sid, Role: "user", Content: message})
	h.acquireHC(sid)
	h.DB.ResumeSession(sid)
	h.DB.UpdateProjectAgentStatus(workerName, sid, "running")

	workDir := h.Config.ProjectDir(refID)
	go h.runSession(sid, refID, workDir, "", RunSessionOpts{
			AgentType:     agent.Role,
			RoleLabel:     workerName,
			CustomPrompt:  agent.Prompt,
			EnableBrowser: agent.EnableBrowser,
				ToolNames:     h.resolveToolNames(agent.AgentTemplate),
		})

	h.Notify(SessionEvent{
		Type: "worker_intervened",
		Data: gin.H{"ref_id": refID, "session_id": sid, "worker_name": workerName, "message": message, "status": "restarted"},
	})
	return fmt.Sprintf("Worker '%s' restarted and message sent.", workerName), nil
}

// listWorkers returns all Workers in a project with their name, status, and role summary.
func (h *Handler) listWorkers(refID string) (string, error) {
	agents, err := h.DB.GetProjectAgents(refID)
	if err != nil {
		return "", err
	}
	if len(agents) == 0 {
		return "No Workers in this project.", nil
	}
	var lines []string
	for _, a := range agents {
		summary := a.Prompt
		if len(summary) > 60 {
			summary = summary[:57] + "..."
		}
		lines = append(lines, fmt.Sprintf("- **%s** [%s]: %s", a.Name, a.Status, summary))
	}
	return strings.Join(lines, "\n"), nil
}

func parseFirstToolName(tcJSON string) string {
	if tcJSON == "" || tcJSON == "[]" {
		return ""
	}
	var tcs []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(tcJSON), &tcs); err != nil || len(tcs) == 0 {
		return ""
	}
	return tcs[0].Function.Name
}

func (h *Handler) handleSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	ch := h.SSE.Subscribe()
	defer h.SSE.Unsubscribe(ch)

	// Flush headers immediately so the browser fires the EventSource 'open'
	// event without waiting for the first broadcast message.
	fmt.Fprintf(c.Writer, ":ok\n\n")
	c.Writer.Flush()

	notify := c.Request.Context().Done()
	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(c.Writer, "data: %s\n\n", msg)
			c.Writer.Flush()
		case <-notify:
			return
		}
	}
}

func (h *Handler) handleListProjects(c *gin.Context) {
	refs, err := h.DB.ListProjects()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, refs)
}

func (h *Handler) handleGetProject(c *gin.Context) {
	refID := c.Param("id")
	ref, err := h.DB.GetProject(refID)
	if err != nil {
		c.JSON(404, gin.H{"error": "project not found"})
		return
	}

	statusContent := ""
	if data, err := os.ReadFile(h.Config.StatusPath(refID)); err == nil {
		statusContent = string(data)
	}

	c.JSON(200, gin.H{
		"ref":    ref,
		"status": statusContent,
	})
}

func (h *Handler) handleDeleteProject(c *gin.Context) {
	refID := c.Param("id")
	result, err := h.DeleteProject(refID)
	if err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": result})
}

func (h *Handler) DeleteProject(refID string) (string, error) {
	if err := h.StopProject(refID); err != nil {
		return "", err
	}

	if err := h.DB.DeleteProjectSessions(refID); err != nil {
		return "", fmt.Errorf("delete project sessions: %w", err)
	}

	if err := h.DB.DeleteProject(refID); err != nil {
		return "", fmt.Errorf("delete project: %w", err)
	}

	os.RemoveAll(h.Config.ProjectDir(refID))

	h.Notify(SessionEvent{Type: "project_deleted", Data: gin.H{"ref_id": refID}})

	return fmt.Sprintf("Project '%s' deleted successfully.", refID), nil
}

func (h *Handler) RegisterDeleteProjectTool() {
	h.SessionMgr.RegisterTool("delete_project", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			RefID string `json:"ref_id"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.RefID == "" {
			return &session.ToolResult{Error: "ref_id required"}
		}
		result, err := h.DeleteProject(p.RefID)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	})
}

func (h *Handler) RegisterInterveneProjectTool() {
	h.SessionMgr.RegisterTool("intervene_project", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			RefID   string `json:"ref_id"`
			Message string `json:"message"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.RefID == "" || p.Message == "" {
			return &session.ToolResult{Error: "ref_id and message required"}
		}

		ref, err := h.DB.GetProject(p.RefID)
		if err != nil {
			return &session.ToolResult{Error: "project not found: " + err.Error()}
		}

		sid := h.getCoordinatorSessionID(ref)
		if sid == "" {
			return &session.ToolResult{Error: "no coordinator session found for project"}
		}

		h.insertUserMessage(sid, p.Message, nil)

		h.pauseMu.Lock()
		_, sessionAlive := h.pauseChs[sid]
		interveneCh, hasCh := h.interveneChs[sid]
		h.pauseMu.Unlock()

		if sessionAlive && hasCh {
			msg := session.InterveneMsg{Message: p.Message}
			select {
			case interveneCh <- msg:
			default:
			}
			h.Notify(SessionEvent{
				Type: "intervention_sent",
				Data: gin.H{"ref_id": p.RefID, "session_id": sid, "message": p.Message, "status": "injected"},
			})
			return &session.ToolResult{Content: fmt.Sprintf("Intervention sent to project %s: %s", p.RefID, p.Message)}
		}

		if ref.Status == "idle" {
			workDir := h.Config.ProjectDir(p.RefID)
			h.acquireHC(sid)
			h.DB.ResumeSession(sid)
			h.DB.UpdateProjectStatus(p.RefID, "running")
			go h.runSession(sid, p.RefID, workDir, "", RunSessionOpts{AgentType: "coordinator", RoleLabel: "coordinator"})
			h.Notify(SessionEvent{
				Type: "intervention_sent",
				Data: gin.H{"ref_id": p.RefID, "session_id": sid, "message": p.Message, "status": "session restarted"},
			})
			return &session.ToolResult{Content: fmt.Sprintf("Coordinator restarted for project %s with message: %s", p.RefID, p.Message)}
		}

		return &session.ToolResult{Content: fmt.Sprintf("Message sent to project %s", p.RefID)}
	})
}

// RegisterMCPTools registers MCP management tools on the shared SessionMgr for main.
func (h *Handler) RegisterMCPTools() {
	tools.RegisterMCPTools(
		h.SessionMgr,
		func() (string, error) {
			servers, err := h.DB.ListMCPServers()
			if err != nil {
				return "", err
			}
			if h.MCPMgr != nil {
				connected := h.MCPMgr.GetConnectedServers()
				connectedSet := make(map[string]bool, len(connected))
				for _, name := range connected {
					connectedSet[name] = true
				}
				for i := range servers {
					if connectedSet[servers[i].Name] {
						servers[i].Status = "connected"
					}
				}
			}
			b, _ := json.Marshal(servers)
			return string(b), nil
		},
		func(name, mcpType, command, args, env, url, headers string, enabled bool) (string, error) {
			s := db.MCPServer{
				Name: name, Type: mcpType, Enabled: enabled,
				Command: command, Args: args, Env: env,
				URL: url, Headers: headers,
			}
			if err := h.DB.CreateMCPServer(&s); err != nil {
				return "", err
			}
			if enabled && h.MCPMgr != nil {
				if err := h.MCPMgr.Connect(s); err != nil {
					return fmt.Sprintf("Created but connect failed: %v", err), nil
				}
				return fmt.Sprintf("MCP server '%s' created and connected.", name), nil
			}
			return fmt.Sprintf("MCP server '%s' created.", name), nil
		},
		func(name, newName, mcpType, command, args, env, url, headers string, enabled bool) (string, error) {
			servers, err := h.DB.ListMCPServers()
			if err != nil {
				return "", err
			}
			var existing *db.MCPServer
			for i := range servers {
				if servers[i].Name == name {
					existing = &servers[i]
					break
				}
			}
			if existing == nil {
				return "", fmt.Errorf("MCP server '%s' not found", name)
			}
			s := *existing
			if newName != "" {
				s.Name = newName
			}
			if mcpType != "" {
				s.Type = mcpType
			}
			if command != "" {
				s.Command = command
			}
			if args != "" {
				s.Args = args
			}
			if env != "" {
				s.Env = env
			}
			if url != "" {
				s.URL = url
			}
			if headers != "" {
				s.Headers = headers
			}
			s.Enabled = enabled
			if err := h.DB.UpdateMCPServer(&s); err != nil {
				return "", err
			}
			nameChanged := newName != "" && newName != name
			if existing.Enabled {
				if !enabled || nameChanged {
					if h.MCPMgr != nil {
						h.MCPMgr.Disconnect(existing.Name)
					}
				}
			}
			if enabled && h.MCPMgr != nil {
				h.MCPMgr.Connect(s)
			}
			return fmt.Sprintf("MCP server '%s' updated.", s.Name), nil
		},
		func(name string) (string, error) {
			servers, err := h.DB.ListMCPServers()
			if err != nil {
				return "", err
			}
			var found *db.MCPServer
			for i := range servers {
				if servers[i].Name == name {
					found = &servers[i]
					break
				}
			}
			if found == nil {
				return "", fmt.Errorf("MCP server '%s' not found", name)
			}
			if h.MCPMgr != nil {
				h.MCPMgr.Disconnect(found.Name)
			}
			if err := h.DB.DeleteMCPServer(found.ID); err != nil {
				return "", err
			}
			return fmt.Sprintf("MCP server '%s' deleted.", name), nil
		},
		func(name string) (string, error) {
			servers, err := h.DB.ListMCPServers()
			if err != nil {
				return "", err
			}
			var found *db.MCPServer
			for i := range servers {
				if servers[i].Name == name {
					found = &servers[i]
					break
				}
			}
			if found == nil {
				return "", fmt.Errorf("MCP server '%s' not found", name)
			}
			if h.MCPMgr == nil {
				return "", fmt.Errorf("MCP manager not initialized")
			}
			if err := h.MCPMgr.Connect(*found); err != nil {
				return "", err
			}
			return fmt.Sprintf("MCP server '%s' connected.", name), nil
		},
		func(name string) (string, error) {
			if h.MCPMgr == nil {
				return "", fmt.Errorf("MCP manager not initialized")
			}
			if err := h.MCPMgr.Disconnect(name); err != nil {
				return "", err
			}
			return fmt.Sprintf("MCP server '%s' disconnected.", name), nil
		},
		func(name string) (string, error) {
			servers, err := h.DB.ListMCPServers()
			if err != nil {
				return "", err
			}
			var found *db.MCPServer
			for i := range servers {
				if servers[i].Name == name {
					found = &servers[i]
					break
				}
			}
			if found == nil {
				return "", fmt.Errorf("MCP server '%s' not found", name)
			}
			if h.MCPMgr == nil {
				return "", fmt.Errorf("MCP manager not initialized")
			}
			count, names, err := h.MCPMgr.TestConnection(*found)
			if err != nil {
				return "", fmt.Errorf("test failed: %v", err)
			}
			return fmt.Sprintf("Test succeeded. %d tools: %v", count, names), nil
		},
	)
}

func (h *Handler) insertUserMessage(sessionID, message string, images []string) {
	if len(images) > 0 {
		multiContent := buildTextAndImageMultiContent(message, images)
		multiJSON, _ := json.Marshal(multiContent)
		h.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "user", Content: message, MultiContent: string(multiJSON)})
	} else {
		h.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "user", Content: message})
	}
}

func (h *Handler) handleIntervene(c *gin.Context) {
	refID := c.Param("id")
	var req struct {
		Message string   `json:"message"`
		Images  []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || (req.Message == "" && len(req.Images) == 0) {
		c.JSON(400, gin.H{"error": "message required"})
		return
	}

	ref, err := h.DB.GetProject(refID)
	if err != nil {
		c.JSON(404, gin.H{"error": "project not found"})
		return
	}

	sid := h.getCoordinatorSessionID(ref)
	if sid == "" {
		c.JSON(500, gin.H{"error": "no coordinator session found"})
		return
	}

	msg := session.InterveneMsg{Message: req.Message, Images: req.Images}

	if ref.Status == "running" {
		h.pauseMu.Lock()
		_, sessionAlive := h.pauseChs[sid]
		h.pauseMu.Unlock()

		if sessionAlive {
			select {
			case h.interveneChs[sid] <- msg:
			default:
				h.insertUserMessage(sid, req.Message, req.Images)
			}

			h.Notify(SessionEvent{
				Type: "intervention_sent",
				Data: gin.H{
					"ref_id":     refID,
					"session_id": sid,
					"message":    req.Message,
					"status":     "injected into running session",
				},
			})
			c.JSON(200, gin.H{"status": "ok"})
			return
		}
		// Session goroutine gone 闁?fall through to restart
	}

	workDir := h.Config.ProjectDir(refID)
	h.acquireHC(sid)
	h.DB.ResumeSession(sid)
	h.DB.UpdateProjectStatus(refID, "running")
	h.insertUserMessage(sid, req.Message, req.Images)

	go h.runSession(sid, refID, workDir, "", RunSessionOpts{AgentType: "coordinator", RoleLabel: "coordinator"})

	h.Notify(SessionEvent{
		Type: "intervention_sent",
		Data: gin.H{
			"ref_id":     refID,
			"session_id": sid,
			"message":    req.Message,
			"status":     "session restarted",
		},
	})

	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handlePause(c *gin.Context) {
	refID := c.Param("id")
	ref, err := h.DB.GetProject(refID)
	if err != nil {
		c.JSON(404, gin.H{"error": "project not found"})
		return
	}

	sid := h.getCoordinatorSessionID(ref)
	if sid == "" {
		c.JSON(404, gin.H{"error": "no coordinator session"})
		return
	}

	h.pauseMu.Lock()
	ch, ok := h.pauseChs[sid]
	h.pauseMu.Unlock()

	if !ok {
		c.JSON(400, gin.H{"error": "session not running"})
		return
	}

	close(ch)
	h.DB.UpdateSessionStatus(sid, "idle")
	h.DB.UpdateProjectStatus(refID, "paused")
	h.releaseHC(sid)

	h.Notify(SessionEvent{
		Type: "project_paused",
		Data: gin.H{"ref_id": refID},
	})

	c.JSON(200, gin.H{"status": "pausing"})
}

func (h *Handler) handleResume(c *gin.Context) {
	refID := c.Param("id")
	ref, err := h.DB.GetProject(refID)
	if err != nil {
		c.JSON(404, gin.H{"error": "project not found"})
		return
	}

	if ref.Status == "running" {
		c.JSON(400, gin.H{"error": "project already running"})
		return
	}
	if ref.Status != "paused" {
		c.JSON(400, gin.H{"error": "project is not paused"})
		return
	}

	sid := h.getCoordinatorSessionID(ref)
	if sid == "" {
		c.JSON(500, gin.H{"error": "no coordinator session"})
		return
	}

	workDir := h.Config.ProjectDir(refID)
	h.acquireHC(sid)
	h.DB.ResumeSession(sid)
	h.DB.UpdateProjectStatus(refID, "running")

	go h.runSession(sid, refID, workDir, "", RunSessionOpts{AgentType: "coordinator", RoleLabel: "coordinator"})

	h.Notify(SessionEvent{
		Type: "project_resumed",
		Data: gin.H{"ref_id": refID},
	})

	c.JSON(200, gin.H{"status": "resumed"})
}

func (h *Handler) StopProject(refID string) error {
	ref, err := h.DB.GetProject(refID)
	if err != nil {
		return fmt.Errorf("project not found: %s", refID)
	}

	coordSID := h.getCoordinatorSessionID(ref)
	if coordSID != "" {
		h.cancelSession(coordSID)
		h.releaseHC(coordSID)
		h.DB.UpdateSessionStatus(coordSID, "idle")
	}

	agents, _ := h.DB.GetProjectAgents(refID)
	for _, a := range agents {
		if a.Role != "coordinator" {
			h.cancelSession(a.SessionID)
			h.releaseHC(a.SessionID)
			h.DB.UpdateSessionStatus(a.SessionID, "idle")
			h.DB.UpdateProjectAgentStatus(a.Name, a.SessionID, "idle")
		}
	}

	h.DB.UpdateProjectStatus(refID, "idle")
	h.Notify(SessionEvent{Type: "project_stopped", Data: gin.H{"ref_id": refID}})
	return nil
}

func (h *Handler) handleStop(c *gin.Context) {
	sessionID := c.Param("id")

	h.cancelSession(sessionID)

	refs, _ := h.DB.ListProjects()
	for _, ref := range refs {
		if h.getCoordinatorSessionID(&ref) == sessionID {
			h.StopProject(ref.ID)
			h.Notify(SessionEvent{Type: "session_stopped", Data: gin.H{"session_id": sessionID}})
			c.JSON(200, gin.H{"status": "stopped"})
			return
		}
	}

	// Non-coordinator session (e.g. standalone worker): stop directly
	h.releaseHC(sessionID)
	h.DB.UpdateSessionStatus(sessionID, "idle")

	h.Notify(SessionEvent{Type: "session_stopped", Data: gin.H{"session_id": sessionID}})
	c.JSON(200, gin.H{"status": "stopped"})
}

func (h *Handler) handleGetSettings(c *gin.Context) {
	cfg := h.Config
	c.JSON(200, gin.H{
		"llm":           cfg.LLM,
		"hc":            cfg.HC,
		"thresholds":    cfg.Thresholds,
		"browser":       cfg.Browser,
		"speak_enabled": cfg.SpeakEnabled,
		"port_maps":     cfg.PortMaps,
		"work_dir":      cfg.WorkDir,
	})
}

func (h *Handler) handleUpdateSettings(c *gin.Context) {
	var newCfg config.Config
	if err := c.ShouldBindJSON(&newCfg); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	// Preserve path and work dir
	newCfg.SetPath(h.Config.Path())
	newCfg.WorkDir = h.Config.WorkDir

	*h.Config = newCfg
	h.clientsMu.Lock()
	h.clients = make(map[string]*llm.Client)
	h.clientsMu.Unlock()
	if err := h.Config.Save(); err != nil {
		c.JSON(500, gin.H{"error": "failed to save config: " + err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleGetMessages(c *gin.Context) {
	turns := 10
	if t, err := strconv.Atoi(c.DefaultQuery("turns", "10")); err == nil && t > 0 {
		turns = t
	}
	sessionID := c.DefaultQuery("session_id", "main")

	var beforeID int64 = 0
	if beforeIDStr := c.Query("before_id"); beforeIDStr != "" {
		var parseErr error
		beforeID, parseErr = strconv.ParseInt(beforeIDStr, 10, 64)
		if parseErr != nil {
			c.JSON(400, gin.H{"error": "invalid before_id"})
			return
		}
	}

	var msgs []db.Message
	var err error

	// Project view: resolve ref_id to all project agents, merge sessions with bubble pagination
	if sessionID != "main" && sessionID != "temp" {
		if ref, refErr := h.DB.GetProject(sessionID); refErr == nil {
			agents, _ := h.DB.GetProjectAgents(sessionID)
			var coordSids, workerSids []string
			for _, a := range agents {
				if a.SessionID == "" {
					continue
				}
				if a.Role == "coordinator" {
					coordSids = append(coordSids, a.SessionID)
				} else {
					workerSids = append(workerSids, a.SessionID)
				}
			}
			if len(coordSids) == 0 {
				// fallback to coordinator session resolution
				if sid := h.getCoordinatorSessionID(ref); sid != "" {
					coordSids = []string{sid}
				}
			}
			if len(coordSids) > 0 || len(workerSids) > 0 {
				msgs, err = h.DB.GetMessagesProjectBubble(coordSids, workerSids, beforeID, turns)
			}
		}
	}

	// Home view or fallback: single session bubble pagination
	if msgs == nil && err == nil {
		msgs, err = h.DB.GetMessagesSessionBubble(sessionID, beforeID, turns)
	}

	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// Filter empty assistant messages
	filtered := make([]db.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "assistant" && strings.TrimSpace(m.Content) == "" && (m.ToolCalls == "[]" || m.ToolCalls == "") {
			continue
		}
		filtered = append(filtered, m)
	}
	c.JSON(200, filtered)
}

func (h *Handler) handleBookmarkMessage(c *gin.Context) {
	idStr := c.Param("id")
	msgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid message id"})
		return
	}
	var body struct{ Bookmarked bool `json:"bookmarked"` }
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(400, gin.H{"error": "invalid body"})
		return
	}
	if err := h.DB.SetBookmark(msgID, body.Bookmarked); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleGetBookmarkedMessages(c *gin.Context) {
	projectID := c.DefaultQuery("project_id", "")
	if projectID == "" {
		c.JSON(400, gin.H{"error": "project_id required"})
		return
	}
	ref, err := h.DB.GetProject(projectID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	sessionIDs := make([]string, 0, len(ref.Agents))
	for _, a := range ref.Agents {
		if a.SessionID != "" {
			sessionIDs = append(sessionIDs, a.SessionID)
		}
	}
	if len(sessionIDs) == 0 {
		c.JSON(200, []db.Message{})
		return
	}
	msgs, err := h.DB.GetBookmarkedMessages(sessionIDs)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, msgs)
}

func (h *Handler) handleClearContext(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		sessionID = "main"
	}

	lastID, err := h.DB.GetLastMessageID(sessionID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.UpdateSessionContextStart(sessionID, lastID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	h.DB.InsertMessage(&db.Message{SessionID: sessionID, Role: "notice", Content: "--- 上下文已清空 ---"})
	h.Notify(SessionEvent{Type: "notice", SessionID: sessionID, Data: gin.H{"content": "--- 上下文已清空 ---"}})

	c.JSON(200, gin.H{"status": "ok", "context_start_id": lastID})
}

func (h *Handler) handleSwitchModel(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		ModelID string `json:"model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Validate model exists (empty = follow default)
	if req.ModelID != "" {
		if m := h.Config.ModelByID(req.ModelID); m == nil {
			c.JSON(400, gin.H{"error": "unknown model: " + req.ModelID})
			return
		}
	}

	if err := h.DB.UpdateSessionModel(sessionID, req.ModelID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	model := h.Config.ResolveModel(req.ModelID)
	if model == nil {
		c.JSON(400, gin.H{"error": "no models configured"})
		return
	}
	c.JSON(200, gin.H{"model_id": req.ModelID, "model_name": model.ID, "vision": model.Vision, "context_limit": model.ContextLimit})
}

func (h *Handler) handleGetSessionModel(c *gin.Context) {
	sessionID := c.Param("id")
	model := h.resolveModel(sessionID)
	sess, err := h.DB.GetSession(sessionID)
	modelID := ""
	if err == nil {
		modelID = sess.ModelID
	}
	if model == nil {
		c.JSON(200, gin.H{"model_id": modelID, "model_name": "", "vision": false, "context_limit": 0})
		return
	}
	c.JSON(200, gin.H{
		"model_id":      modelID,
		"model_name":    model.ID,
		"vision":        model.Vision,
		"context_limit": model.ContextLimit,
	})
}

func (h *Handler) resolveModel(sessionID string) *config.ModelProfile {
	sess, err := h.DB.GetSession(sessionID)
	if err != nil {
		return h.Config.ActiveModel()
	}
	return h.Config.ResolveModel(sess.ModelID)
}

// 闁冲厜鍋撻柍鍏夊亾 Knowledge API handlers 闁冲厜鍋撻柍鍏夊亾

func (h *Handler) handleListKnowledge(c *gin.Context) {
	offset := 0
	limit := 50
	if v := c.Query("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	if v := c.Query("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	knowledge, err := h.DB.ListKnowledge(limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	count, _ := h.DB.KnowledgeCount()
	c.JSON(200, gin.H{"knowledge": knowledge, "count": count})
}

func (h *Handler) handleSearchKnowledge(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(400, gin.H{"error": "query required"})
		return
	}
	offset := 0
	limit := 20
	if v := c.Query("offset"); v != "" {
		fmt.Sscanf(v, "%d", &offset)
	}
	if v := c.Query("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	knowledge, count, err := h.DB.SearchKnowledgeByQuery(q, limit, offset)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"knowledge": knowledge, "count": count})
}

func (h *Handler) handleUpdateKnowledge(c *gin.Context) {
	var id int64
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Title   string `json:"title"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Title == "" && req.Content == "" {
		c.JSON(400, gin.H{"error": "title or content required"})
		return
	}
	if err := h.DB.UpdateKnowledge(id, req.Title, req.Content); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "updated"})
}

func (h *Handler) handleDeleteKnowledge(c *gin.Context) {
	var id int64
	if _, err := fmt.Sscanf(c.Param("id"), "%d", &id); err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := h.DB.DeleteKnowledge(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}



func (h *Handler) getClient(modelID string) *llm.Client {
	model := h.Config.ResolveModel(modelID)
	if model == nil {
		return h.LLM
	}

	h.clientsMu.RLock()
	c, ok := h.clients[model.ID]
	h.clientsMu.RUnlock()
	if ok {
		return c
	}

	h.clientsMu.Lock()
	defer h.clientsMu.Unlock()
	if c, ok := h.clients[model.ID]; ok {
		return c
	}
	c = llm.New(model.BaseURL, model.APIKey, model.Model)
	h.clients[model.ID] = c
	return c
}

func (h *Handler) acquireHC(sessionID string) {
	h.hcSlots <- struct{}{}
	h.hcSessions.Store(sessionID, true)
}

func (h *Handler) releaseHC(sessionID string) {
	if _, ok := h.hcSessions.LoadAndDelete(sessionID); !ok {
		return
	}
	<-h.hcSlots
}

func extractTitle(stateMD string) string {
	lines := strings.Split(stateMD, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

func envInfo(workDir string) string {
	shell := "bash"
	if runtime.GOOS == "windows" {
		shell = "PowerShell"
	}
	hostname, _ := os.Hostname()
	return fmt.Sprintf("\n\n## Environment\n- OS: %s\n- Shell: %s\n- Hostname: %s\n- Work directory: %s",
		runtime.GOOS, shell, hostname, workDir)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "..."
}


func (h *Handler) listProjectsForLLM() (string, error) {
	refs, err := h.DB.ListProjects()
	if err != nil {
		return "", err
	}
	var lines []string
	for _, r := range refs {
		sid := h.getCoordinatorSessionID(&r)
		lines = append(lines, fmt.Sprintf("ref_id=%s session_id=%s title=%s status=%s", r.ID, sid, r.Title, r.Status))
	}
	if len(lines) == 0 {
		return "No projects found.", nil
	}
	return fmt.Sprintf("%d project(s):\n%s", len(lines), strings.Join(lines, "\n")), nil
}

func (h *Handler) getProjectStatusForLLM(refID string) (string, error) {
	content, err := os.ReadFile(h.Config.StatusPath(refID))
	if err != nil {
		return "", fmt.Errorf("project not found: %s", refID)
	}
	return string(content), nil
}

func (h *Handler) searchConversationForLLM(query string) (string, error) {
	msgs, err := h.DB.SearchMessages(query, 20)
	if err != nil {
		return "", err
	}
	if len(msgs) == 0 {
		return "No messages found.", nil
	}
	var lines []string
	for _, m := range msgs {
		lines = append(lines, fmt.Sprintf("[%s] %s: %s", m.SessionID, m.Role, truncate(m.Content, 200)))
	}
	return strings.Join(lines, "\n"), nil
}

func buildTextAndImageMultiContent(text string, images []string) []openai.ChatMessagePart {
	var parts []openai.ChatMessagePart
	if text != "" {
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeText,
			Text: text,
		})
	}
	for _, img := range images {
		parts = append(parts, openai.ChatMessagePart{
			Type: openai.ChatMessagePartTypeImageURL,
			ImageURL: &openai.ChatMessageImageURL{
				URL: img,
			},
		})
	}
	return parts
}

// resolveToolNames looks up the agent template and returns its tool names.
func (h *Handler) resolveToolNames(agentName string) []string {
	agent, err := h.DB.GetAgentByName(agentName)
	if err != nil {
		return nil
	}
	var toolNames []string
	json.Unmarshal([]byte(agent.Tools), &toolNames)
	return toolNames
}

