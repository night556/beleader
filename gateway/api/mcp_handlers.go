package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

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
		c.JSON(200, gin.H{"success": false, "error": "only http type is supported for testing"})
		return
	}

	count, names, err := testHTTPMCPConnection(existing.URL, existing.Headers)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"success": true, "tool_count": count, "tools": names})
}

// testHTTPMCPConnection connects to an HTTP MCP server via JSON-RPC,
// initializes, and lists tools.
func testHTTPMCPConnection(url, headersJSON string) (int, []string, error) {
	var headers map[string]string
	if headersJSON != "" && headersJSON != "{}" {
		json.Unmarshal([]byte(headersJSON), &headers)
	}

	// 1. Initialize
	initReq, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"clientInfo":      map[string]string{"name": "BeLeader", "version": "1.0"},
		},
	})
	if _, err := mcpHTTPPost(url, headers, initReq); err != nil {
		return 0, nil, fmt.Errorf("initialize: %w", err)
	}

	// 2. List tools
	listReq, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	respBody, err := mcpHTTPPost(url, headers, listReq)
	if err != nil {
		return 0, nil, fmt.Errorf("list tools: %w", err)
	}

	var listResp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error any `json:"error"`
	}
	json.Unmarshal(respBody, &listResp)
	if listResp.Error != nil {
		return 0, nil, fmt.Errorf("server error: %v", listResp.Error)
	}

	var names []string
	for _, t := range listResp.Result.Tools {
		names = append(names, t.Name)
	}
	return len(names), names, nil
}

func mcpHTTPPost(url string, headers map[string]string, body []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return io.ReadAll(io.LimitReader(resp.Body, 500000))
}
