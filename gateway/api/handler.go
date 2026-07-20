package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"beleader/gateway/config"
	"beleader/gateway/db"
	"beleader/gateway/llm"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// turnHandle tracks an active turn for a thread.
type turnHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
	token  int64
}

// Handler is the Gateway HTTP handler.
type Handler struct {
	DB       *db.DB
	LLM      *llm.Client
	Config   *config.Config
	SSE      *SSEBroker
	Runtimes *RuntimeClientPool

	RegToken   string // shared secret for runtime registration, from GATEWAY_TOKEN env var

	turnHandles map[string]*turnHandle
	mu          sync.Mutex

	observers []SessionObserver
}

func NewHandler(database *db.DB, llmClient *llm.Client, cfg *config.Config) *Handler {
	broker := NewSSEBroker()
	h := &Handler{
		DB:           database,
		LLM:          llmClient,
		Config:       cfg,
		SSE:          broker,
		Runtimes:     NewRuntimeClientPool(),
		RegToken:     cfg.RegToken,
		turnHandles: make(map[string]*turnHandle),
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
		api.GET("/threads/:id/workers", h.handleListWorkers)
		api.POST("/threads/:id/workers/:workerID/stop", h.handleStopWorker)

		api.GET("/agents", h.handleListAgents)
		api.POST("/agents", h.handleCreateAgent)
		api.PUT("/agents/:id", h.handleUpdateAgent)
		api.DELETE("/agents/:id", h.handleDeleteAgent)

		api.GET("/tools", h.handleListTools)

		api.GET("/knowledge", h.handleListKnowledge)
		api.GET("/knowledge/search", h.handleSearchKnowledge)
		api.PUT("/knowledge/:id", h.handleUpdateKnowledge)
		api.DELETE("/knowledge/:id", h.handleDeleteKnowledge)

		api.GET("/models", h.handleListModels)
		api.POST("/models", h.handleCreateModel)
		api.PUT("/models", h.handleUpdateModel)
		api.DELETE("/models", h.handleDeleteModel)

		api.GET("/mcp/servers", h.handleListMCPServers)
		api.POST("/mcp/servers", h.handleCreateMCPServer)
		api.PUT("/mcp/servers/:id", h.handleUpdateMCPServer)
		api.DELETE("/mcp/servers/:id", h.handleDeleteMCPServer)
		api.POST("/mcp/servers/:id/test", h.handleTestMCPServer)

		api.POST("/runtimes/register", h.handleRuntimeRegister)
		api.POST("/runtimes/heartbeat", h.handleRuntimeHeartbeat)
		api.GET("/runtimes", h.handleListRuntimes)
		api.DELETE("/runtimes/:id", h.handleDeleteRuntime)
	}
}

// ── Chat ──

func (h *Handler) handleChat(c *gin.Context) {
	var req struct {
		ThreadID        string   `json:"thread_id"`
		AgentID         int64    `json:"agent_id"`
		ModelID         string   `json:"model_id"`
		ReasoningEffort string   `json:"reasoning_effort"`
		Message         string   `json:"message"`
		Images          []string `json:"images"`
		ParentThreadID  string   `json:"parent_thread_id"`
		WorkspaceDir    string   `json:"workspace_dir"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Message == "" {
		c.JSON(400, gin.H{"error": "message required"})
		return
	}
	if !h.Runtimes.HasAny() {
		c.JSON(503, gin.H{"error": "no runtime available — wait for a runtime to register"})
		return
	}

	agent, err := h.DB.GetAgent(req.AgentID)
	if err != nil {
		c.JSON(400, gin.H{"error": "agent not found"})
		return
	}

	// Resolve model: request override > agent default > first available.
	model := h.resolveModel(agent.ID, req.ModelID)
	if model != nil && req.ReasoningEffort != "" {
		model = overrideEffort(model, req.ReasoningEffort)
	}

	runtime, _ := h.Runtimes.Pick()
	if runtime == nil {
		c.JSON(503, gin.H{"error": "no runtime available"})
		return
	}

	threadID := req.ThreadID
	if threadID == "" {
		modelID := ""
		if model != nil {
			modelID = model.ModelID
		}

		// If this is a worker thread, share the parent's workspace.
		if req.ParentThreadID != "" && req.WorkspaceDir == "" {
			req.WorkspaceDir = filepath.Join(h.Config.DataDir, "threads", req.ParentThreadID, "workspace")
		}

		rtID, err := h.createRuntimeThread(runtime, agent, model, req.WorkspaceDir, req.ParentThreadID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		threadID = rtID

		title := req.Message
		if len(title) > 80 {
			title = title[:80]
		}
		if req.ParentThreadID != "" {
			if err := h.DB.CreateWorkerThread(threadID, title, req.ParentThreadID, agent.ID, modelID); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
			h.Notify(SessionEvent{Type: "worker.dispatched", SessionID: req.ParentThreadID, Data: map[string]any{
				"thread_id":  threadID,
				"agent_name": agent.Name,
				"task":       req.Message,
			}})
		} else {
			if err := h.DB.CreateThread(threadID, title, agent.ID, modelID); err != nil {
				c.JSON(500, gin.H{"error": err.Error()})
				return
			}
		}
	} else {
		h.cancelThread(threadID)
	}

	// Run the turn in background. All events flow through Gateway SSE.
	go h.runSession(runtime, threadID, agent, model, req.Message, req.Images)

	c.JSON(http.StatusOK, gin.H{"thread_id": threadID, "status": "started"})
}

// onTurnComplete runs after a turn finishes. Handles both trigger points:
// Trigger 1: if this is a parent thread, check for completed worker results and auto-resume.
// Trigger 2: if this is a worker thread, mark completed and check if parent needs wakeup.
func (h *Handler) onTurnComplete(threadID string) {
	t, err := h.DB.GetThread(threadID)
	if err != nil {
		return
	}

	if t.ParentThreadID != "" {
		// Trigger 2: worker turn just finished.
		// Only mark completed if the worker wasn't explicitly stopped.
		finalStatus := t.Status
		if t.Status == "running" {
			h.DB.SetThreadStatus(threadID, "completed")
			h.tryAutoResumeParent(t.ParentThreadID)
			finalStatus = "completed"
		}

		h.Notify(SessionEvent{Type: "worker.completed", SessionID: t.ParentThreadID, Data: map[string]any{
			"thread_id": threadID,
			"status":    finalStatus,
		}})
	} else {
		// Trigger 1: parent turn ended. Skip if cancelled (turnHandle already removed).
		h.mu.Lock()
		_, exists := h.turnHandles[threadID]
		h.mu.Unlock()
		if exists {
			h.tryCheckCompletedWorkers(threadID)
		}
	}
}

// tryAutoResumeParent checks if the parent thread is idle and has pending worker results.
// If so, it auto-resumes the parent with batched worker results.
func (h *Handler) tryAutoResumeParent(parentThreadID string) {
	h.mu.Lock()
	_, active := h.turnHandles[parentThreadID]
	h.mu.Unlock()
	if active {
		return // parent still running, Trigger 1 will handle it
	}
	h.tryCheckCompletedWorkers(parentThreadID)
}

// tryCheckCompletedWorkers checks for completed undelivered workers and auto-resumes the parent.
func (h *Handler) tryCheckCompletedWorkers(threadID string) {
	workers, err := h.DB.GetCompletedWorkers(threadID)
	if err != nil || len(workers) == 0 {
		return
	}

	// Build batched result message.
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[%d workers completed]\n", len(workers)))
	for _, w := range workers {
		msgs, _ := h.DB.GetRecentMessages(w.ID, 5)
		var result string
		for _, m := range msgs {
			if m.Kind == "agent_message" {
				result = m.Content
			}
		}
		b.WriteString(fmt.Sprintf("\n## %s (thread %s)\n", w.Title, w.ID))
		if result != "" {
			b.WriteString(result)
		}
		b.WriteString("\n")
	}

	// Mark as delivered.
	var ids []string
	for _, w := range workers {
		ids = append(ids, w.ID)
	}
	h.DB.MarkWorkersDelivered(ids)

	// Load parent thread info for resume.
	t, err := h.DB.GetThread(threadID)
	if err != nil {
		return
	}
	agent, err := h.DB.GetAgent(t.AgentID)
	if err != nil {
		return
	}
	model := h.resolveModel(agent.ID, "")
	runtime, _ := h.Runtimes.Pick()
	if runtime == nil {
		return
	}

	// Auto-resume the parent with worker results as the message.
	go h.runSession(runtime, threadID, agent, model, b.String(), nil)
}

func (h *Handler) cancelThread(threadID string) {
	h.mu.Lock()
	th, ok := h.turnHandles[threadID]
	if ok {
		delete(h.turnHandles, threadID)
	}
	h.mu.Unlock()
	if ok {
		th.cancel()
		<-th.done // wait for old turn to fully exit
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
	if runtime, _ := h.Runtimes.Pick(); runtime != nil {
		runtime.DeleteThread(id)
	}
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
	resp := gin.H{"messages": msgs, "latest_seq": int64(0)}
	if runtime, _ := h.Runtimes.Pick(); runtime != nil {
		if seq, err2 := runtime.GetLatestSeq(threadID); err2 == nil {
			resp["latest_seq"] = seq
		}
	}
	c.JSON(200, resp)
}

// ── SSE ──

func (h *Handler) handleSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	threadID := c.Query("thread_id")
	if threadID == "" {
		c.JSON(400, gin.H{"error": "thread_id query param required"})
		return
	}
	ch := h.SSE.Subscribe(threadID)
	defer h.SSE.Unsubscribe(threadID, ch)

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
	threadID := c.Param("id")
	// Fetch active workers before marking them stopped.
	workers, _ := h.DB.GetActiveWorkers(threadID)
	// Mark workers stopped before cancelling their turns, so onTurnComplete skips auto-resume.
	h.DB.StopWorkers(threadID)
	h.mu.Lock()
	for _, w := range workers {
		if th, ok := h.turnHandles[w.ID]; ok {
			th.cancel()
			delete(h.turnHandles, w.ID)
		}
	}
	h.mu.Unlock()
	h.cancelThread(threadID)
	c.JSON(200, gin.H{"status": "paused"})
}

// handleListWorkers returns status of all worker threads for a parent.
func (h *Handler) handleListWorkers(c *gin.Context) {
	parentID := c.Param("id")
	running, _ := h.DB.GetActiveWorkers(parentID)
	completed, _ := h.DB.GetCompletedWorkers(parentID)

	type workerStatus struct {
		ID     string `json:"id"`
		Title  string `json:"title"`
		Status string `json:"status"`
	}
	var workers []workerStatus
	for _, w := range running {
		workers = append(workers, workerStatus{ID: w.ID, Title: w.Title, Status: "running"})
	}
	for _, w := range completed {
		workers = append(workers, workerStatus{ID: w.ID, Title: w.Title, Status: "completed"})
	}
	if workers == nil {
		workers = []workerStatus{}
	}
	c.JSON(200, gin.H{"workers": workers})
}

// handleStopWorker stops a specific worker thread.
func (h *Handler) handleStopWorker(c *gin.Context) {
	workerID := c.Param("workerID")
	h.DB.SetThreadStatus(workerID, "stopped") // set before cancel so onTurnComplete sees it
	h.cancelThread(workerID)
	c.JSON(200, gin.H{"status": "stopped"})
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
	model := h.resolveModel(agent.ID, "")
	runtime, _ := h.Runtimes.Pick()
	if runtime == nil {
		c.JSON(503, gin.H{"error": "no runtime available"})
		return
	}
	go h.runSession(runtime, threadID, agent, model, "[System] Please continue.", nil)
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
	model := h.resolveModel(agent.ID, "")
	runtime, _ := h.Runtimes.Pick()
	if runtime == nil {
		c.JSON(503, gin.H{"error": "no runtime available"})
		return
	}
	go h.runSession(runtime, threadID, agent, model, req.Message, req.Images)
	c.JSON(200, gin.H{"status": "intervened"})
}

// ── Models ──

func (h *Handler) handleListModels(c *gin.Context) {
	dbModels, err := h.DB.ListModels()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if dbModels == nil {
		dbModels = []db.ModelProfile{}
	}
	models := make([]config.ModelProfile, len(dbModels))
	for i, m := range dbModels {
		models[i] = config.ModelProfile{
			ID:              m.ModelID,
			BaseURL:         m.BaseURL,
			APIKey:          m.APIKey,
			Model:           m.Model,
			Vision:          m.Vision,
			ContextLimit:    m.ContextLimit,
			ReasoningEffort: m.ReasoningEffort,
		}
	}
	c.JSON(200, models)
}


func (h *Handler) handleCreateModel(c *gin.Context) {
	var req config.ModelProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.ID == "" {
		c.JSON(400, gin.H{"error": "id is required"})
		return
	}
	m := &db.ModelProfile{
		ModelID:         strings.TrimSpace(req.ID),
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		Model:           req.Model,
		Vision:          req.Vision,
		ContextLimit:    req.ContextLimit,
		ReasoningEffort: req.ReasoningEffort,
	}
	if err := h.DB.CreateModel(m); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, modelToConfig(m))
}

func (h *Handler) handleUpdateModel(c *gin.Context) {
	var req config.ModelProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	modelID := strings.TrimSpace(req.ID)
	if modelID == "" {
		c.JSON(400, gin.H{"error": "id is required"})
		return
	}
	m := &db.ModelProfile{
		ModelID:         modelID,
		BaseURL:         req.BaseURL,
		APIKey:          req.APIKey,
		Model:           req.Model,
		Vision:          req.Vision,
		ContextLimit:    req.ContextLimit,
		ReasoningEffort: req.ReasoningEffort,
	}
	if err := h.DB.UpdateModel(modelID, m); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleDeleteModel(c *gin.Context) {
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	modelID := strings.TrimSpace(req.ID)
	if modelID == "" {
		c.JSON(400, gin.H{"error": "id is required"})
		return
	}
	if err := h.DB.DeleteModel(modelID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}

func modelToConfig(m *db.ModelProfile) config.ModelProfile {
	return config.ModelProfile{
		ID:              m.ModelID,
		BaseURL:         m.BaseURL,
		APIKey:          m.APIKey,
		Model:           m.Model,
		Vision:          m.Vision,
		ContextLimit:    m.ContextLimit,
		ReasoningEffort: m.ReasoningEffort,
	}
}

// ── Helpers ──

func (h *Handler) handleListTools(c *gin.Context) {
	tools, err := h.Runtimes.ToolDefs()
	if err != nil {
		c.JSON(503, gin.H{"error": "no runtime available: " + err.Error()})
		return
	}
	for i := range tools {
		if tools[i]["source"] == nil {
			tools[i]["source"] = "builtin"
		}
	}
	c.JSON(200, tools)
}

func (h *Handler) resolveModel(agentID int64, overrideModelID string) *db.ModelProfile {
	// 1. Use explicit override if provided (per-conversation model switch).
	if overrideModelID != "" {
		if m, err := h.DB.GetModelByID(overrideModelID); err == nil {
			return m
		}
	}
	// 2. Use agent's default model if set.
	if agentID != 0 {
		agent, err := h.DB.GetAgent(agentID)
		if err == nil && agent.DefaultModelID != "" {
			if m, err := h.DB.GetModelByID(agent.DefaultModelID); err == nil {
				return m
			}
		}
	}
	// 3. Fall back to first available model.
	models, _ := h.DB.ListModels()
	if len(models) > 0 {
		return &models[0]
	}
	return nil
}

func overrideEffort(m *db.ModelProfile, effort string) *db.ModelProfile {
	copy := *m
	copy.ReasoningEffort = effort
	return &copy
}

func (h *Handler) buildModelMap(m *db.ModelProfile) map[string]any {
	if m == nil {
		return map[string]any{"context_limit": 128000}
	}
	return map[string]any{
		"base_url":         m.BaseURL,
		"api_key":          m.APIKey,
		"model":            m.Model,
		"context_limit":    m.ContextLimit,
		"vision":           m.Vision,
		"reasoning_effort": m.ReasoningEffort,
	}
}

func (h *Handler) createRuntimeThread(runtime *RuntimeClient, agent *db.Agent, model *db.ModelProfile, workspaceOverride string, parentThreadID string) (string, error) {
	allTools, err := runtime.ToolDefs()
	if err != nil {
		return "", fmt.Errorf("fetch tools: %w", err)
	}

	var toolNames []string
	json.Unmarshal([]byte(agent.Tools), &toolNames)
	if len(toolNames) == 0 {
		for _, t := range allTools {
			toolNames = append(toolNames, t["name"].(string))
		}
	}

	// Auto-strip spawn/worker tools from worker agents to prevent nesting.
	if parentThreadID != "" {
		strip := map[string]bool{"spawn_worker": true, "check_workers": true, "stop_worker": true}
		filtered := make([]string, 0, len(toolNames))
		for _, n := range toolNames {
			if !strip[n] {
				filtered = append(filtered, n)
			}
		}
		toolNames = filtered
	}

	// Build MCP server configs from agent's mcp_servers list.
	var mcpConfigs []MCPConfig
	if agent.MCPServers != "" {
		var serverNames []string
		if err := json.Unmarshal([]byte(agent.MCPServers), &serverNames); err == nil {
			for _, name := range serverNames {
				allServers, _ := h.DB.ListMCPServers()
				for _, s := range allServers {
					if s.Name == name && s.Enabled {
						mcpConfigs = append(mcpConfigs, MCPConfig{
							Name:    s.Name,
							Type:    s.Type,
							Command: s.Command,
							Args:    s.Args,
							Env:     s.Env,
							URL:     s.URL,
							Headers: s.Headers,
						})
						break
					}
				}
			}
		}
	}

	threadID := uuid.New().String()
	threadDir := filepath.Join(h.Config.DataDir, "threads", threadID)
	workspaceDir := workspaceOverride
	if workspaceDir == "" {
		workspaceDir = filepath.Join(threadDir, "workspace")
	}

	// Build system prompt with worker agent info injected.
	systemPrompt := agent.SystemPrompt
	workerNames, workerInfo := h.buildWorkerPrompt(agent)
	if workerInfo != "" {
		systemPrompt += workerInfo
	}

	req := CreateThreadRequest{
		ThreadID:          threadID,
		ThreadDir:         threadDir,
		WorkspaceDir:      workspaceDir,
		RestrictWorkspace: h.Config.RestrictWorkspace,
		SystemPrompt:      systemPrompt,
		Model:             h.buildModelMap(model),
		Tools:             filterToolsByName(allTools, toolNames),
		MCPServers:        mcpConfigs,
		Metadata: map[string]any{
			"agent_id":      agent.ID,
			"worker_agents": workerNames,
		},
	}

	if _, err := runtime.CreateThread(req); err != nil {
		return "", err
	}
	return threadID, nil
}

// buildWorkerPrompt resolves an agent's worker_agents list and builds a
// system prompt section describing available workers. Returns the list of
// worker agent names and the prompt text to append.
func (h *Handler) buildWorkerPrompt(agent *db.Agent) ([]string, string) {
	var names []string
	if agent.WorkerAgents == "" || agent.WorkerAgents == "[]" {
		return names, ""
	}
	if err := json.Unmarshal([]byte(agent.WorkerAgents), &names); err != nil || len(names) == 0 {
		return names, ""
	}

	allAgents, err := h.DB.ListAgents()
	if err != nil {
		return names, ""
	}
	agentMap := make(map[string]db.Agent, len(allAgents))
	for _, a := range allAgents {
		agentMap[a.Name] = a
	}

	var b strings.Builder
	b.WriteString("\n\n## Available Workers\n")
	b.WriteString("You can delegate work to the following sub-agents using spawn_worker(agent=\"name\", task=\"...\"):\n")
	for _, name := range names {
		if wa, ok := agentMap[name]; ok {
			fmt.Fprintf(&b, "- **%s**: %s\n", wa.Name, wa.Desc)
		}
	}
	return names, b.String()
}

func filterToolsByName(allTools []map[string]any, names []string) []map[string]any {
	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}
	var filtered []map[string]any
	for _, t := range allTools {
		if nameSet[t["name"].(string)] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
