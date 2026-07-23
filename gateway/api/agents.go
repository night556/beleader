package api

import (
	"encoding/json"
	"strconv"
	"time"

	"beleader/gateway/db"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleListAgents(c *gin.Context) {
	agents, err := h.DB.ListAgents()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, agents)
}

func (h *Handler) handleCreateAgent(c *gin.Context) {
	var req struct {
		Name           string `json:"name"`
		Desc           string `json:"desc"`
		SystemPrompt   string `json:"system_prompt"`
		Tools          string `json:"tools"`
		DefaultModelID string `json:"default_model_id"`
		MCPServers     string `json:"mcp_servers"`
		WorkerAgents   string `json:"worker_agents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" || req.SystemPrompt == "" {
		c.JSON(400, gin.H{"error": "name and system_prompt required"})
		return
	}
	if req.Tools == "" {
		req.Tools = "[]"
	}
	if req.MCPServers == "" {
		req.MCPServers = "[]"
	}
	if req.WorkerAgents == "" {
		req.WorkerAgents = "[]"
	}
	if err := h.DB.CreateAgent(req.Name, req.Desc, req.SystemPrompt, req.Tools, req.DefaultModelID, req.MCPServers, req.WorkerAgents); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// Fetch and return the created agent
	agent, _ := h.DB.GetAgentByName(req.Name)
	c.JSON(201, agent)
}

func (h *Handler) handleUpdateAgent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name           string `json:"name"`
		Desc           string `json:"desc"`
		SystemPrompt   string `json:"system_prompt"`
		Tools          string `json:"tools"`
		DefaultModelID string `json:"default_model_id"`
		MCPServers     string `json:"mcp_servers"`
		WorkerAgents   string `json:"worker_agents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" || req.SystemPrompt == "" {
		c.JSON(400, gin.H{"error": "name and system_prompt required"})
		return
	}
	if err := h.DB.UpdateAgent(id, req.Name, req.Desc, req.SystemPrompt, req.Tools, req.DefaultModelID, req.MCPServers, req.WorkerAgents); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	a, _ := h.DB.GetAgent(id)
	c.JSON(200, a)
}

func (h *Handler) handleDeleteAgent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := h.DB.DeleteAgent(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
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
	c.JSON(200, dbModels)
}

func (h *Handler) handleCreateModel(c *gin.Context) {
	var m db.ModelProfile
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if m.ModelID == "" {
		c.JSON(400, gin.H{"error": "id is required"})
		return
	}
	if err := h.DB.CreateModel(&m); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, m)
}

func (h *Handler) handleUpdateModel(c *gin.Context) {
	var m db.ModelProfile
	if err := c.ShouldBindJSON(&m); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if m.ModelID == "" {
		c.JSON(400, gin.H{"error": "id is required"})
		return
	}
	if err := h.DB.UpdateModel(m.ModelID, &m); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleDeleteModel(c *gin.Context) {
	var req struct{ ID string `json:"id"` }
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.ID == "" {
		c.JSON(400, gin.H{"error": "id is required"})
		return
	}
	if err := h.DB.DeleteModel(req.ID); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}

// ── Pools ──

func (h *Handler) handleListPools(c *gin.Context) {
	pools, err := h.DB.ListPools()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if pools == nil {
		pools = []db.Pool{}
	}
	c.JSON(200, pools)
}

func (h *Handler) handleCreatePool(c *gin.Context) {
	var p db.Pool
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if p.Name == "" {
		c.JSON(400, gin.H{"error": "name is required"})
		return
	}
	if p.ToolDefs == "" {
		p.ToolDefs = "[]"
	}
	if err := h.DB.CreatePool(&p); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, p)
}

func (h *Handler) handleUpdatePool(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var p db.Pool
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	p.ID = id
	if err := h.DB.UpdatePool(&p); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, p)
}

func (h *Handler) handleDeletePool(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.DB.DeletePool(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}

func (h *Handler) handleSetDefaultPool(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.DB.SetDefaultPool(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

// ── Tool Agent registration ──

func (h *Handler) handleToolAgentRegister(c *gin.Context) {
	var req struct {
		Name              string             `json:"name" binding:"required"`
		URL               string             `json:"url" binding:"required"`
		Token             string             `json:"token" binding:"required"`
		Pool              string             `json:"pool"`
		WorkspaceRoot     string             `json:"workspace_root"`
		RestrictWorkspace bool               `json:"restrict_workspace"`
		Env               map[string]string  `json:"env"`
		ToolDefs          []json.RawMessage  `json:"tool_defs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if req.Token != h.RegToken {
		c.JSON(401, gin.H{"error": "invalid token"})
		return
	}

	poolName := req.Pool
	if poolName == "" {
		poolName = req.Name
	}

	// Find or create pool
	pool, err := h.DB.GetPoolByName(poolName)
	if err != nil {
		// Create new pool
		shell := ""
		platform := ""
		goVersion := ""
		if req.Env != nil {
			shell = req.Env["shell"]
			platform = req.Env["platform"]
			goVersion = req.Env["go_version"]
		}
		pool = &db.Pool{
			Name:              poolName,
			Shell:             shell,
			Platform:          platform,
			GoVersion:         goVersion,
			WorkspaceRoot:     req.WorkspaceRoot,
			RestrictWorkspace: req.RestrictWorkspace,
			ToolDefs:          "[]",
			IsDefault:         false,
		}
		// Check if this is the first pool → make it default
		pools, _ := h.DB.ListPools()
		if len(pools) == 0 {
			pool.IsDefault = true
		}
		h.DB.CreatePool(pool)
	}

	// Update tool defs if provided
	if len(req.ToolDefs) > 0 {
		toolDefsJSON, _ := json.Marshal(req.ToolDefs)
		h.DB.UpdatePoolToolDefs(pool.ID, string(toolDefsJSON))
	}

	// Register tool agent
	ta, err := h.DB.UpsertToolAgent(req.Name, req.URL, pool.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, ta)
}

func (h *Handler) handleToolAgentHeartbeat(c *gin.Context) {
	var req struct {
		ID     int64  `json:"id" binding:"required"`
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	if err := h.DB.UpdateToolAgentHeartbeat(req.ID, req.Status); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleListToolAgents(c *gin.Context) {
	// Delete stale records: heartbeat older than 60s
	cutoff := time.Now().Add(-60 * time.Second)
	h.DB.GORM.Where("last_heartbeat < ? AND status = ?", cutoff, "active").Delete(&db.ToolAgent{})

	agents, err := h.DB.ListToolAgents()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if agents == nil {
		agents = []db.ToolAgent{}
	}
	c.JSON(200, agents)
}

func (h *Handler) handleDeleteToolAgent(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.DB.DeleteToolAgent(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}
