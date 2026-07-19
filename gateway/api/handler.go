package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"beleader/gateway/config"
	"beleader/gateway/db"
	"beleader/gateway/llm"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Handler is the Gateway HTTP handler.
type Handler struct {
	DB       *db.DB
	LLM      *llm.Client
	Config   *config.Config
	SSE      *SSEBroker
	Runtimes *RuntimeClientPool

	RegToken   string // shared secret for runtime registration, from GATEWAY_TOKEN env var

	cancelFuncs map[string]context.CancelFunc
	turnTokens  map[string]int64
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
		cancelFuncs: make(map[string]context.CancelFunc),
		turnTokens:  make(map[string]int64),
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

		api.GET("/models", h.handleListModels)
		api.POST("/models", h.handleCreateModel)
		api.PUT("/models/:id", h.handleUpdateModel)
		api.DELETE("/models/:id", h.handleDeleteModel)

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

		rtID, err := h.createRuntimeThread(runtime, agent, model)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		threadID = rtID

		title := req.Message
		if len(title) > 80 {
			title = title[:80]
		}
		if err := h.DB.CreateThread(threadID, title, agent.ID, modelID); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	} else {
		h.cancelThread(threadID)
	}

	// Persist user message to DB.
	h.DB.InsertMessage(&db.Message{
		ThreadID:     threadID,
		Kind:         "user_message",
		Content:      req.Message,
		MultiContent: encodeMultiContent(req.Images),
	})

	// Create cancellable context for this turn so pause/stop can abort the HTTP request.
	turnCtx, turnCancel := context.WithCancel(context.Background())
	h.mu.Lock()
	token := time.Now().UnixNano()
	h.cancelFuncs[threadID] = turnCancel
	h.turnTokens[threadID] = token
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.turnTokens[threadID] == token {
			delete(h.cancelFuncs, threadID)
			delete(h.turnTokens, threadID)
		}
		h.mu.Unlock()
		turnCancel()
	}()

	// Call Runtime to start the turn and stream SSE back.
	threadDir := filepath.Join(h.Config.DataDir, "threads", threadID)
	workspaceDir := filepath.Join(threadDir, "workspace")
	resp, err := runtime.SendTurn(turnCtx, threadID, TurnRequest{
		Message:      req.Message,
		Images:       req.Images,
		Model:        h.buildModelMap(model),
		ThreadDir:    threadDir,
		WorkspaceDir: workspaceDir,
	})
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("X-Thread-Id", threadID)
	c.Header("Cache-Control", "no-cache")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	flusher, _ := c.Writer.(http.Flusher)
	thinkingAcc := map[string]string{} // item_id → accumulated thinking
	ParseAndForwardSSE(resp.Body, c.Writer, flusher, func(eventType string, envelope map[string]any) {
		payload, _ := envelope["payload"].(map[string]any)
		if payload == nil {
			payload = map[string]any{}
		}

		switch eventType {
		case "item.delta":
			itemID, _ := envelope["item_id"].(string)
			kind, _ := payload["kind"].(string)
			delta, _ := payload["delta"].(string)
			if itemID != "" && kind == "thinking" {
				thinkingAcc[itemID] += delta
			}

		case "item.started":
			item, _ := payload["item"].(map[string]any)
			if item != nil {
				kind, _ := item["kind"].(string)
				// Reset thinking accumulator for this item.
				if id, _ := item["id"].(string); id != "" {
					delete(thinkingAcc, id)
				}
				if kind == "tool_call" {
					metadata, _ := item["metadata"].(map[string]any)
					toolName, _ := metadata["tool_name"].(string)
					toolUseID, _ := metadata["tool_use_id"].(string)
					tcsJSON, _ := json.Marshal([]map[string]any{{
						"id":   toolUseID,
						"type": "function",
						"function": map[string]any{
							"name": toolName,
						},
					}})
					h.DB.InsertMessage(&db.Message{
						ThreadID:  threadID,
						Kind:      "tool_call",
						ToolCalls: string(tcsJSON),
					})
				}
			}

		case "item.completed":
			item, _ := payload["item"].(map[string]any)
			if item != nil {
				kind, _ := item["kind"].(string)
				detail, _ := item["detail"].(string)
				itemID, _ := item["id"].(string)
				switch kind {
				case "agent_message":
					h.DB.InsertMessage(&db.Message{
						ThreadID:         threadID,
						Kind:             "agent_message",
						Content:          detail,
						ReasoningContent: thinkingAcc[itemID],
					})
					delete(thinkingAcc, itemID)
				case "tool_call":
					metadata, _ := item["metadata"].(map[string]any)
					toolUseID, _ := metadata["tool_use_id"].(string)
					if toolUseID != "" {
						dbContent := detail
						if m, ok := parseJSONMap(detail); ok {
							delete(m, "images")
							if b, err := json.Marshal(m); err == nil {
								dbContent = string(b)
							}
						}
						h.DB.InsertMessage(&db.Message{
							ThreadID:   threadID,
							Kind:       "tool_result",
							Content:    dbContent,
							ToolCallID: toolUseID,
						})
					}
				}
			}

		case "item.failed":
			item, _ := payload["item"].(map[string]any)
			if item != nil {
				detail, _ := item["detail"].(string)
				h.DB.InsertMessage(&db.Message{
					ThreadID: threadID,
					Kind:     "error",
					Content:  detail,
				})
			}
		}
	})
}

func (h *Handler) cancelThread(threadID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancel, ok := h.cancelFuncs[threadID]; ok {
		cancel()
		delete(h.cancelFuncs, threadID)
		delete(h.turnTokens, threadID)
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
	h.cancelThread(c.Param("id"))
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
		ModelID:         req.ID,
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
	modelID := c.Param("id")
	var req config.ModelProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	m := &db.ModelProfile{
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
	if err := h.DB.DeleteModel(c.Param("id")); err != nil {
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

func (h *Handler) createRuntimeThread(runtime *RuntimeClient, agent *db.Agent, model *db.ModelProfile) (string, error) {
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
	workspaceDir := filepath.Join(threadDir, "workspace")

	req := CreateThreadRequest{
		ThreadID:          threadID,
		ThreadDir:         threadDir,
		WorkspaceDir:      workspaceDir,
		RestrictWorkspace: h.Config.RestrictWorkspace,
		SystemPrompt:      agent.SystemPrompt,
		Model:             h.buildModelMap(model),
		Tools:             filterToolsByName(allTools, toolNames),
		MCPServers:        mcpConfigs,
		Metadata: map[string]any{
			"agent_id": agent.ID,
		},
	}

	if _, err := runtime.CreateThread(req); err != nil {
		return "", err
	}
	return threadID, nil
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
