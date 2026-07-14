package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"

	"beleader/gateway/db"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

// ToolResult is a local copy to avoid depending on the runtime engine package.
type ToolResult struct {
	Content string   `json:"content,omitempty"`
	Error   string   `json:"error,omitempty"`
	Images  []string `json:"images,omitempty"`
}

// ServerStatus holds the connection status of an MCP server.
type ServerStatus struct {
	Status string
	Error  string
}

// ToolEntry describes a registered MCP tool.
type ToolEntry struct {
	Name        string
	Description string
	Server      string
	InputSchema map[string]any
}

// Manager manages MCP client lifecycle.
type Manager struct {
	db      *db.DB
	mu      sync.RWMutex
	clients map[string]*clientEntry
	tools   map[string]*ToolEntry // fullName → entry
}

type clientEntry struct {
	client *client.Client
	config db.MCPServer
}

// NewManager creates a new MCP Manager.
func NewManager(d *db.DB) *Manager {
	return &Manager{
		db:      d,
		clients: make(map[string]*clientEntry),
		tools:   make(map[string]*ToolEntry),
	}
}

// Start loads all enabled MCP servers and connects to them.
func (m *Manager) Start() {
	servers, err := m.db.ListEnabledMCPServers()
	if err != nil {
		log.Printf("[MCP] list enabled servers: %v", err)
		return
	}
	for _, s := range servers {
		if err := m.Connect(s); err != nil {
			log.Printf("[MCP] connect %s: %v", s.Name, err)
		}
	}
}

// Stop disconnects all MCP servers.
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for name := range m.clients {
		m.disconnectLocked(name)
	}
}

// Connect establishes a connection to an MCP server and records its tools.
func (m *Manager) Connect(cfg db.MCPServer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.clients[cfg.Name]; ok {
		m.disconnectLocked(cfg.Name)
	}

	var c *client.Client
	var err error

	switch cfg.Type {
	case "stdio":
		c, err = m.connectStdio(cfg)
	case "http":
		c, err = m.connectHTTP(cfg)
	default:
		return fmt.Errorf("unknown MCP server type: %s", cfg.Type)
	}
	if err != nil {
		m.db.UpdateMCPServer(&db.MCPServer{
			ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, Enabled: cfg.Enabled,
			Command: cfg.Command, Args: cfg.Args, Env: cfg.Env,
			URL: cfg.URL, Headers: cfg.Headers, Status: "error", Error: err.Error(),
		})
		return err
	}

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
		c.Close()
		m.db.UpdateMCPServer(&db.MCPServer{
			ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, Enabled: cfg.Enabled,
			Command: cfg.Command, Args: cfg.Args, Env: cfg.Env,
			URL: cfg.URL, Headers: cfg.Headers, Status: "error", Error: "initialize: " + err.Error(),
		})
		return fmt.Errorf("initialize: %w", err)
	}

	listResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		c.Close()
		m.db.UpdateMCPServer(&db.MCPServer{
			ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, Enabled: cfg.Enabled,
			Command: cfg.Command, Args: cfg.Args, Env: cfg.Env,
			URL: cfg.URL, Headers: cfg.Headers, Status: "error", Error: "list tools: " + err.Error(),
		})
		return fmt.Errorf("list tools: %w", err)
	}

	// Record tools
	for _, t := range listResult.Tools {
		fullName := "mcp__" + cfg.Name + "__" + t.Name
		desc := t.Description
		if desc == "" {
			desc = t.Name
		}
		schema := convertInputSchema(t.InputSchema)
		m.tools[fullName] = &ToolEntry{
			Name:        fullName,
			Description: desc,
			Server:      cfg.Name,
			InputSchema: schema,
		}
	}

	m.clients[cfg.Name] = &clientEntry{client: c, config: cfg}

	c.OnConnectionLost(func(err error) {
		log.Printf("[MCP] connection lost: %s: %v", cfg.Name, err)
		m.mu.Lock()
		delete(m.clients, cfg.Name)
		m.removeToolsForServer(cfg.Name)
		m.mu.Unlock()
		m.db.UpdateMCPServer(&db.MCPServer{
			ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, Enabled: true,
			Command: cfg.Command, Args: cfg.Args, Env: cfg.Env,
			URL: cfg.URL, Headers: cfg.Headers, Status: "error", Error: err.Error(),
		})
	})

	m.db.UpdateMCPServer(&db.MCPServer{
		ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, Enabled: true,
		Command: cfg.Command, Args: cfg.Args, Env: cfg.Env,
		URL: cfg.URL, Headers: cfg.Headers, Status: "connected", Error: "",
	})

	log.Printf("[MCP] connected %s: %d tools", cfg.Name, len(listResult.Tools))
	return nil
}

// Disconnect disconnects an MCP server and removes its tools.
func (m *Manager) Disconnect(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.disconnectLocked(name)
}

func (m *Manager) disconnectLocked(name string) error {
	entry, ok := m.clients[name]
	if !ok {
		return fmt.Errorf("MCP server %s not connected", name)
	}
	if entry.client != nil {
		entry.client.Close()
	}
	delete(m.clients, name)
	m.removeToolsForServer(name)
	m.db.UpdateMCPServer(&db.MCPServer{
		ID: entry.config.ID, Name: name, Type: entry.config.Type, Enabled: entry.config.Enabled,
		Command: entry.config.Command, Args: entry.config.Args, Env: entry.config.Env,
		URL: entry.config.URL, Headers: entry.config.Headers, Status: "disconnected", Error: "",
	})
	log.Printf("[MCP] disconnected %s", name)
	return nil
}

// CallTool bridges a tool call to the MCP server.
func (m *Manager) CallTool(ctx context.Context, fullName, args string) *ToolResult {
	server, tool, ok := parseMCPToolName(fullName)
	if !ok {
		return &ToolResult{Error: "invalid MCP tool name: " + fullName}
	}

	m.mu.RLock()
	entry, ok := m.clients[server]
	m.mu.RUnlock()
	if !ok {
		return &ToolResult{Error: "MCP server not connected: " + server}
	}

	var parsedArgs map[string]any
	if args != "" {
		json.Unmarshal([]byte(args), &parsedArgs)
	}
	if parsedArgs == nil {
		parsedArgs = make(map[string]any)
	}

	result, err := entry.client.CallTool(ctx, mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      tool,
			Arguments: parsedArgs,
		},
	})
	if err != nil {
		log.Printf("[MCP] call tool %s/%s: %v", server, tool, err)
		m.db.UpdateMCPServer(&db.MCPServer{
			ID: entry.config.ID, Name: entry.config.Name, Type: entry.config.Type, Enabled: true,
			Command: entry.config.Command, Args: entry.config.Args, Env: entry.config.Env,
			URL: entry.config.URL, Headers: entry.config.Headers, Status: "error", Error: err.Error(),
		})
		return &ToolResult{Error: err.Error()}
	}

	if result.IsError {
		var errTexts []string
		for _, c := range result.Content {
			if textContent, ok := c.(mcp.TextContent); ok {
				errTexts = append(errTexts, textContent.Text)
			}
		}
		return &ToolResult{Error: strings.Join(errTexts, "\n")}
	}

	var contents []string
	var images []string
	for _, c := range result.Content {
		switch ct := c.(type) {
		case mcp.TextContent:
			contents = append(contents, ct.Text)
		case mcp.ImageContent:
			images = append(images, ct.Data)
		case mcp.AudioContent:
			contents = append(contents, "[audio: "+ct.MIMEType+"]")
		}
	}

	return &ToolResult{Content: strings.Join(contents, "\n"), Images: images}
}

// TestConnection connects, lists tools, and disconnects without persisting state.
func (m *Manager) TestConnection(cfg db.MCPServer) (int, []string, error) {
	var c *client.Client
	var err error

	switch cfg.Type {
	case "stdio":
		c, err = m.connectStdio(cfg)
	case "http":
		c, err = m.connectHTTP(cfg)
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

// Statuses returns the current status of all connected MCP servers.
func (m *Manager) Statuses() map[string]ServerStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]ServerStatus, len(m.clients))
	for name, entry := range m.clients {
		result[name] = ServerStatus{
			Status: entry.config.Status,
			Error:  entry.config.Error,
		}
	}
	return result
}

// GetConnectedServers returns names of currently connected servers.
func (m *Manager) GetConnectedServers() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var names []string
	for name := range m.clients {
		names = append(names, name)
	}
	return names
}

// ListTools returns all registered MCP tools.
func (m *Manager) ListTools() []ToolEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []ToolEntry
	for _, t := range m.tools {
		list = append(list, *t)
	}
	return list
}

// ── internal helpers ──

func (m *Manager) connectStdio(cfg db.MCPServer) (*client.Client, error) {
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

func (m *Manager) connectHTTP(cfg db.MCPServer) (*client.Client, error) {
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

func (m *Manager) removeToolsForServer(name string) {
	prefix := "mcp__" + name + "__"
	for k := range m.tools {
		if strings.HasPrefix(k, prefix) {
			delete(m.tools, k)
		}
	}
}

func convertInputSchema(schema mcp.ToolInputSchema) map[string]any {
	b, err := json.Marshal(schema)
	if err != nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	var result map[string]any
	json.Unmarshal(b, &result)
	if result == nil {
		return map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return result
}

func parseMCPToolName(fullName string) (string, string, bool) {
	if !strings.HasPrefix(fullName, "mcp__") {
		return "", "", false
	}
	rest := fullName[5:]
	idx := strings.Index(rest, "__")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+2:], true
}
