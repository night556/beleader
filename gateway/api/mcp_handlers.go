package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"beleader/gateway/db"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleListMCPServers(c *gin.Context) {
	servers, err := h.DB.ListMCPServers()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, servers)
}

func (h *Handler) handleCreateMCPServer(c *gin.Context) {
	var s db.MCPServer
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if s.Name == "" {
		c.JSON(400, gin.H{"error": "name required"})
		return
	}
	if s.Type != "stdio" && s.Type != "http" {
		c.JSON(400, gin.H{"error": "type must be 'stdio' or 'http'"})
		return
	}

	if err := h.DB.CreateMCPServer(&s); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(201, s)
}

func (h *Handler) handleUpdateMCPServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	var s db.MCPServer
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	s.ID = id

	if s.Name == "" {
		c.JSON(400, gin.H{"error": "name required"})
		return
	}
	if s.Type != "stdio" && s.Type != "http" {
		c.JSON(400, gin.H{"error": "type must be 'stdio' or 'http'"})
		return
	}

	if err := h.DB.UpdateMCPServer(&s); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, s)
}

func (h *Handler) handleDeleteMCPServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	if err := h.DB.DeleteMCPServer(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "ok"})
}

// handleTestMCPServer proxies the test request to a tool-agent in the
// MCP server's pool. The tool-agent has mcp-go and handles the actual
// MCP protocol (initialize, list tools, SSE, etc.).
func (h *Handler) handleTestMCPServer(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "invalid id"})
		return
	}

	existing, err := h.DB.GetMCPServerByID(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "MCP server not found"})
		return
	}

	if existing.Type != "http" {
		c.JSON(200, gin.H{"success": false, "error": "only http type is supported"})
		return
	}

	// Find a tool-agent in the MCP server's pool
	agents, _ := h.DB.ListActiveToolAgentsByPool(existing.PoolID)
	if len(agents) == 0 {
		c.JSON(200, gin.H{"success": false, "error": "no tool-agent available in pool"})
		return
	}

	// Forward to tool-agent's /mcp/test endpoint
	var headers map[string]string
	if existing.Headers != "" && existing.Headers != "{}" {
		json.Unmarshal([]byte(existing.Headers), &headers)
	}

	testReq, _ := json.Marshal(map[string]any{
		"url":     existing.URL,
		"headers": headers,
	})

	resp, err := http.Post(agents[0].URL+"/mcp/test", "application/json", strings.NewReader(string(testReq)))
	if err != nil {
		c.JSON(200, gin.H{"success": false, "error": "tool-agent unreachable: " + err.Error()})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 50000))
	c.Data(resp.StatusCode, "application/json", body)
}
