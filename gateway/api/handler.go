package api

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"beleader/gateway/config"
	"beleader/gateway/db"
	"beleader/gateway/engine"
	"beleader/gateway/llm"
	"beleader/gateway/tools"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sashabaranov/go-openai"
)

// turnHandle tracks an active turn for a thread.
type turnHandle struct {
	cancel context.CancelFunc
	done   chan struct{}
	token  int64
}

// Handler is the Gateway HTTP handler.
type Handler struct {
	DB     *db.DB
	LLM    *llm.Client
	SSE    *SSEBroker
	Engine *engine.Engine
	Router *tools.Router

	RegToken string

	turnHandles map[string]*turnHandle
	mu          sync.Mutex

	observers []SessionObserver
}

func NewHandler(database *db.DB, llmClient *llm.Client, cfg *config.Config) *Handler {
	broker := NewSSEBroker()
	router := tools.NewRouter(database)
	tools.SetDB(database)
	tools.RegisterLocalTools(router)
	tools.RegisterWorkerTools(router)
	tools.RegisterManagementTools(router)

	h := &Handler{
		DB:       database,
		LLM:      llmClient,
		SSE:      broker,
		Router:   router,
		RegToken: cfg.RegToken,
		turnHandles: make(map[string]*turnHandle),
	}
	h.Engine = engine.NewEngine(database, llmClient, router)

	// Set worker callbacks
	tools.SetWorkerCallbacks(&tools.WorkerCallbacks{
		SpawnWorker:     h.spawnWorker,
		ListWorkers:     h.listWorkerThreads,
		InterveneWorker: h.interveneWorkerThread,
		TerminateWorker: h.terminateWorkerThread,
	})

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
		api.PUT("/threads/:id", h.handleRenameThread)
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

		api.GET("/models", h.handleListModels)
		api.POST("/models", h.handleCreateModel)
		api.PUT("/models", h.handleUpdateModel)
		api.DELETE("/models", h.handleDeleteModel)

		api.GET("/mcp/servers", h.handleListMCPServers)
		api.POST("/mcp/servers", h.handleCreateMCPServer)
		api.PUT("/mcp/servers/:id", h.handleUpdateMCPServer)
		api.DELETE("/mcp/servers/:id", h.handleDeleteMCPServer)
		api.POST("/mcp/servers/:id/test", h.handleTestMCPServer)

		// Pool management
		api.GET("/pools", h.handleListPools)
		api.POST("/pools", h.handleCreatePool)
		api.PUT("/pools/:id", h.handleUpdatePool)
		api.DELETE("/pools/:id", h.handleDeletePool)

		// Tool Agent management (replaces runtime)
		api.POST("/tool-agents/register", h.handleToolAgentRegister)
		api.POST("/tool-agents/heartbeat", h.handleToolAgentHeartbeat)
		api.GET("/tool-agents", h.handleListToolAgents)
		api.DELETE("/tool-agents/:id", h.handleDeleteToolAgent)
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
		PoolID          int64    `json:"pool_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Message == "" {
		c.JSON(400, gin.H{"error": "message required"})
		return
	}

	threadID := req.ThreadID

	var agent *db.Agent
	var model *db.ModelProfile
	var err error

	if threadID == "" {
		// New thread — use req.AgentID and req.ModelID
		agent, err = h.DB.GetAgent(req.AgentID)
		if err != nil {
			c.JSON(400, gin.H{"error": "agent not found"})
			return
		}

		model = h.resolveModel(agent.ID, req.ModelID)
		if model != nil && req.ReasoningEffort != "" {
			m := *model
			m.ReasoningEffort = req.ReasoningEffort
			model = &m
		}

		// Create new thread
		threadID = uuid.New().String()

		// Determine pool
		poolID := req.PoolID
		if poolID == 0 {
			if req.ParentThreadID != "" {
				parent, _ := h.DB.GetThread(req.ParentThreadID)
				if parent != nil {
					poolID = parent.PoolID
				}
			}
			if poolID == 0 {
				pool, _ := h.DB.GetDefaultPool()
				if pool != nil {
					poolID = pool.ID
				}
			}
		}

		// Init workspace on a tool agent
		workspacePath := ""
		if poolID > 0 {
			agents, _ := h.DB.ListActiveToolAgentsByPool(poolID)
			if len(agents) > 0 {
				client := tools.NewAgentClient(agents[0].URL)
				ws, err := client.InitWorkspace(c.Request.Context(), threadID)
				if err == nil {
					workspacePath = ws
				}
			}
		}

		title := req.Message
		if len(title) > 80 {
			title = title[:80]
		}

		modelID := ""
		if model != nil {
			modelID = model.ModelID
		}

		if req.ParentThreadID != "" {
			h.DB.CreateWorkerThread(threadID, title, req.ParentThreadID, agent.ID, modelID, poolID, workspacePath)
		} else {
			h.DB.CreateThread(threadID, title, agent.ID, modelID, poolID, workspacePath)
		}
	} else {
		// Existing thread — read agent and model from thread, ignore req
		h.cancelThread(threadID)
		existingThread, err := h.DB.GetThread(threadID)
		if err != nil {
			c.JSON(404, gin.H{"error": "thread not found"})
			return
		}
		agent, err = h.DB.GetAgent(existingThread.AgentID)
		if err != nil {
			c.JSON(400, gin.H{"error": "thread's agent not found"})
			return
		}
		model = h.resolveModel(existingThread.AgentID, existingThread.ModelID)
		if model != nil && req.ReasoningEffort != "" {
			m := *model
			m.ReasoningEffort = req.ReasoningEffort
			model = &m
		}
	}

	// Run the turn in background
	go h.runSession(threadID, agent, model, req.Message, req.Images)

	c.JSON(http.StatusOK, gin.H{"thread_id": threadID, "status": "started"})
}

func (h *Handler) runSession(threadID string, agent *db.Agent, model *db.ModelProfile, message string, images []string) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	token := time.Now().UnixNano()

	h.mu.Lock()
	h.turnHandles[threadID] = &turnHandle{cancel: cancel, done: done, token: token}
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		if th, ok := h.turnHandles[threadID]; ok && th.token == token {
			delete(h.turnHandles, threadID)
		}
		h.mu.Unlock()
		cancel()
		close(done)
		h.onTurnComplete(threadID)
	}()

	thread, err := h.DB.GetThread(threadID)
	if err != nil {
		h.Notify(SessionEvent{Type: "error", SessionID: threadID, Data: map[string]any{
			"message": "Failed to load thread: " + err.Error(),
		}})
		return
	}

	// Build system prompt
	sysPrompt := engine.BuildSystemPrompt(agent.SystemPrompt)

	// Update thread's updated_at timestamp (for sorting in thread list)
	h.DB.GORM.Model(&db.Thread{}).Where("id = ?", threadID).Update("updated_at", time.Now())

	// Build turn_meta
	turnMeta := h.buildTurnMeta(thread)

	// Build tool list
	toolList := h.buildToolList(thread, agent)

	// Check if we have a model configured
	if model == nil {
		h.Notify(SessionEvent{Type: "error", SessionID: threadID, Data: map[string]any{
			"message": "No model configured. Add a model in the Model tab.",
		}})
		return
	}

	// Get model config
	modelContextLimit := 128000
	visionEnabled := false
	reasoningEffort := "high"
	llmClient := h.LLM
	if model != nil {
		if model.ContextLimit > 0 {
			modelContextLimit = model.ContextLimit
		}
		visionEnabled = model.Vision
		if model.ReasoningEffort != "" {
			reasoningEffort = model.ReasoningEffort
		}
		llmClient = llm.New(model.BaseURL, model.APIKey, model.Model)
	}

	// Emit callback: stores event to DB + pushes SSE
	emit := func(eventType, turnID, itemID string, payload map[string]any) {
		// Store event in DB
		payloadJSON, _ := json.Marshal(payload)
		eventID, _ := h.DB.InsertEvent(&db.Event{
			ThreadID: threadID,
			TurnID:   turnID,
			ItemID:   itemID,
			Event:    eventType,
			Payload:  string(payloadJSON),
		})
		// Push to SSE
		if payload == nil {
			payload = map[string]any{}
		}
		payload["thread_id"] = threadID
		payload["turn_id"] = turnID
		payload["item_id"] = itemID
		payload["event_id"] = eventID
		ev := SessionEvent{Type: eventType, SessionID: threadID, Data: payload}
		h.Notify(ev)
	}

	result, err := h.Engine.RunLoop(ctx, thread, sysPrompt, turnMeta, message, images, toolList, llmClient, modelContextLimit, visionEnabled, reasoningEffort, emit)
	if err != nil {
		h.Notify(SessionEvent{Type: "error", SessionID: threadID, Data: map[string]any{
			"message": "Agent loop error: " + err.Error(),
		}})
	}
	_ = result
}

func (h *Handler) buildTurnMeta(thread *db.Thread) string {
	if thread.PoolID == 0 {
		return ""
	}
	pool, err := h.DB.GetPool(thread.PoolID)
	if err != nil || pool == nil {
		return ""
	}
	return engine.BuildTurnMeta(pool.Shell, pool.Platform, thread.WorkspacePath, pool.GoVersion, pool.RestrictWorkspace)
}

func (h *Handler) buildToolList(thread *db.Thread, agent *db.Agent) []openai.Tool {
	var result []openai.Tool

	// Local tools
	localDefs := tools.LocalToolDefs()

	// Filter by agent's tool whitelist
	var agentToolNames []string
	if agent.Tools != "" && agent.Tools != "[]" {
		json.Unmarshal([]byte(agent.Tools), &agentToolNames)
	}
	toolNameSet := map[string]bool{}
	for _, n := range agentToolNames {
		toolNameSet[n] = true
	}

	for _, td := range localDefs {
		if len(agentToolNames) > 0 && toolNameSet[td.Name] {
			result = append(result, engine.ToolDefsToOpenAI([]engine.ToolDef{td})[0])
		} else if len(agentToolNames) == 0 {
			// Empty whitelist means no tools — don't add anything.
		}
	}

	// Remote tools from Pool.ToolDefs
	if thread.PoolID > 0 {
		pool, _ := h.DB.GetPool(thread.PoolID)
		if pool != nil && pool.ToolDefs != "[]" && pool.ToolDefs != "" {
			var remoteDefs []engine.ToolDef
			if err := json.Unmarshal([]byte(pool.ToolDefs), &remoteDefs); err == nil {
				for _, td := range remoteDefs {
					if len(agentToolNames) > 0 && toolNameSet[td.Name] {
						result = append(result, engine.ToolDefsToOpenAI([]engine.ToolDef{td})[0])
					}
				}
			}
		}
	}

	return result
}

// ── Worker lifecycle (callbacks for tools package) ──

func (h *Handler) spawnWorker(ctx context.Context, parentThread *db.Thread, agentName, task, poolName string) (string, error) {
	workerAgent, err := h.DB.GetAgentByName(agentName)
	if err != nil {
		return "", fmt.Errorf("agent '%s' not found", agentName)
	}

	poolID := parentThread.PoolID
	if poolName != "" {
		pool, err := h.DB.GetPoolByName(poolName)
		if err != nil {
			return "", fmt.Errorf("pool '%s' not found", poolName)
		}
		poolID = pool.ID
	}

	workerID := uuid.New().String()
	model := h.resolveModel(workerAgent.ID, "")

	// Init workspace
	workspacePath := ""
	agents, _ := h.DB.ListActiveToolAgentsByPool(poolID)
	if len(agents) > 0 {
		client := tools.NewAgentClient(agents[0].URL)
		ws, err := client.InitWorkspace(ctx, workerID)
		if err == nil {
			workspacePath = ws
		}
	}

	modelID := ""
	if model != nil {
		modelID = model.ModelID
	}

	title := task
	if len(title) > 80 {
		title = title[:80]
	}

	h.DB.CreateWorkerThread(workerID, title, parentThread.ID, workerAgent.ID, modelID, poolID, workspacePath)

	// Notify parent thread that a worker was dispatched
	h.Notify(SessionEvent{Type: "worker.dispatched", SessionID: parentThread.ID, Data: map[string]any{
		"thread_id":  workerID,
		"agent_name": workerAgent.Name,
		"task":       task,
	}})

	// Run worker agent loop asynchronously
	go h.runSession(workerID, workerAgent, model, task, nil)

	return workerID, nil
}

func (h *Handler) listWorkerThreads(parentThreadID string) ([]db.Thread, error) {
	return h.DB.GetActiveWorkers(parentThreadID)
}

func (h *Handler) interveneWorkerThread(ctx context.Context, workerThreadID, message string) error {
	// Cancel current turn, start new one with the message
	thread, err := h.DB.GetThread(workerThreadID)
	if err != nil {
		return err
	}
	agent, _ := h.DB.GetAgent(thread.AgentID)
	if agent == nil {
		return fmt.Errorf("agent not found")
	}
	model := h.resolveModel(agent.ID, "")
	h.cancelThread(workerThreadID)
	go h.runSession(workerThreadID, agent, model, message, nil)
	return nil
}

func (h *Handler) terminateWorkerThread(workerThreadID string) error {
	h.DB.SetThreadStatus(workerThreadID, "stopped")
	h.cancelThread(workerThreadID)
	return nil
}

// ── Turn completion ──

func (h *Handler) onTurnComplete(threadID string) {
	t, err := h.DB.GetThread(threadID)
	if err != nil {
		return
	}

	if t.ParentThreadID != "" {
		// Worker turn finished
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
		// Parent turn ended
		h.mu.Lock()
		_, exists := h.turnHandles[threadID]
		h.mu.Unlock()
		if exists {
			h.tryCheckCompletedWorkers(threadID)
		}
	}
}

func (h *Handler) tryAutoResumeParent(parentThreadID string) {
	h.mu.Lock()
	_, active := h.turnHandles[parentThreadID]
	h.mu.Unlock()
	if active {
		return
	}
	h.tryCheckCompletedWorkers(parentThreadID)
}

func (h *Handler) tryCheckCompletedWorkers(threadID string) {
	workers, err := h.DB.GetCompletedWorkers(threadID)
	if err != nil || len(workers) == 0 {
		return
	}

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

	var ids []string
	for _, w := range workers {
		ids = append(ids, w.ID)
	}
	h.DB.MarkWorkersDelivered(ids)

	t, err := h.DB.GetThread(threadID)
	if err != nil {
		return
	}
	agent, err := h.DB.GetAgent(t.AgentID)
	if err != nil {
		return
	}
	model := h.resolveModel(agent.ID, "")
	go h.runSession(threadID, agent, model, b.String(), nil)
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
		<-th.done
	}
}

// ── Helpers ──

func (h *Handler) resolveModel(agentID int64, overrideModelID string) *db.ModelProfile {
	if overrideModelID != "" {
		if m, err := h.DB.GetModelByID(overrideModelID); err == nil {
			return m
		}
	}
	if agentID != 0 {
		agent, err := h.DB.GetAgent(agentID)
		if err == nil && agent.DefaultModelID != "" {
			if m, err := h.DB.GetModelByID(agent.DefaultModelID); err == nil {
				return m
			}
		}
	}
	models, _ := h.DB.ListModels()
	if len(models) > 0 {
		return &models[0]
	}
	return nil
}

func (h *Handler) pickAgentFromPool(poolID int64) *db.ToolAgent {
	agents, _ := h.DB.ListActiveToolAgentsByPool(poolID)
	if len(agents) == 0 {
		return nil
	}
	return &agents[rand.Intn(len(agents))]
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

	// Subscribe first — captures live events that arrive during DB replay.
	ch := h.SSE.Subscribe(threadID)
	defer h.SSE.Unsubscribe(threadID, ch)

	// Replay missed events from DB.
	sinceID := int64(0)
	if v := c.Query("since_id"); v != "" {
		fmt.Sscanf(v, "%d", &sinceID)
	}
	var events []db.Event
	if sinceID > 0 {
		events, _ = h.DB.GetEvents(threadID, sinceID)
	} else {
		events, _ = h.DB.GetEventsSinceLastCompleted(threadID)
	}
	for _, e := range events {
		msg := fmt.Sprintf("event: %s\ndata: %s\n\n", e.Event, e.Payload)
		fmt.Fprint(c.Writer, msg)
	}
	c.Writer.Flush()

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

func (h *Handler) handleRenameThread(c *gin.Context) {
	var req struct {
		Title   string `json:"title"`
		AgentID *int64 `json:"agent_id"`
		ModelID string `json:"model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	threadID := c.Param("id")
	updates := map[string]any{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.AgentID != nil {
		updates["agent_id"] = *req.AgentID
	}
	if req.ModelID != "" {
		updates["model_id"] = req.ModelID
	}
	if len(updates) == 0 {
		c.JSON(400, gin.H{"error": "no fields to update"})
		return
	}
	updates["updated_at"] = time.Now()
	if err := h.DB.UpdateThread(threadID, updates); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	updated, _ := h.DB.GetThread(threadID)
	c.JSON(200, updated)
}

func (h *Handler) handleDeleteThread(c *gin.Context) {
	id := c.Param("id")
	h.cancelThread(id)

	// Cleanup workspace on tool agent
	thread, _ := h.DB.GetThread(id)
	if thread != nil && thread.PoolID > 0 {
		if agent := h.pickAgentFromPool(thread.PoolID); agent != nil {
			client := tools.NewAgentClient(agent.URL)
			client.CleanupWorkspace(context.Background(), id)
		}
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
	limit := 100
	if v := c.Query("limit"); v != "" {
		fmt.Sscanf(v, "%d", &limit)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	var msgs []db.Message
	var err error
	if afterID > 0 {
		msgs, err = h.DB.GetMessages(threadID, afterID)
	} else {
		msgs, err = h.DB.GetRecentMessagesByCount(threadID, limit)
	}
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []db.Message{}
	}
	// Return the oldest message ID so frontend can request older messages
	oldestID := int64(0)
	if len(msgs) > 0 {
		oldestID = msgs[0].ID
	}
	c.JSON(200, gin.H{"messages": msgs, "oldest_id": oldestID, "has_more": len(msgs) == limit})
}

// ── Pause / Resume / Intervene ──

func (h *Handler) handlePause(c *gin.Context) {
	threadID := c.Param("id")
	workers, _ := h.DB.GetActiveWorkers(threadID)
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

func (h *Handler) handleStopWorker(c *gin.Context) {
	workerID := c.Param("workerID")
	h.DB.SetThreadStatus(workerID, "stopped")
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
	go h.runSession(threadID, agent, model, "[System] Please continue.", nil)
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
	threadID := c.Param("id")
	h.cancelThread(threadID)

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
	go h.runSession(threadID, agent, model, req.Message, req.Images)
	c.JSON(200, gin.H{"status": "intervened"})
}

// ── Tools list ──

func (h *Handler) handleListTools(c *gin.Context) {
	localDefs := tools.LocalToolDefs()
	result := make([]map[string]any, 0, len(localDefs))
	for _, td := range localDefs {
		result = append(result, map[string]any{
			"name":        td.Name,
			"description": td.Description,
			"source":      "local",
		})
	}

	// Add remote tools from pools
	pools, _ := h.DB.ListPools()
	for _, p := range pools {
		if p.ToolDefs == "" || p.ToolDefs == "[]" {
			continue
		}
		var remoteDefs []engine.ToolDef
		if err := json.Unmarshal([]byte(p.ToolDefs), &remoteDefs); err == nil {
			for _, td := range remoteDefs {
				result = append(result, map[string]any{
					"name":        td.Name,
					"description": td.Description,
					"source":      "pool:" + p.Name,
				})
			}
		}
	}

	c.JSON(200, result)
}
