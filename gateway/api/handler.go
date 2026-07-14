package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"beleader/gateway/config"
	"beleader/gateway/db"
	"beleader/gateway/llm"
	"beleader/gateway/mcp"

	"github.com/gin-gonic/gin"
)

// Handler is the Gateway HTTP handler.
type Handler struct {
	DB      *db.DB
	LLM     *llm.Client
	Config  *config.Config
	SSE     *SSEBroker
	MCPMgr  *mcp.Manager
	Runtime *RuntimeClient

	pauseChs     map[string]chan struct{}
	interveneChs map[string]chan struct{}
	cancelFuncs  map[string]context.CancelFunc
	mu           sync.Mutex

	observers []SessionObserver
}

func NewHandler(database *db.DB, llmClient *llm.Client, cfg *config.Config) *Handler {
	broker := NewSSEBroker()
	h := &Handler{
		DB:           database,
		LLM:          llmClient,
		Config:       cfg,
		SSE:          broker,
		Runtime:      NewRuntimeClient(cfg.RuntimeURL),
		pauseChs:     make(map[string]chan struct{}),
		interveneChs: make(map[string]chan struct{}),
		cancelFuncs:  make(map[string]context.CancelFunc),
	}
	h.RegisterObserver(broker)
	return h
}

func (h *Handler) RegisterObserver(o SessionObserver) {
	h.observers = append(h.observers, o)
}

func (h *Handler) Notify(event SessionEvent) {
	for _, o := range h.observers {
		o.OnSessionEvent(event)
	}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		api.POST("/chat", h.handleChat)
		api.GET("/sse", h.handleSSE)

		api.GET("/threads", h.handleListThreads)
		api.GET("/threads/:id", h.handleGetThread)
		api.DELETE("/threads/:id", h.handleDeleteThread)
		api.GET("/threads/:id/messages", h.handleGetMessages)

		api.POST("/threads/:id/pause", h.handlePause)
		api.POST("/threads/:id/resume", h.handleResume)
		api.POST("/threads/:id/intervene", h.handleIntervene)

		api.GET("/agents", h.handleListAgents)
		api.POST("/agents", h.handleCreateAgent)
		api.PUT("/agents/:id", h.handleUpdateAgent)
		api.DELETE("/agents/:id", h.handleDeleteAgent)

		api.GET("/tools", h.handleListTools)

		api.GET("/knowledge", h.handleListKnowledge)
		api.GET("/knowledge/search", h.handleSearchKnowledge)
		api.PUT("/knowledge/:id", h.handleUpdateKnowledge)
		api.DELETE("/knowledge/:id", h.handleDeleteKnowledge)

		api.GET("/settings", h.handleGetSettings)
		api.PUT("/settings", h.handleUpdateSettings)

		api.GET("/mcp/servers", h.handleListMCPServers)
		api.POST("/mcp/servers", h.handleCreateMCPServer)
		api.PUT("/mcp/servers/:id", h.handleUpdateMCPServer)
		api.DELETE("/mcp/servers/:id", h.handleDeleteMCPServer)
		api.POST("/mcp/servers/:id/test", h.handleTestMCPServer)
		api.POST("/mcp/servers/:id/connect", h.handleConnectMCPServer)
		api.POST("/mcp/servers/:id/disconnect", h.handleDisconnectMCPServer)
	}
}

// ── Chat ──

func (h *Handler) handleChat(c *gin.Context) {
	var req struct {
		ThreadID string   `json:"thread_id"`
		AgentID  int64    `json:"agent_id"`
		Message  string   `json:"message"`
		Images   []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Message == "" {
		c.JSON(400, gin.H{"error": "message required"})
		return
	}

	agent, err := h.DB.GetAgent(req.AgentID)
	if err != nil {
		c.JSON(400, gin.H{"error": "agent not found"})
		return
	}

	threadID := req.ThreadID
	if threadID == "" {
		// New thread: create on Runtime first — Runtime owns the ID.
		rtID, err := h.createRuntimeThread(agent)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		threadID = rtID

		title := req.Message
		if len(title) > 80 {
			title = title[:80]
		}
		model := h.resolveModel()
		modelID := ""
		if model != nil {
			modelID = model.ModelID
		}
		if err := h.DB.CreateThread(threadID, title, agent.ID, modelID, h.Config.RuntimeURL); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	} else {
		h.cancelThread(threadID)
	}

	c.JSON(200, gin.H{"thread_id": threadID})
	go h.runSession(threadID, agent, req.Message, req.Images)
}

func (h *Handler) cancelThread(threadID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancel, ok := h.cancelFuncs[threadID]; ok {
		cancel()
		delete(h.cancelFuncs, threadID)
	}
}

// ── Thread CRUD ──

func (h *Handler) handleListThreads(c *gin.Context) {
	threads, err := h.DB.ListThreads()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, threads)
}

func (h *Handler) handleGetThread(c *gin.Context) {
	t, err := h.DB.GetThread(c.Param("id"))
	if err != nil {
		c.JSON(404, gin.H{"error": "thread not found"})
		return
	}
	c.JSON(200, t)
}

func (h *Handler) handleDeleteThread(c *gin.Context) {
	id := c.Param("id")
	h.cancelThread(id)
	h.Runtime.DeleteThread(id)
	if err := h.DB.DeleteThread(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}

func (h *Handler) handleGetMessages(c *gin.Context) {
	threadID := c.Param("id")
	afterID := int64(0)
	if v := c.Query("after_id"); v != "" {
		fmt.Sscanf(v, "%d", &afterID)
	}
	msgs, err := h.DB.GetMessages(threadID, afterID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, msgs)
}

// ── SSE ──

func (h *Handler) handleSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	ch := h.SSE.Subscribe()
	defer h.SSE.Unsubscribe(ch)

	for {
		select {
		case msg := <-ch:
			fmt.Fprint(c.Writer, msg)
			c.Writer.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

// ── Pause / Resume / Intervene ──

func (h *Handler) handlePause(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.pauseChs[c.Param("id")]; ok {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	c.JSON(200, gin.H{"status": "paused"})
}

func (h *Handler) handleResume(c *gin.Context) {
	threadID := c.Param("id")
	t, err := h.DB.GetThread(threadID)
	if err != nil {
		c.JSON(404, gin.H{"error": "thread not found"})
		return
	}
	agent, err := h.DB.GetAgent(t.AgentID)
	if err != nil {
		c.JSON(404, gin.H{"error": "agent not found"})
		return
	}
	go h.runSession(threadID, agent, "[System] Please continue.", nil)
	c.JSON(200, gin.H{"status": "resumed"})
}

func (h *Handler) handleIntervene(c *gin.Context) {
	var req struct {
		Message string   `json:"message"`
		Images  []string `json:"images"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	h.cancelThread(c.Param("id"))

	threadID := c.Param("id")
	t, err := h.DB.GetThread(threadID)
	if err != nil {
		c.JSON(404, gin.H{"error": "thread not found"})
		return
	}
	agent, _ := h.DB.GetAgent(t.AgentID)
	if agent == nil {
		c.JSON(404, gin.H{"error": "agent not found"})
		return
	}
	go h.runSession(threadID, agent, req.Message, req.Images)
	c.JSON(200, gin.H{"status": "intervened"})
}

// ── Settings ──

func (h *Handler) handleGetSettings(c *gin.Context) {
	mcpServers, _ := h.DB.ListMCPServers()
	if mcpServers == nil {
		mcpServers = []db.MCPServer{}
	}
	if h.MCPMgr != nil {
		statuses := h.MCPMgr.Statuses()
		for i := range mcpServers {
			if s, ok := statuses[mcpServers[i].Name]; ok {
				mcpServers[i].Status = s.Status
				mcpServers[i].Error = s.Error
			}
		}
	}
	agents, _ := h.DB.ListAgents()
	if agents == nil {
		agents = []db.Agent{}
	}

	dbModels, _ := h.DB.ListModels()
	models := make([]config.ModelProfile, len(dbModels))
	active := ""
	for i, m := range dbModels {
		models[i] = config.ModelProfile{
			ID:           m.ModelID,
			BaseURL:      m.BaseURL,
			APIKey:       m.APIKey,
			Model:        m.Model,
			Vision:       m.Vision,
			ContextLimit: m.ContextLimit,
		}
		if m.IsActive {
			active = m.ModelID
		}
	}
	if models == nil {
		models = []config.ModelProfile{}
	}

	c.JSON(200, gin.H{
		"llm":         gin.H{"models": models, "active": active},
		"mcp_servers": mcpServers,
		"agents":      agents,
	})
}

func (h *Handler) handleUpdateSettings(c *gin.Context) {
	var req struct {
		LLM *struct {
			Models []config.ModelProfile `json:"models"`
			Active string                `json:"active"`
		} `json:"llm"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.LLM != nil {
		dbModels := make([]db.ModelProfile, len(req.LLM.Models))
		for i, m := range req.LLM.Models {
			dbModels[i] = db.ModelProfile{
				ModelID:      m.ID,
				BaseURL:      m.BaseURL,
				APIKey:       m.APIKey,
				Model:        m.Model,
				Vision:       m.Vision,
				ContextLimit: m.ContextLimit,
			}
		}
		if err := h.DB.SetModels(dbModels); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if req.LLM.Active != "" {
			h.DB.SetActiveModel(req.LLM.Active)
		}
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// ── Helpers ──

func (h *Handler) handleListTools(c *gin.Context) {
	tools := baseToolDefs()
	// Add MCP tools
	if h.MCPMgr != nil {
		for _, t := range h.MCPMgr.ListTools() {
			tools = append(tools, map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"source":      "mcp",
				"parameters":  t.InputSchema,
			})
		}
	}
	// Tag builtin tools
	for i := range tools {
		if tools[i]["source"] == nil {
			tools[i]["source"] = "builtin"
		}
	}
	c.JSON(200, tools)
}

func (h *Handler) resolveModel() *db.ModelProfile {
	m, err := h.DB.ActiveModel()
	if err != nil {
		models, _ := h.DB.ListModels()
		if len(models) > 0 {
			return &models[0]
		}
		return nil
	}
	return m
}

func (h *Handler) buildModelMap() map[string]any {
	m := h.resolveModel()
	if m == nil {
		return map[string]any{"context_limit": 128000}
	}
	return map[string]any{
		"base_url":      m.BaseURL,
		"api_key":       m.APIKey,
		"model":         m.Model,
		"context_limit": m.ContextLimit,
		"vision":        m.Vision,
	}
}

func (h *Handler) createRuntimeThread(agent *db.Agent) (string, error) {
	var toolNames []string
	json.Unmarshal([]byte(agent.Tools), &toolNames)
	if len(toolNames) == 0 {
		toolNames = defaultToolNames()
	}

	req := CreateThreadRequest{
		SystemPrompt: agent.SystemPrompt,
		Model:        h.buildModelMap(),
		Tools:        baseToolDefsFiltered(toolNames),
		Metadata: map[string]any{
			"agent_id": agent.ID,
		},
	}

	resp, err := h.Runtime.CreateThread(req)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

func defaultToolNames() []string {
	return []string{"read_file", "read_dir", "write_file", "edit_file", "delete_file", "search_content", "search_files", "read_status", "update_status", "run_command", "web_search", "web_fetch", "run_http_request", "spawn_worker"}
}
