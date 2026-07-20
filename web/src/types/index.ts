// ── Timeline / Message ──

export interface TokenUsage {
  prompt: number;
  completion: number;
  total: number;
  cached: number;
}

export type TimelineItemType = 'user' | 'agent' | 'tool_call' | 'tool_result' | 'error' | 'notice' | 'worker';

export interface TimelineItem {
  id: string;
  type: TimelineItemType;
  icon?: string;
  label: string;
  content: string;
  html?: string;
  status: 'pending' | 'streaming' | 'done' | 'fail';
  threadId?: string;
  toolCallId?: string;
  toolName?: string;
  thinking?: string;
  args?: string;
  usage?: TokenUsage;
  time: number;
  // Worker fields
  workerThreadId?: string;
  workerAgent?: string;
  workerTask?: string;
  workerStatus?: 'running' | 'completed' | 'stopped';
}

// ── SSE Events ──

export type SSEEventType =
  | 'turn.started'
  | 'turn.completed'
  | 'item.started'
  | 'item.delta'
  | 'item.completed'
  | 'item.failed'
  | 'error';

export interface SSEItem {
  id: string;
  turn_id: string;
  kind: string;  // agent_message | tool_call | command_execution | error
  status: string; // in_progress | completed | failed | interrupted
  summary?: string;
  detail?: string;
  metadata?: Record<string, any>;
}

export interface SSETurn {
  id: string;
  thread_id: string;
  status: string;
  input_summary?: string;
  started_at?: string;
  ended_at?: string;
  duration_ms?: number;
  item_ids?: string[];
}

// RuntimeEventRecord matches the Runtime's SSE event envelope.
export interface RuntimeEventRecord {
  schema_version: number;
  seq: number;
  timestamp: string;
  thread_id: string;
  turn_id?: string;
  item_id?: string;
  event: SSEEventType;
  payload: {
    item?: SSEItem;
    turn?: SSETurn;
    delta?: string;
    kind?: string;
    message?: string;
  };
}

// Legacy SSEEvent — kept for Gateway broadcast compatibility.
export interface SSEEvent {
  event: SSEEventType;
  item?: SSEItem;
  delta?: string;
  kind?: string;
  turn?: SSETurn;
  message?: string;
}

// ── Thread ──

export interface Thread {
  id: string;
  title: string;
  agent_id: number;
  model_id: string;
  parent_thread_id: string;
  status: string;
  created_at: string;
  updated_at: string;
}

export interface WorkerStatus {
  id: string;
  title: string;
  status: string;
  agent_name?: string;
}

// ── Agent ──

export interface Agent {
  id: number;
  name: string;
  desc: string;
  system_prompt: string;
  tools: string;  // JSON array
  default_model_id: string;
  mcp_servers: string;  // JSON array
  created_at: string;
  updated_at: string;
}

// ── Model ──

export interface ModelProfile {
  id: string;
  base_url: string;
  api_key: string;
  model: string;
  vision: boolean;
  context_limit: number;
  reasoning_effort?: string;
}

// ── Tool def ──

export interface ToolParam {
  type: string;
  description?: string;
  enum?: string[];
}

export interface ToolDef {
  name: string;
  description: string;
  source: 'builtin' | 'mcp';
  parameters?: {
    properties: Record<string, ToolParam>;
    required: string[];
  };
}

// ── MCP Server ──

export interface MCPServer {
  id: number;
  name: string;
  type: 'stdio' | 'http';
  enabled: boolean;
  command: string;
  args: string;
  env: string;
  url: string;
  headers: string;
  status: 'connected' | 'disconnected' | 'error';
  error: string;
  created_at: string;
  updated_at: string;
}

// ── Knowledge ──

export interface Knowledge {
  id: number;
  title: string;
  content: string;
  source: string;
  created_at: string;
}

// ── Runtime ──

export interface Runtime {
  id: number;
  name: string;
  url: string;
  status: 'active' | 'inactive';
  restrict_workspace: boolean;
  last_heartbeat: string;
  created_at: string;
  updated_at: string;
}

// ── App State ──

export type Page = 'chat' | 'agent' | 'mcp' | 'model' | 'runtime';
export type AppStateName = 'idle' | 'thinking' | 'tool_calls' | 'responding' | 'error';

export interface AppState {
  page: Page;
  state: AppStateName;
  timeline: TimelineItem[];
  liveItem: TimelineItem | null;
  activeThreadId: string | null;
  threads: Thread[];
  activeAgentId: number | null;
  agents: Agent[];
  models: ModelProfile[];
  activeModelId: string;
  hasModels: boolean;
  tools: ToolDef[];
  mcpServers: MCPServer[];
  contextPct: number;
  totalTokens: number;
  pendingImages: string[];
  workerParentId: string | null;
}
