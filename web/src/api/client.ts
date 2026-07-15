import type { Thread, Agent, ModelProfile, ToolDef, MCPServer, Knowledge, Settings } from '../types';

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
  getMessages: (threadId: string, afterId = 0) => api<{ messages: Message[]; latest_seq: number }>(`/api/threads/${encodeURIComponent(threadId)}/messages?after_id=${afterId}`),
  pauseThread: (id: string) => api<{ status: string }>(`/api/threads/${encodeURIComponent(id)}/pause`, { method: 'POST' }),
  resumeThread: (id: string) => api<{ status: string }>(`/api/threads/${encodeURIComponent(id)}/resume`, { method: 'POST' }),

  // Chat
  // Returns raw Response for SSE streaming. Thread ID via X-Thread-Id header.
  sendChat: (body: { message: string; images: string[]; agent_id: number; thread_id?: string }, signal?: AbortSignal) =>
    fetch(`${SERVER_URL}/api/chat`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
      signal,
    }),

  // Agents
  listAgents: () => api<Agent[]>('/api/agents'),
  createAgent: (body: { name: string; desc: string; system_prompt: string; tools: string }) => api<Agent>('/api/agents', { method: 'POST', body: JSON.stringify(body) }),
  updateAgent: (id: number, body: { name: string; desc: string; system_prompt: string; tools: string }) => api<Agent>(`/api/agents/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteAgent: (id: number) => api<{ status: string }>(`/api/agents/${id}`, { method: 'DELETE' }),

  // Tools
  listTools: () => api<ToolDef[]>('/api/tools'),

  // Settings
  getSettings: () => api<Settings>('/api/settings'),
  updateSettings: (body: { llm: { models: ModelProfile[]; active: string } }) => api<{ status: string }>('/api/settings', { method: 'PUT', body: JSON.stringify(body) }),

  // MCP
  listMCPServers: () => api<MCPServer[]>('/api/mcp/servers'),
  createMCPServer: (body: Record<string, unknown>) => api<MCPServer>('/api/mcp/servers', { method: 'POST', body: JSON.stringify(body) }),
  updateMCPServer: (id: number, body: Record<string, unknown>) => api<MCPServer>(`/api/mcp/servers/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteMCPServer: (id: number) => api<{ status: string }>(`/api/mcp/servers/${id}`, { method: 'DELETE' }),
  testMCPServer: (id: number) => api<{ success: boolean; tool_count: number; tools: string[]; error?: string }>(`/api/mcp/servers/${id}/test`, { method: 'POST' }),
  connectMCPServer: (id: number) => api<{ status: string }>(`/api/mcp/servers/${id}/connect`, { method: 'POST' }),
  disconnectMCPServer: (id: number) => api<{ status: string }>(`/api/mcp/servers/${id}/disconnect`, { method: 'POST' }),

  // Knowledge
  listKnowledge: (limit = 20, offset = 0) => api<Knowledge[]>(`/api/knowledge?limit=${limit}&offset=${offset}`),
  searchKnowledge: (q: string) => api<Knowledge[]>(`/api/knowledge/search?q=${encodeURIComponent(q)}`),
  updateKnowledge: (id: number, body: { title?: string; content?: string }) => api<Knowledge>(`/api/knowledge/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
  deleteKnowledge: (id: number) => api<{ status: string }>(`/api/knowledge/${id}`, { method: 'DELETE' }),

  // Bookmarks
  getBookmarks: (threadId: string) => api<Message[]>(`/api/messages/bookmarked?thread_id=${encodeURIComponent(threadId)}`),
  toggleBookmark: (msgId: number, bookmarked: boolean) => api<{ status: string }>(`/api/messages/${msgId}/bookmark`, { method: 'PUT', body: JSON.stringify({ bookmarked }) }),
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
  created_at: string;
  bookmarked?: boolean;
}
