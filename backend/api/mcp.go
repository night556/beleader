package api

import (
	"strconv"

	"beleader/backend/db"

	"github.com/gin-gonic/gin"
)

func (h *Handler) handleListMCPServers(c *gin.Context) {
	servers, err := h.DB.ListMCPServers()
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	// Inject live connection status
	if h.MCPMgr != nil {
		connected := h.MCPMgr.GetConnectedServers()
		connectedSet := make(map[string]bool, len(connected))
		for _, name := range connected {
			connectedSet[name] = true
		}
		for i := range servers {
			if connectedSet[servers[i].Name] {
				servers[i].Status = "connected"
			}
		}
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

	// If enabled, connect immediately
	if s.Enabled && h.MCPMgr != nil {
		if err := h.MCPMgr.Connect(s); err != nil {
			c.JSON(201, gin.H{"server": s, "warning": "created but connect failed: " + err.Error()})
			return
		}
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

	// Get existing to compare status
	existing, err := h.DB.GetMCPServerByID(id)
	if err != nil {
		c.JSON(404, gin.H{"error": "MCP server not found"})
		return
	}

	if err := h.DB.UpdateMCPServer(&s); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Handle connect/disconnect based on enabled and name change
	if h.MCPMgr != nil {
		nameChanged := s.Name != existing.Name
		if existing.Enabled {
			if !s.Enabled || nameChanged {
				h.MCPMgr.Disconnect(existing.Name)
			}
		}
		if s.Enabled {
			h.MCPMgr.Connect(s)
		}
	}

	c.JSON(200, s)
}

func (h *Handler) handleDeleteMCPServer(c *gin.Context) {
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

	// Disconnect first
	if h.MCPMgr != nil {
		h.MCPMgr.Disconnect(existing.Name)
	}

	if err := h.DB.DeleteMCPServer(id); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "ok"})
}

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

	if h.MCPMgr == nil {
		c.JSON(500, gin.H{"error": "MCP manager not initialized"})
		return
	}

	count, names, err := h.MCPMgr.TestConnection(*existing)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"success": true, "tool_count": count, "tools": names})
}

func (h *Handler) handleConnectMCPServer(c *gin.Context) {
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

	if h.MCPMgr == nil {
		c.JSON(500, gin.H{"error": "MCP manager not initialized"})
		return
	}

	if err := h.MCPMgr.Connect(*existing); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Mark enabled
	h.DB.UpdateMCPServer(&db.MCPServer{
		ID: id, Name: existing.Name, Type: existing.Type, Enabled: true,
		Command: existing.Command, Args: existing.Args, Env: existing.Env,
		URL: existing.URL, Headers: existing.Headers, Status: "connected", Error: "",
	})

	c.JSON(200, gin.H{"status": "connected"})
}

func (h *Handler) handleDisconnectMCPServer(c *gin.Context) {
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

	if h.MCPMgr == nil {
		c.JSON(500, gin.H{"error": "MCP manager not initialized"})
		return
	}

	if err := h.MCPMgr.Disconnect(existing.Name); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	// Mark disabled
	h.DB.UpdateMCPServer(&db.MCPServer{
		ID: id, Name: existing.Name, Type: existing.Type, Enabled: false,
		Command: existing.Command, Args: existing.Args, Env: existing.Env,
		URL: existing.URL, Headers: existing.Headers, Status: "disconnected", Error: "",
	})

	c.JSON(200, gin.H{"status": "disconnected"})
}
