// ── Timeline / Message ──

export type TimelineItemType = 'user' | 'agent' | 'tool_call' | 'tool_result' | 'error' | 'notice';

export interface TimelineItem {
  id: string;
  type: TimelineItemType;
  icon: string;
  label: string;
  content: string;
  html?: string;
  status: 'pending' | 'streaming' | 'done' | 'fail';
  threadId?: string;
  toolCallId?: string;
  toolName?: string;
  time: number;
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

export interface SSEEvent {
  event: SSEEventType;
  // item.started / item.completed / item.failed
  item?: SSEItem;
  // item.delta
  delta?: string;
  kind?: string;
  // turn.started / turn.completed
  turn?: SSETurn;
  // error
  message?: string;
}

// ── Thread ──

export interface Thread {
  id: string;
  title: string;
  agent_id: number;
  model_id: string;
  created_at: string;
  updated_at: string;
}

// ── Agent ──

export interface Agent {
  id: number;
  name: string;
  desc: string;
  system_prompt: string;
  tools: string;  // JSON array
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

// ── Settings ──

export interface Settings {
  llm: {
    models: ModelProfile[];
    active: string;
  };
  mcp_servers: MCPServer[];
  agents: Agent[];
}

// ── App State ──

export type AppStateName = 'idle' | 'thinking' | 'tool_calls' | 'responding' | 'error';

export interface AppState {
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
  historyOpen: boolean;
  settingsOpen: boolean;
  pendingImages: string[];
}
