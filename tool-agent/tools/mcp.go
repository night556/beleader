package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClient manages a single HTTP MCP server connection.
type MCPClient struct {
	name    string
	client  *client.Client
	tools   []mcp.Tool
	mu      sync.RWMutex
}

// MCPManager manages all MCP server connections for this tool-agent.
type MCPManager struct {
	clients map[string]*MCPClient
	mu      sync.RWMutex
}

func NewMCPManager() *MCPManager {
	return &MCPManager{
		clients: make(map[string]*MCPClient),
	}
}

// MCPServerConfig is the config received from gateway during registration.
type MCPServerConfig struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

// ConnectAll connects to all MCP servers, adds their tools to the global
// toolDefs, and returns combined tool definitions as raw JSON.
func (m *MCPManager) ConnectAll(configs []MCPServerConfig) []json.RawMessage {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Close existing connections and remove old MCP tool defs
	for _, c := range m.clients {
		c.client.Close()
	}
	m.clients = make(map[string]*MCPClient)
	// Remove old MCP tools from toolDefs
	var nonMCP []ToolDef
	for _, td := range toolDefs {
		if !IsMCPTool(td.Name) {
			nonMCP = append(nonMCP, td)
		}
	}
	toolDefs = nonMCP

	for _, cfg := range configs {
		defs, err := m.connectOne(cfg)
		if err != nil {
			log.Printf("[mcp] failed to connect %s: %v", cfg.Name, err)
			continue
		}
		toolDefs = append(toolDefs, defs...)
		log.Printf("[mcp] connected to %s, %d tools", cfg.Name, len(defs))
	}

	// Return combined as raw JSON
	return AllToolDefs()
}

func (m *MCPManager) connectOne(cfg MCPServerConfig) ([]ToolDef, error) {
	var opts []transport.StreamableHTTPCOption
	if len(cfg.Headers) > 0 {
		opts = append(opts, transport.WithHTTPHeaders(cfg.Headers))
	}

	c, err := client.NewStreamableHttpClient(cfg.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("create client: %w", err)
	}

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		c.Close()
		return nil, fmt.Errorf("start: %w", err)
	}

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo: mcp.Implementation{
				Name:    "beleader-tool-agent",
				Version: "1.0",
			},
		},
	}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		c.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	listResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("list tools: %w", err)
	}

	mc := &MCPClient{
		name:   cfg.Name,
		client: c,
		tools:  listResult.Tools,
	}
	m.clients[cfg.Name] = mc

	var defs []ToolDef
	for _, t := range listResult.Tools {
		toolName := fmt.Sprintf("mcp__%s__%s", cfg.Name, t.Name)
		var params map[string]any
		raw, _ := json.Marshal(t.InputSchema)
		if len(raw) > 0 && string(raw) != "null" {
			json.Unmarshal(raw, &params)
		}
		defs = append(defs, ToolDef{
			Name:        toolName,
			Description: fmt.Sprintf("[MCP/%s] %s", cfg.Name, t.Description),
			Parameters:  params,
		})
	}
	return defs, nil
}

// ExecuteTool calls an MCP tool by its prefixed name.
func (m *MCPManager) ExecuteTool(toolName string, args string) *ToolResult {
	// Parse mcp__<server>__<tool>
	parts := strings.SplitN(toolName, "__", 3)
	if len(parts) != 3 || parts[0] != "mcp" {
		return &ToolResult{Error: "invalid MCP tool name: " + toolName}
	}
	serverName := parts[1]
	originalName := parts[2]

	m.mu.RLock()
	mc, ok := m.clients[serverName]
	m.mu.RUnlock()
	if !ok {
		return &ToolResult{Error: fmt.Sprintf("MCP server %s not connected", serverName)}
	}

	var arguments map[string]any
	if err := json.Unmarshal([]byte(args), &arguments); err != nil {
		return &ToolResult{Error: "invalid arguments: " + err.Error()}
	}

	ctx := context.Background()
	result, err := mc.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      originalName,
			Arguments: arguments,
		},
	})
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("MCP call failed: %v", err)}
	}

	// Extract text content from result
	var content strings.Builder
	for _, c := range result.Content {
		if textContent, ok := c.(mcp.TextContent); ok {
			content.WriteString(textContent.Text)
		}
	}

	return &ToolResult{Content: content.String()}
}

// IsMCPTool checks if a tool name is an MCP tool.
func IsMCPTool(name string) bool {
	return strings.HasPrefix(name, "mcp__")
}

// Close closes all MCP connections.
func (m *MCPManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		c.client.Close()
	}
	m.clients = make(map[string]*MCPClient)
}
