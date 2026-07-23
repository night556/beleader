import type { Thread, Agent, ModelProfile, ToolDef, MCPServer, Pool, ToolAgent } from '../types';

const SERVER_URL = window.location.origin;

async function api<T>(path: string, opts?: RequestInit): Promise<T> {
  const r = await fetch(`${SERVER_URL}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...opts,
  });
  if (!r.ok) {
    const err = await r.json().catch(() => ({ error: r.statusText }));
    throw new Error((err as { error?: string }).error || r.statusText);
  }
  return r.json();
}

export const client = {
  // Threads
  listThreads: () => api<Thread[]>('/api/threads'),
  getThread: (id: string) => api<Thread>(`/api/threads/${encodeURIComponent(id)}`),
  deleteThread: (id: string) => api<{ status: string }>(`/api/threads/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  getMessages: (threadId: string, afterId = 0, limit = 100) => api<{ messages: Message[]; oldest_id: number; has_more: boolean }>(`/api/threads/${encodeURIComponent(threadId)}/messages?after_id=${afterId}&limit=${limit}`),
  pauseThread: (id: string) => api<{ status: string }>(`/api/threads/${encodeURIComponent(id)}/pause`, { method: 'POST' }),
  resumeThread: (id: string) => api<{ status: string }>(`/api/threads/${encodeURIComponent(id)}/resume`, { method: 'POST' }),
  getWorkers: (threadId: string) => api<{ workers: Array<{ id: string; title: string; status: string }> }>(`/api/threads/${encodeURIComponent(threadId)}/workers`),
  stopWorker: (threadId: string, workerId: string) => api<{ status: string }>(`/api/threads/${encodeURIComponent(threadId)}/workers/${encodeURIComponent(workerId)}/stop`, { method: 'POST' }),

  // Chat
  sendChat: (body: { message: string; images: string[]; agent_id: number; thread_id?: string; model_id?: string; reasoning_effort?: string }, signal?: AbortSignal) =>
    api<{ thread_id: string; status: string }>('/api/chat', { method: 'POST', body: JSON.stringify(body), signal }),

  // Agents
  listAgents: () => api<Agent[]>('/api/agents'),
  createAgent: (body: { name: string; desc: string; system_prompt: string; tools: string; default_model_id?: string; mcp_servers?: string; worker_agents?: string }) => api<Agent>('/api/agents', { method: 'POST', body: JSON.stringify(body) }),
  updateAgent: (id: number, body: { name: string; desc: string; system_prompt: string; tools: string; default_model_id?: string; mcp_servers?: string; worker_agents?: string }) => api<Agent>(`/api/agents/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteAgent: (id: number) => api<{ status: string }>(`/api/agents/${id}`, { method: 'DELETE' }),

  // Tools
  listTools: () => api<ToolDef[]>('/api/tools'),

  // Models
  listModels: () => api<ModelProfile[]>('/api/models'),
  createModel: (body: ModelProfile) => api<ModelProfile>('/api/models', { method: 'POST', body: JSON.stringify(body) }),
  updateModel: (_id: string, body: ModelProfile) => api<{ status: string }>('/api/models', { method: 'PUT', body: JSON.stringify(body) }),
  deleteModel: (id: string) => api<{ status: string }>('/api/models', { method: 'DELETE', body: JSON.stringify({ id }) }),

  // MCP
  listMCPServers: () => api<MCPServer[]>('/api/mcp/servers'),
  createMCPServer: (body: Record<string, unknown>) => api<MCPServer>('/api/mcp/servers', { method: 'POST', body: JSON.stringify(body) }),
  updateMCPServer: (id: number, body: Record<string, unknown>) => api<MCPServer>(`/api/mcp/servers/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteMCPServer: (id: number) => api<{ status: string }>(`/api/mcp/servers/${id}`, { method: 'DELETE' }),
  testMCPServer: (id: number) => api<{ success: boolean; tool_count: number; tools: string[]; error?: string }>(`/api/mcp/servers/${id}/test`, { method: 'POST' }),

  // Pools
  listPools: () => api<Pool[]>('/api/pools'),
  createPool: (body: Partial<Pool>) => api<Pool>('/api/pools', { method: 'POST', body: JSON.stringify(body) }),
  updatePool: (id: number, body: Partial<Pool>) => api<Pool>(`/api/pools/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deletePool: (id: number) => api<{ status: string }>(`/api/pools/${id}`, { method: 'DELETE' }),

  // Tool Agents
  listToolAgents: () => api<ToolAgent[]>('/api/tool-agents'),
  deleteToolAgent: (id: number) => api<{ status: string }>(`/api/tool-agents/${id}`, { method: 'DELETE' }),
};

export interface Message {
  id: number;
  thread_id: string;
  kind: string;
  content: string;
  multi_content: string;
  tool_calls: string;
  tool_call_id: string;
  reasoning_content: string;
  usage?: string;
  created_at: string;
}
