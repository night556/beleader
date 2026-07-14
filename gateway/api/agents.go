package api

import (
	"encoding/json"
	"fmt"
	"strconv"

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
		Name         string `json:"name"`
		Desc         string `json:"desc"`
		SystemPrompt string `json:"system_prompt"`
		Tools        string `json:"tools"`
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
	if err := h.DB.CreateAgent(req.Name, req.Desc, req.SystemPrompt, req.Tools); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(201, gin.H{"status": "ok"})
}

func (h *Handler) handleUpdateAgent(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name         string `json:"name"`
		Desc         string `json:"desc"`
		SystemPrompt string `json:"system_prompt"`
		Tools        string `json:"tools"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Name == "" || req.SystemPrompt == "" {
		c.JSON(400, gin.H{"error": "name and system_prompt required"})
		return
	}
	if err := h.DB.UpdateAgent(id, req.Name, req.Desc, req.SystemPrompt, req.Tools); err != nil {
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

// ── Knowledge handlers ──

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

// searchKnowledgeForLLM performs an FTS5 search on the knowledge base.
func (h *Handler) searchKnowledgeForLLM(query string, limit int) (string, error) {
	knowledge, err := h.DB.SearchKnowledge(query, limit)
	if err != nil {
		return "", err
	}
	b, _ := json.Marshal(knowledge)
	return string(b), nil
}

// saveKnowledgeForLLM saves a knowledge entry and returns its ID.
func (h *Handler) saveKnowledgeForLLM(title, content string) (int64, error) {
	return h.DB.InsertKnowledge(title, content, "main")
}

// deleteKnowledgeForLLM deletes a knowledge entry by ID.
func (h *Handler) deleteKnowledgeForLLM(id int64) error {
	return h.DB.DeleteKnowledge(id)
}
