package api

import (
	"strconv"
	"time"

	"beleader/gateway/db"

	"github.com/gin-gonic/gin"
)

// ── Runtime registration (Runtime → Gateway) ──

func (h *Handler) handleRuntimeRegister(c *gin.Context) {
	var req struct {
		Name              string `json:"name" binding:"required"`
		URL               string `json:"url" binding:"required"`
		Token             string `json:"token" binding:"required"`
		RestrictWorkspace bool   `json:"restrict_workspace"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	if req.Token != h.RegToken {
		c.JSON(401, gin.H{"error": "invalid token"})
		return
	}

	runtime, err := h.DB.UpsertRuntime(req.Name, req.URL, req.RestrictWorkspace)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	h.Runtime.SetBaseURL(req.URL)
	h.Runtime.RestrictWorkspace = req.RestrictWorkspace

	c.JSON(200, runtime)
}

func (h *Handler) handleRuntimeHeartbeat(c *gin.Context) {
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

	if err := h.DB.UpdateRuntimeHeartbeat(req.ID, req.Status); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func (h *Handler) handleListRuntimes(c *gin.Context) {
	// Lazy stale check: mark runtimes with last_heartbeat > 60s as inactive.
	cutoff := time.Now().Add(-60 * time.Second)
	h.DB.GORM.Model(&db.Runtime{}).
		Where("last_heartbeat < ? AND status = ?", cutoff, "active").
		Update("status", "inactive")

	runtimes, err := h.DB.ListRuntimes()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	if runtimes == nil {
		runtimes = []db.Runtime{}
	}
	c.JSON(200, runtimes)
}

func (h *Handler) handleDeleteRuntime(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}
	if err := h.DB.DeleteRuntime(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "deleted"})
}
