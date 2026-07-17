package api

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strconv"

	"beleader/gateway/db"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
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

	count, names, err := testMCPConnection(*existing)
	if err != nil {
		c.JSON(200, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"success": true, "tool_count": count, "tools": names})
}

// testMCPConnection connects to an MCP server, lists tools, and disconnects.
// Standalone — does not use the Runtime MCP manager.
func testMCPConnection(cfg db.MCPServer) (int, []string, error) {
	var c *client.Client
	var err error

	switch cfg.Type {
	case "stdio":
		c, err = connectStdioForTest(cfg)
	case "http":
		c, err = connectHTTPForTest(cfg)
	default:
		return 0, nil, fmt.Errorf("unknown MCP server type: %s", cfg.Type)
	}
	if err != nil {
		return 0, nil, err
	}
	defer c.Close()

	ctx := context.Background()
	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "BeLeader",
				Version: "1.0.0",
			},
		},
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		return 0, nil, fmt.Errorf("initialize: %w", err)
	}

	listResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return 0, nil, fmt.Errorf("list tools: %w", err)
	}

	var names []string
	for _, t := range listResult.Tools {
		names = append(names, t.Name)
	}
	return len(names), names, nil
}

func connectStdioForTest(cfg db.MCPServer) (*client.Client, error) {
	var args []string
	if cfg.Args != "" {
		json.Unmarshal([]byte(cfg.Args), &args)
	}

	var env []string
	if cfg.Env != "" {
		var envMap map[string]string
		json.Unmarshal([]byte(cfg.Env), &envMap)
		for k, v := range envMap {
			env = append(env, k+"="+v)
		}
	}

	command := cfg.Command
	if runtime.GOOS == "windows" && (command == "npx" || command == "npx.cmd") {
		allArgs := append([]string{"/c", command}, args...)
		command = "cmd"
		args = allArgs
	}

	c, err := client.NewStdioMCPClient(command, env, args...)
	if err != nil {
		return nil, fmt.Errorf("create stdio client: %w", err)
	}

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("start stdio transport: %w", err)
	}

	return c, nil
}

func connectHTTPForTest(cfg db.MCPServer) (*client.Client, error) {
	var headers map[string]string
	if cfg.Headers != "" {
		json.Unmarshal([]byte(cfg.Headers), &headers)
	}

	var opts []transport.StreamableHTTPCOption
	if len(headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(headers))
	}

	c, err := client.NewStreamableHttpClient(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("create http client: %w", err)
	}

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("start http transport: %w", err)
	}

	return c, nil
}
