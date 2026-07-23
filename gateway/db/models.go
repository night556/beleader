package db

import "time"


type Pool struct {
	ID                int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name              string    `gorm:"size:64;uniqueIndex" json:"name"`
	Shell             string    `gorm:"size:128;default:''" json:"shell"`
	Platform          string    `gorm:"size:64;default:''" json:"platform"`
	GoVersion         string    `gorm:"size:32;default:''" json:"go_version"`
	WorkspaceRoot     string    `gorm:"size:512;default:''" json:"workspace_root"`
	RestrictWorkspace bool      `gorm:"default:false;column:restrict_workspace" json:"restrict_workspace"`
	ToolDefs          string    `gorm:"type:text;default:'[]';column:tool_defs" json:"tool_defs"`
	IsDefault         bool      `gorm:"default:false;column:is_default" json:"is_default"`
	CreatedAt         time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Pool) TableName() string { return "pools" }

// ── ToolAgent ──

type ToolAgent struct {
	ID            int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string    `gorm:"size:128;uniqueIndex" json:"name"`
	URL           string    `gorm:"size:256" json:"url"`
	PoolID        int64     `gorm:"index" json:"pool_id"`
	Status        string    `gorm:"size:16;default:'active'" json:"status"`
	LastHeartbeat time.Time `gorm:"autoCreateTime" json:"last_heartbeat"`
	CreatedAt     time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (ToolAgent) TableName() string { return "tool_agents" }

// ── Thread ──

type Thread struct {
	ID              string    `gorm:"primaryKey;size:64" json:"id"`
	Title           string    `gorm:"default:''" json:"title"`
	AgentID         int64     `gorm:"default:0" json:"agent_id"`
	ModelID         string    `gorm:"size:64;default:''" json:"model_id"`
	PoolID          int64     `gorm:"default:0;index" json:"pool_id"`
	WorkspacePath   string    `gorm:"size:512;default:''" json:"workspace_path"`
	ParentThreadID  string    `gorm:"size:64;default:'';index;column:parent_thread_id" json:"parent_thread_id"`
	Status          string    `gorm:"size:16;default:'idle';column:status" json:"status"`
	ResultDelivered bool      `gorm:"default:false;column:result_delivered" json:"result_delivered"`
	StatusContent   string    `gorm:"type:text;default:''" json:"status_content"`
	ContextStartID  int64     `gorm:"default:0" json:"context_start_id"`
	PinnedIDs       string    `gorm:"type:text;default:'[]'" json:"pinned_ids"`
	TotalTokens     int       `gorm:"default:0" json:"total_tokens"`
	MaxContextPct   int       `gorm:"default:80" json:"max_context_pct"`
	CreatedAt       time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt       time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Thread) TableName() string { return "threads" }

// ── Message ──

type Message struct {
	ID               int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID         string    `gorm:"size:64;index" json:"thread_id"`
	TurnID           string    `gorm:"size:64;default:'';index" json:"turn_id"`
	Kind             string    `gorm:"size:32" json:"kind"`
	Content          string    `gorm:"type:text;default:''" json:"content"`
	MultiContent     string    `gorm:"type:text;default:''" json:"multi_content"`
	ToolCalls        string    `gorm:"type:text;default:'[]'" json:"tool_calls"`
	ToolCallID       string    `gorm:"size:64;default:''" json:"tool_call_id"`
	ReasoningContent string    `gorm:"type:text;default:''" json:"reasoning_content"`
	Usage            string    `gorm:"type:text;default:''" json:"usage"`
	CreatedAt        time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (Message) TableName() string { return "messages" }

// ── Event ──

type Event struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID  string    `gorm:"size:64;index" json:"thread_id"`
	TurnID    string    `gorm:"size:64;default:''" json:"turn_id"`
	ItemID    string    `gorm:"size:64;default:''" json:"item_id"`
	Event     string    `gorm:"size:32" json:"event"`
	Payload   string    `gorm:"type:text" json:"payload"`
	CreatedAt time.Time `gorm:"autoCreateTime;index" json:"created_at"`
}

func (Event) TableName() string { return "events" }

// ── Agent ──

type Agent struct {
	ID             int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string    `gorm:"size:128;uniqueIndex" json:"name"`
	Desc           string    `gorm:"size:512;default:''" json:"desc"`
	SystemPrompt   string    `gorm:"type:text;default:''" json:"system_prompt"`
	Tools          string    `gorm:"type:text;default:'[]'" json:"tools"`
	DefaultModelID string    `gorm:"size:64;default:''" json:"default_model_id"`
	MCPServers     string    `gorm:"type:text;default:'[]'" json:"mcp_servers"`
	WorkerAgents   string    `gorm:"type:text;default:'[]'" json:"worker_agents"`
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (Agent) TableName() string { return "agents" }

// ── MCPServer ──

type MCPServer struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"size:64;uniqueIndex" json:"name"`
	Type      string    `gorm:"size:16" json:"type"`
	Enabled   bool      `gorm:"default:false" json:"enabled"`
	Command   string    `gorm:"size:512;default:''" json:"command"`
	Args      string    `gorm:"type:text;default:'[]'" json:"args"`
	Env       string    `gorm:"type:text;default:'{}'" json:"env"`
	URL       string    `gorm:"size:512;default:''" json:"url"`
	Headers   string    `gorm:"type:text;default:'{}'" json:"headers"`
	Status    string    `gorm:"size:16;default:'disconnected'" json:"status"`
	Error     string    `gorm:"size:512;default:''" json:"error"`
	PoolID    int64     `gorm:"default:0" json:"pool_id"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}

func (MCPServer) TableName() string { return "mcp_servers" }

// ── ModelProfile ──

type ModelProfile struct {
	ID              int64  `gorm:"primaryKey;autoIncrement" json:"-"`
	ModelID         string `gorm:"size:64;uniqueIndex;column:model_id" json:"id"`
	BaseURL         string `gorm:"size:512;default:'';column:base_url" json:"base_url"`
	APIKey          string `gorm:"size:512;default:'';column:api_key" json:"api_key"`
	Model           string `gorm:"size:128;default:'';column:model" json:"model"`
	Vision          bool   `gorm:"default:false" json:"vision"`
	ContextLimit    int    `gorm:"default:128000;column:context_limit" json:"context_limit"`
	ReasoningEffort string `gorm:"size:16;default:'high';column:reasoning_effort" json:"reasoning_effort"`
	IsActive        bool   `gorm:"default:false;column:is_active" json:"-"`
}

func (ModelProfile) TableName() string { return "model_profiles" }

// ── Seed ──

func (db *DB) seedDefaultAgent() {
	var count int64

	// Default: general-purpose worker agent.
	// Has file ops, shell, web, worker spawning, and STATUS.md.
	defaultTools := `["read_file","read_dir","write_file","edit_file","delete_file","search_content","search_files","run_command","task_output","task_stop","web_search","web_fetch","run_http_request","read_status","update_status","spawn_worker","list_workers","intervene_worker","terminate_worker"]`
	if db.GORM.Model(&Agent{}).Where("name = 'Default'").Count(&count); count == 0 {
		db.GORM.Create(&Agent{
			Name:         "Default",
			Desc:         "General-purpose assistant — read, write, edit files, run commands, search the web, spawn workers for parallel tasks",
			SystemPrompt: "You are a helpful AI assistant. You can read and write files, run shell commands, search the web, and spawn sub-agents for complex tasks.\n\n## How to work\n- Before editing files, read them first to understand the current state\n- After making changes, verify they work — run tests or check the build\n- When a task is complex or can be parallelized, use spawn_worker to delegate sub-tasks\n- Use read_status and update_status to persist important context, progress, and decisions across turns\n- When done, summarize what you accomplished",
			Tools:        defaultTools,
			WorkerAgents: "[]",
		})
	}

	// Manager: system management agent.
	// Has management tools + read-only file access + STATUS.md. No run_command, no spawn_worker.
	managerTools := `["create_agent","update_agent","delete_agent","list_agents","create_mcp_server","delete_mcp_server","list_mcp_servers","create_model","list_resources","read_file","read_dir","search_content","read_status","update_status","web_search","web_fetch"]`
	if db.GORM.Model(&Agent{}).Where("name = 'Manager'").Count(&count); count == 0 {
		db.GORM.Create(&Agent{
			Name:         "Manager",
			Desc:         "System manager — create and configure agents, MCP servers, models, and other resources",
			SystemPrompt: "You are a system management assistant. You can create, update, delete, and list agents, MCP servers, models, and other resources.\n\n## How to work\n- Before creating resources, list existing ones to understand the current state\n- When creating resources, validate that required fields are provided\n- Use read_status and update_status to track system state across turns\n- You can read files and search code to understand the project, but you cannot run commands or modify files directly — delegate that to the Default agent or a worker",
			Tools:        managerTools,
			WorkerAgents: "[]",
		})
	}
}
