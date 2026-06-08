package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"beleader/backend/db"
	"beleader/backend/session"

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
