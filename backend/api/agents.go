package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"beleader/backend/db"
	"beleader/backend/session"
	"beleader/backend/tools"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleListTools(c *gin.Context) {
	c.JSON(200, tools.Global.ListExposed())
}

func (h *Handler) handleListAgents(c *gin.Context) {
	agents, err := h.DB.ListAgents()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, agents)
}

func (h *Handler) handleUpdateAgentDesc(c *gin.Context) {
	var req struct {
		Name string `json:"name"`
		Desc string `json:"desc"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if err := h.DB.UpdateAgentDesc(req.Name, req.Desc); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleCreateAgent(c *gin.Context) {
	var req struct {
		Name       string `json:"name"`
		Desc       string `json:"desc"`
		Content    string `json:"content"`
		Type       string `json:"type"`
		Tools      string `json:"tools"`
		ToolAgents string `json:"tool_agents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" || req.Content == "" {
		c.JSON(400, gin.H{"error": "name and content required"})
		return
	}
	if err := h.DB.CreateAgent(req.Name, req.Content); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	updates := map[string]any{}
	if req.Desc != "" {
		updates["desc"] = req.Desc
	}
	if req.Type != "" {
		updates["type"] = req.Type
	}
	if req.Tools != "" {
		updates["tools"] = req.Tools
	}
	if req.ToolAgents != "" {
		updates["tool_agents"] = req.ToolAgents
	}
	if len(updates) > 0 {
		a, _ := h.DB.GetAgentByName(req.Name)
		if a != nil {
			h.DB.UpdateAgentByIDFull(a.ID, a.Name, valOr(updates, "desc", a.Desc), a.Content, valOr(updates, "type", a.Type), valOr(updates, "tools", a.Tools), valOr(updates, "tool_agents", a.ToolAgents))
		}
	}
	a, _ := h.DB.GetAgentByName(req.Name)
	c.JSON(200, a)
}

func valOr(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		return v.(string)
	}
	return fallback
}

func (h *Handler) handleUpdateAgent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name       string `json:"name"`
		Desc       string `json:"desc"`
		Content    string `json:"content"`
		Type       string `json:"type"`
		Tools      string `json:"tools"`
		ToolAgents string `json:"tool_agents"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" || req.Content == "" {
		c.JSON(400, gin.H{"error": "name and content required"})
		return
	}
	existing, err := h.DB.GetAgentByID(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "agent not found"})
		return
	}
	if err := h.DB.UpdateAgentByIDFull(id, req.Name, req.Desc, req.Content, req.Type, req.Tools, req.ToolAgents); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	_ = existing
	a, _ := h.DB.GetAgentByID(id)
	c.JSON(200, a)
}

func (h *Handler) handleDeleteAgent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := h.DB.DeleteAgentByID(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleGetSessionTokens(c *gin.Context) {
	sid := c.Param("id")
	total, err := h.DB.GetSessionTokens(sid)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	projectTotal := 0
	if sid != "main" {
		if pa, perr := h.DB.GetProjectAgent(sid); perr == nil && pa != nil {
			pt, _ := h.DB.GetProjectTotalTokens(pa.ProjectID)
			projectTotal = pt
		}
	}
	c.JSON(200, gin.H{"session_total": total, "project_total": projectTotal})
}

func (h *Handler) RegisterAgentTools() {
	registerAgentTools(h.SessionMgr, h.DB)
}

func registerAgentTools(mgr *session.Manager, database *db.DB) {
	// list_agents — returns name + desc only
	mgr.RegisterTool("list_agents", func(ctx context.Context, args string) *session.ToolResult {
		agents, err := database.ListAgents()
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		var lines []string
		for _, a := range agents {
			if a.Name == "coordinator" {
				continue
			}
			lines = append(lines, fmt.Sprintf("- **%s**: %s", a.Name, a.Desc))
		}
		return &session.ToolResult{Content: strings.Join(lines, "\n")}
	})

	// create_agent
	mgr.RegisterTool("create_agent", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Name    string `json:"name"`
			Desc    string `json:"desc"`
			Content string `json:"content"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.Name == "" || p.Content == "" {
			return &session.ToolResult{Error: "name and content required"}
		}
		if err := database.CreateAgent(p.Name, p.Content); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		if p.Desc != "" {
			database.UpdateAgentDesc(p.Name, p.Desc)
		}
		return &session.ToolResult{Content: "Agent created: " + p.Name}
	})

	// edit_agent
	mgr.RegisterTool("edit_agent", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Name    string `json:"name"`
			Desc    string `json:"desc"`
			Content string `json:"content"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.Name == "" || p.Content == "" {
			return &session.ToolResult{Error: "name and content required"}
		}
		if err := database.UpdateAgent(p.Name, p.Content); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		if p.Desc != "" {
			database.UpdateAgentDesc(p.Name, p.Desc)
		}
		return &session.ToolResult{Content: "Agent updated: " + p.Name}
	})

	// delete_agent
	mgr.RegisterTool("delete_agent", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Name string `json:"name"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.Name == "" {
			return &session.ToolResult{Error: "name required"}
		}
		if err := database.DeleteAgent(p.Name); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: "Agent deleted: " + p.Name}
	})
}
