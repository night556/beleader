// ── Timeline / Message ──

export interface TokenUsage {
  prompt: number;
  completion: number;
  total: number;
  cached: number;
}

export type TimelineItemType = 'user' | 'agent' | 'tool_call' | 'error' | 'worker' | 'system';

export interface TimelineItem {
  id: string;
  type: TimelineItemType;
  turnId?: string;
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
  | 'error'
  | 'context.compressed'
  | 'worker.dispatched'
  | 'worker.completed';

export interface SSEItem {
  id: string;
  turn_id: string;
  kind: string;
  status: string;
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

export interface SSEPayload {
  thread_id: string;
  turn_id: string;
  item_id: string;
  event_id?: number;
  item?: SSEItem;
  turn?: SSETurn;
  delta?: string;
  kind?: string;
  message?: string;
  status?: string;
  usage?: string;
}

// ── Thread ──

export interface Thread {
  id: string;
  title: string;
  agent_id: number;
  model_id: string;
  pool_id: number;
  workspace_path: string;
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
  tools: string;
  default_model_id: string;
  mcp_servers: string;
  worker_agents: string;
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
  source: string;
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
  pool_id: number;
  created_at: string;
  updated_at: string;
}

// ── Pool ──

export interface Pool {
  id: number;
  name: string;
  shell: string;
  platform: string;
  go_version: string;
  workspace_root: string;
  restrict_workspace: boolean;
  tool_defs: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

// ── ToolAgent ──

export interface ToolAgent {
  id: number;
  name: string;
  url: string;
  pool_id: number;
  status: 'active' | 'inactive';
  last_heartbeat: string;
  created_at: string;
  updated_at: string;
}

// ── App State ──

export type Page = 'chat' | 'agent' | 'mcp' | 'model' | 'pool';
export type AppStateName = 'idle' | 'thinking' | 'tool_calls' | 'responding' | 'error';

export interface AppState {
  page: Page;
  state: AppStateName;
  timeline: TimelineItem[];
  liveItem: TimelineItem | null;
  hasMoreMessages: boolean;
  loadingMore: boolean;
  activeThreadId: string | null;
  threads: Thread[];
  activeAgentId: number | null;
  agents: Agent[];
  models: ModelProfile[];
  activeModelId: string;
  hasModels: boolean;
  pools: Pool[];
  activePoolId: number;
  tools: ToolDef[];
  mcpServers: MCPServer[];
  contextPct: number;
  totalTokens: number;
  pendingImages: string[];
  workerParentId: string | null;
}
