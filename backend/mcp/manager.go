package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"

	"beleader/backend/db"
	"beleader/backend/session"
	"beleader/backend/tools"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/sashabaranov/go-openai"
)

// Manager manages MCP client lifecycle and bridges tools into the global Registry.
type Manager struct {
	db      *db.DB
	mu      sync.RWMutex
	clients map[string]*clientEntry
}

type clientEntry struct {
	client *client.Client
	config db.MCPServer
	tools  []mcp.Tool
}

// NewManager creates a new MCP Manager.
func NewManager(d *db.DB) *Manager {
	return &Manager{
		db:      d,
		clients: make(map[string]*clientEntry),
	}
}

// Start loads all enabled MCP servers and connects to them.
// Errors are logged; a single failed server does not block others.
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

// Connect establishes a connection to an MCP server and registers its tools.
func (m *Manager) Connect(cfg db.MCPServer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Disconnect existing if reconnecting
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

	// Initialize the client
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

	// List tools
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

	toolList := listResult.Tools
	if toolList == nil {
		toolList = []mcp.Tool{}
	}

	// Register each tool in the global registry
	var toolNames []string
	for _, t := range toolList {
		fullName := "mcp__" + cfg.Name + "__" + t.Name
		desc := t.Description
		if desc == "" {
			desc = t.Name
		}
		def := mcpToolToOpenAI(fullName, desc, t.InputSchema)
		tools.Global.Register(fullName, def, m.makeBridgeHandler(cfg.Name, t.Name, c), desc, "mcp")
		toolNames = append(toolNames, fullName)
	}

	// Create or update the corresponding tool_agent
	toolNamesJSON, _ := json.Marshal(toolNames)
	desc := cfg.Name + " MCP tools"
	existing, err := m.db.GetAgentByName(cfg.Name)
	if err == nil {
		m.db.UpdateAgentByIDFull(existing.ID, cfg.Name, desc, cfg.Name+" MCP tools", "", string(toolNamesJSON), "[]")
	} else {
		m.db.CreateAgent(cfg.Name, cfg.Name+" MCP tools")
		ag, _ := m.db.GetAgentByName(cfg.Name)
		if ag != nil {
			m.db.UpdateAgentByIDFull(ag.ID, cfg.Name, desc, cfg.Name+" MCP tools", "", string(toolNamesJSON), "[]")
		}
	}

	// Store entry
	m.clients[cfg.Name] = &clientEntry{client: c, config: cfg, tools: toolList}

	// Register connection lost callback (SSE/HTTP idle timeout, stdio process exit)
	c.OnConnectionLost(func(err error) {
		log.Printf("[MCP] connection lost: %s: %v", cfg.Name, err)
		m.mu.Lock()
		delete(m.clients, cfg.Name)
		m.mu.Unlock()
		tools.Global.UnregisterPrefix("mcp__" + cfg.Name + "__")
		m.db.DeleteAgent(cfg.Name)
		m.db.UpdateMCPServer(&db.MCPServer{
			ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, Enabled: true,
			Command: cfg.Command, Args: cfg.Args, Env: cfg.Env,
			URL: cfg.URL, Headers: cfg.Headers, Status: "error", Error: err.Error(),
		})
	})

	// Update DB status
	m.db.UpdateMCPServer(&db.MCPServer{
		ID: cfg.ID, Name: cfg.Name, Type: cfg.Type, Enabled: true,
		Command: cfg.Command, Args: cfg.Args, Env: cfg.Env,
		URL: cfg.URL, Headers: cfg.Headers, Status: "connected", Error: "",
	})

	log.Printf("[MCP] connected %s: %d tools", cfg.Name, len(toolList))
	return nil
}

// Disconnect disconnects an MCP server and unregisters its tools.
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

	// Unregister tools from global registry
	prefix := "mcp__" + name + "__"
	tools.Global.UnregisterPrefix(prefix)

	// Delete the corresponding tool_agent
	m.db.DeleteAgent(name)

	// Update DB status
	m.db.UpdateMCPServer(&db.MCPServer{
		ID: entry.config.ID, Name: name, Type: entry.config.Type, Enabled: entry.config.Enabled,
		Command: entry.config.Command, Args: entry.config.Args, Env: entry.config.Env,
		URL: entry.config.URL, Headers: entry.config.Headers, Status: "disconnected", Error: "",
	})

	log.Printf("[MCP] disconnected %s", name)
	return nil
}

// CallTool bridges a tool call to the MCP server.
func (m *Manager) CallTool(ctx context.Context, fullName, args string) *session.ToolResult {
	// Parse mcp__<server>__<tool>
	server, tool, ok := parseMCPToolName(fullName)
	if !ok {
		return &session.ToolResult{Error: "invalid MCP tool name: " + fullName}
	}

	m.mu.RLock()
	entry, ok := m.clients[server]
	m.mu.RUnlock()
	if !ok {
		return &session.ToolResult{Error: "MCP server not connected: " + server}
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
		return &session.ToolResult{Error: err.Error()}
	}

	if result.IsError {
		// Extract error text from content
		var errTexts []string
		for _, c := range result.Content {
			if textContent, ok := c.(mcp.TextContent); ok {
				errTexts = append(errTexts, textContent.Text)
			}
		}
		return &session.ToolResult{Error: strings.Join(errTexts, "\n")}
	}

	// Extract text/images from result
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

	return &session.ToolResult{
		Content: strings.Join(contents, "\n"),
		Images:  images,
	}
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

	// On Windows, wrap npx/npx.cmd in cmd /c to ensure proper subprocess spawning
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

	// Prefer streamable HTTP
	c, err := client.NewStreamableHttpClient(cfg.URL)
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

func (m *Manager) makeBridgeHandler(serverName, toolName string, c *client.Client) func(ctx context.Context, args string) *session.ToolResult {
	fullName := "mcp__" + serverName + "__" + toolName
	return func(ctx context.Context, args string) *session.ToolResult {
		return m.CallTool(ctx, fullName, args)
	}
}

// mcpToolToOpenAI converts an MCP tool to an OpenAI tool definition.
func mcpToolToOpenAI(name, desc string, inputSchema mcp.ToolInputSchema) openai.Tool {
	params := convertMCPSchemaToOpenAI(inputSchema)
	return openai.Tool{
		Type: "function",
		Function: &openai.FunctionDefinition{
			Name:        name,
			Description: desc,
			Parameters:  params,
		},
	}
}

// convertMCPSchemaToOpenAI converts MCP JSON Schema to OpenAI's expected map format.
func convertMCPSchemaToOpenAI(schema mcp.ToolInputSchema) map[string]any {
	// Marshal the schema to JSON and unmarshal to map[string]any
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

// parseMCPToolName parses mcp__<server>__<tool> into (server, tool, ok).
func parseMCPToolName(fullName string) (string, string, bool) {
	if !strings.HasPrefix(fullName, "mcp__") {
		return "", "", false
	}
	rest := fullName[5:] // remove "mcp__"
	idx := strings.Index(rest, "__")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+2:], true
}
