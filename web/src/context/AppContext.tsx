import React, { createContext, useContext, useReducer, useCallback, useRef } from 'react';
import type { AppState, AppStateName, TimelineItem, Thread, Agent, ModelProfile, ToolDef, MCPServer, SSEPayload, TokenUsage } from '../types';

// ── Actions ──

type Action =
  | { type: 'SET_PAGE'; page: import('../types').Page }
  | { type: 'SET_STATE'; state: AppStateName }
  | { type: 'PUSH_TIMELINE_ITEM'; item: TimelineItem }
  | { type: 'PREPEND_TIMELINE_ITEMS'; items: TimelineItem[] }
  | { type: 'UPDATE_TIMELINE_ITEM'; id: string; updates: Partial<TimelineItem> }
  | { type: 'UPDATE_TIMELINE_ITEM_BY_WORKER'; workerThreadId: string; updates: Partial<TimelineItem> }
  | { type: 'SET_LIVE_ITEM'; item: TimelineItem | null }
  | { type: 'SET_HAS_MORE'; hasMore: boolean }
  | { type: 'SET_LOADING_MORE'; loading: boolean }
  | { type: 'SET_ACTIVE_THREAD'; threadId: string | null }
  | { type: 'SET_THREADS'; threads: Thread[] }
  | { type: 'ADD_THREAD'; thread: Thread }
  | { type: 'REMOVE_THREAD'; threadId: string }
  | { type: 'SET_AGENTS'; agents: Agent[] }
  | { type: 'SET_ACTIVE_AGENT'; agentId: number | null }
  | { type: 'SET_MODELS'; models: ModelProfile[] }
  | { type: 'SET_ACTIVE_MODEL'; modelId: string }
  | { type: 'SET_HAS_MODELS'; has: boolean }
  | { type: 'SET_TOOLS'; tools: ToolDef[] }
  | { type: 'SET_MCP_SERVERS'; servers: MCPServer[] }
  | { type: 'SET_CONTEXT_PCT'; pct: number }
  | { type: 'ADD_TOKENS'; n: number }
  | { type: 'LOAD_TIMELINE'; items: TimelineItem[]; hasMore: boolean }
  | { type: 'CLEAR_TIMELINE' }
  | { type: 'PRUNE_TIMELINE' }
  | { type: 'SET_PENDING_IMAGES'; images: string[] }
  | { type: 'ADD_PENDING_IMAGE'; image: string }
  | { type: 'CLEAR_PENDING_IMAGES' }
  | { type: 'VIEW_WORKER'; threadId: string; parentId: string }
  | { type: 'BACK_TO_PARENT' };

let _seq = 0;
function newId() { return `ti${++_seq}_${Date.now()}`; }

function initState(): AppState {
  return {
    page: 'chat',
    state: 'idle',
    timeline: [],
    liveItem: null,
    hasMoreMessages: false,
    loadingMore: false,
    activeThreadId: null,
    threads: [],
    activeAgentId: null,
    agents: [],
    models: [],
    activeModelId: '',
    hasModels: false,
    tools: [],
    mcpServers: [],
    contextPct: 0,
    totalTokens: 0,
    pendingImages: [],
    workerParentId: null,
  };
}

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
    case 'SET_PAGE':
      return { ...state, page: action.page };
    case 'SET_STATE':
      return { ...state, state: action.state };
    case 'PUSH_TIMELINE_ITEM': {
      const item = { ...action.item };
      if (!item.id) item.id = newId();
      item.time = item.time || Date.now();
      const timeline = [...state.timeline, item];
      return { ...state, timeline };
    }
    case 'PREPEND_TIMELINE_ITEMS': {
      if (action.items.length === 0) return state;
      const timeline = [...action.items, ...state.timeline];
      return { ...state, timeline };
    }
    case 'UPDATE_TIMELINE_ITEM':
      return {
        ...state,
        timeline: state.timeline.map(ti =>
          ti.id === action.id ? { ...ti, ...action.updates } : ti
        ),
        liveItem: state.liveItem?.id === action.id
          ? { ...state.liveItem, ...action.updates }
          : state.liveItem,
      };
    case 'UPDATE_TIMELINE_ITEM_BY_WORKER': {
      const wId = action.workerThreadId;
      const found = state.timeline.find(ti => ti.workerThreadId === wId);
      if (found) {
        return {
          ...state,
          timeline: state.timeline.map(ti =>
            ti.workerThreadId === wId ? { ...ti, ...action.updates } : ti
          ),
        };
      }
      // worker.dispatched may arrive before item.completed sets workerThreadId
      // on the original spawn_worker card. Find a pending worker card without an ID.
      let pendingIdx = -1;
      for (let i = state.timeline.length - 1; i >= 0; i--) {
        const ti = state.timeline[i];
        if (ti.type === 'worker' && ti.status === 'pending' && !ti.workerThreadId) {
          pendingIdx = i;
          break;
        }
      }
      if (pendingIdx >= 0) {
        return {
          ...state,
          timeline: state.timeline.map((ti, i) =>
            i === pendingIdx ? { ...ti, ...action.updates, workerThreadId: wId } : ti
          ),
        };
      }
      // Fallback: no matching or pending card, create a placeholder.
      const item: TimelineItem = {
        id: `wk_${wId}`, type: 'worker',
        label: '', content: '',
        workerThreadId: wId, workerStatus: 'running',
        status: 'pending', time: Date.now(),
        ...action.updates,
      };
      if (!item.id) item.id = newId();
      const timeline = [...state.timeline, item];
      return { ...state, timeline };
    }
    case 'SET_LIVE_ITEM':
      return { ...state, liveItem: action.item };
    case 'SET_ACTIVE_THREAD':
      return { ...state, activeThreadId: action.threadId };
    case 'SET_THREADS':
      return { ...state, threads: action.threads };
    case 'ADD_THREAD':
      return { ...state, threads: [action.thread, ...state.threads] };
    case 'REMOVE_THREAD':
      return { ...state, threads: state.threads.filter(t => t.id !== action.threadId) };
    case 'SET_AGENTS':
      return { ...state, agents: action.agents };
    case 'SET_ACTIVE_AGENT': {
      // When agent changes, pick the agent's default model if set.
      const agent = action.agentId ? state.agents.find(a => a.id === action.agentId) : null;
      const modelId = agent?.default_model_id || state.activeModelId;
      return { ...state, activeAgentId: action.agentId, activeModelId: modelId };
    }
    case 'SET_MODELS':
      return { ...state, models: action.models };
    case 'SET_ACTIVE_MODEL':
      return { ...state, activeModelId: action.modelId };
    case 'SET_HAS_MODELS':
      return { ...state, hasModels: action.has };
    case 'SET_TOOLS':
      return { ...state, tools: action.tools };
    case 'SET_MCP_SERVERS':
      return { ...state, mcpServers: action.servers };
    case 'SET_CONTEXT_PCT':
      return { ...state, contextPct: action.pct };
    case 'ADD_TOKENS':
      return { ...state, totalTokens: state.totalTokens + action.n };
    case 'SET_HAS_MORE':
      return { ...state, hasMoreMessages: action.hasMore };
    case 'SET_LOADING_MORE':
      return { ...state, loadingMore: action.loading };
    case 'LOAD_TIMELINE':
      return { ...state, timeline: action.items, liveItem: null, hasMoreMessages: action.hasMore };
    case 'CLEAR_TIMELINE':
      return { ...state, timeline: [], liveItem: null };
    case 'PRUNE_TIMELINE':
      return { ...state, timeline: state.timeline.filter(ti => ti.status === 'done' || ti.status === 'fail'), liveItem: null };
    case 'SET_PENDING_IMAGES':
      return { ...state, pendingImages: action.images };
    case 'ADD_PENDING_IMAGE':
      return { ...state, pendingImages: [...state.pendingImages, action.image] };
    case 'CLEAR_PENDING_IMAGES':
      return { ...state, pendingImages: [] };
    case 'VIEW_WORKER':
      return { ...state, activeThreadId: action.threadId, workerParentId: action.parentId };
    case 'BACK_TO_PARENT':
      return { ...state, activeThreadId: state.workerParentId, workerParentId: null, timeline: [], liveItem: null };
    default:
      return state;
  }
}

// ── SSE Event Processing ──

// processSSEEvent handles a single SSE event from the Gateway.
export function processSSEEvent(
  eventType: string,
  data: SSEPayload,
  dispatch: React.Dispatch<Action>,
  timelineRef: React.MutableRefObject<TimelineItem[]>,
  contentAccRef: React.MutableRefObject<Record<string, string>>,
  thinkingAccRef: React.MutableRefObject<Record<string, string>>,
  turnIdRef: React.MutableRefObject<string>,
): boolean {
  const turnId = data.payload?.turn_id || '';

  // turn.started always processes to track the new turn ID.
  if (eventType === 'turn.started') {
    const turn = data.payload?.turn;
    turnIdRef.current = turn?.id || turnId;
    dispatch({ type: 'PRUNE_TIMELINE' });
    dispatch({ type: 'SET_STATE', state: 'thinking' });
    return false;
  }

  // Ignore events from stale turns only once we've seen a turn.started.
  if (turnId && turnIdRef.current && turnId !== turnIdRef.current) return false;

  switch (eventType) {
    case 'turn.completed': {
      dispatch({ type: 'SET_STATE', state: 'idle' });
      // Extract usage from payload
      const usageStr = data.payload?.usage as string | undefined;
      if (usageStr) {
        try {
          const u = JSON.parse(usageStr) as TokenUsage;
          if (u.total) dispatch({ type: 'ADD_TOKENS', n: u.total });
          // Estimate context percentage (rough)
          // Can't know exact context limit here, use 128000 as default
          const pct = Math.round((u.prompt / 128000) * 100);
          if (pct > 0) dispatch({ type: 'SET_CONTEXT_PCT', pct: Math.min(pct, 100) });
        } catch {}
      }
      return true;
    }

    case 'item.started': {
      const item = data.payload?.item;
      if (!item) break;

      switch (item.kind) {
        case 'user_message':
          // Skip if we already added this optimistically (or if it's the same content as the last user msg)
          {
            const existing = timelineRef.current.find(ti =>
              ti.type === 'user' && ti.content === (item.detail || item.summary || '') &&
              Date.now() - ti.time < 10000 // within 10 seconds
            );
            if (existing) break;
          }
          dispatch({
            type: 'PUSH_TIMELINE_ITEM', item: {
              id: item.id, type: 'user', label: 'You',
              content: item.detail || item.summary || '',
              status: 'done', time: Date.now(),
            },
          });
          break;

        case 'agent_message':
          dispatch({ type: 'SET_STATE', state: 'thinking' });
          dispatch({
            type: 'PUSH_TIMELINE_ITEM', item: {
              id: item.id, type: 'agent', label: 'AI',
              content: '', status: 'streaming', time: Date.now(),
            },
          });
          break;

        case 'tool_call': {
          dispatch({ type: 'SET_STATE', state: 'tool_calls' });
          const meta = item.metadata || {};
          const rawArgs = meta.arguments || '';
          const toolName = meta.tool_name || 'Tool';
          // Worker spawn: render as a worker card instead of a regular tool card.
          if (toolName === 'spawn_worker') {
            let agent = '';
            let task = '';
            try { const args = JSON.parse(rawArgs); agent = args.agent || ''; task = args.task || ''; } catch {}
            dispatch({
              type: 'PUSH_TIMELINE_ITEM', item: {
                id: item.id, type: 'worker',
                label: agent || 'Worker',
                content: task,
                args: rawArgs,
                status: 'pending', toolName: 'spawn_worker', toolCallId: meta.tool_use_id,
                workerAgent: agent, workerTask: task, workerStatus: 'running',
                time: Date.now(),
              },
            });
          } else {
            dispatch({
              type: 'PUSH_TIMELINE_ITEM', item: {
                id: item.id, type: 'tool_call',
                label: toolName,
                content: '',
                args: rawArgs,
                status: 'pending', toolName, toolCallId: meta.tool_use_id, time: Date.now(),
              },
            });
          }
          break;
        }

        case 'command_execution': {
          const meta = item.metadata || {};
          const cmd = meta.command || item.summary || 'run';
          dispatch({
            type: 'PUSH_TIMELINE_ITEM', item: {
              id: item.id, type: 'tool_call',
              label: cmd, content: cmd,
              status: 'streaming', toolName: 'run_command', time: Date.now(),
            },
          });
          break;
        }
      }
      break;
    }

    case 'item.delta': {
      dispatch({ type: 'SET_STATE', state: 'responding' });
      const kind = data.payload?.kind || '';
      const delta = data.payload?.delta || '';
      const itemId = data.payload?.item_id;
      if (!itemId) break;

      if (kind === 'thinking') {
        const acc = thinkingAccRef.current;
        acc[itemId] = (acc[itemId] || '') + delta;
        dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: itemId, updates: { thinking: acc[itemId] } });
      } else {
        const acc = contentAccRef.current;
        acc[itemId] = (acc[itemId] || '') + delta;
        dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: itemId, updates: { content: acc[itemId] } });
      }
      break;
    }

    case 'item.completed': {
      dispatch({ type: 'SET_STATE', state: 'idle' });
      const item = data.payload?.item;
      if (!item) break;

      if (item.kind === 'tool_call') {
        const meta = item.metadata || {};
        const toolName = meta.tool_name || '';
        let output = item.detail || '';
        try {
          const parsed = JSON.parse(output);
          output = parsed.content || parsed.error || output;
        } catch {}
        const toolCallId = meta.tool_use_id;
        // Worker dispatch: extract thread ID from result.
        if (toolName === 'spawn_worker') {
          const m = output.match(/thread (\S+)\)/);
          const workerThreadId = m ? m[1] : '';
          dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: item.id, updates: { content: output, status: 'done', workerThreadId } });
        } else {
          dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: item.id, updates: { content: output, status: 'done' } });
        }
        if (toolCallId) {
          const t = timelineRef.current;
          for (let i = t.length - 1; i >= 0; i--) {
            if (t[i].toolCallId === toolCallId && t[i].status === 'pending' && t[i].id !== item.id) {
              dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: t[i].id, updates: { content: output, status: 'done' } });
              break;
            }
          }
        }
      } else if (item.kind === 'agent_message') {
        const meta = item.metadata || {};
        const usage = meta.usage as TokenUsage | undefined;
        dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: item.id, updates: { status: 'done', usage } });
      } else if (item.kind === 'command_execution') {
        const meta = item.metadata || {};
        const cmd = meta.command || item.summary || 'run';
        const exitCode = meta.exit_code ?? 0;
        const label = exitCode === 0 ? cmd : `${cmd} (exit ${exitCode})`;
        dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: item.id, updates: { status: 'done', label } });
      }
      break;
    }

    case 'item.failed': {
      dispatch({ type: 'SET_STATE', state: 'error' });
      const item = data.payload?.item;
      // LLM errors send detail directly in payload, not nested in item
      const detail = item?.detail || data.payload?.detail || data.payload?.message || 'An error occurred';
      dispatch({
        type: 'PUSH_TIMELINE_ITEM', item: {
          id: '', type: 'error', label: 'Error',
          content: detail,
          status: 'fail', time: Date.now(),
        },
      });
      break;
    }

    case 'error':
      dispatch({ type: 'SET_STATE', state: 'error' });
      dispatch({
        type: 'PUSH_TIMELINE_ITEM', item: {
          id: '', type: 'error', label: 'Error',
          content: (data as any).message || 'An error occurred',
          status: 'fail', time: Date.now(),
        },
      });
      break;
  }
  return false;
}

// ── Context ──

interface AppContextValue {
  state: AppState;
  dispatch: React.Dispatch<Action>;
  getActiveThreadId: () => string | null;
}

const AppCtx = createContext<AppContextValue | null>(null);

export function AppProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(reducer, undefined, initState);
  const stateRef = useRef(state);
  stateRef.current = state;

  const getActiveThreadId = useCallback(() => stateRef.current.activeThreadId, []);

  return (
    <AppCtx.Provider value={{ state, dispatch, getActiveThreadId }}>
      {children}
    </AppCtx.Provider>
  );
}

export function useAppState() {
  const ctx = useContext(AppCtx);
  if (!ctx) throw new Error('useAppState must be used within AppProvider');
  return ctx;
}

export { newId };

// ── Messages → Timeline ──

export interface APIMessage {
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

function parseUsage(raw: string | undefined): import('../types').TokenUsage | undefined {
  if (!raw) return undefined;
  try { return JSON.parse(raw); } catch { return undefined; }
}

// messagesToTimeline converts DB messages to timeline items for initial load.
// tool_result rows update their matching tool_call item instead of creating separate entries.
export function messagesToTimeline(messages: APIMessage[]): TimelineItem[] {
  const items: TimelineItem[] = [];
  for (const m of messages) {
    const time = new Date(m.created_at).getTime() || Date.now();
    switch (m.kind) {
      case 'user_message':
        items.push({
          id: `msg${m.id}`, type: 'user', label: 'You',
          content: m.content, status: 'done', time,
        });
        break;
      case 'agent_message':
        items.push({
          id: `msg${m.id}`, type: 'agent', label: 'AI',
          content: m.content,
          thinking: m.reasoning_content || undefined,
          usage: parseUsage(m.usage),
          status: 'done', time,
        });
        break;
      case 'tool_call': {
        // If the message has text content, push an agent item first.
        if (m.content) {
          items.push({
            id: `msg${m.id}`, type: 'agent', label: 'AI',
            content: m.content,
            thinking: m.reasoning_content || undefined,
            usage: parseUsage(m.usage),
            status: 'done', time,
          });
        }
        let tcs: Array<{ id?: string; function?: { name?: string; arguments?: string } }> = [];
        try { tcs = JSON.parse(m.tool_calls || '[]'); } catch {}
        for (const tc of tcs) {
          const toolName = tc.function?.name || 'Tool';
          const args = tc.function?.arguments || '';
          items.push({
            id: `tc${m.id}_${tc.id || ''}`, type: 'tool_call',
            label: toolName,
            content: '',
            args,
            toolName, toolCallId: tc.id,
            status: 'done', time,
          });
        }
        break;
      }
      case 'tool_result': {
        let output = m.content || '';
        try {
          const parsed = JSON.parse(output);
          output = parsed.content || parsed.error || output;
        } catch {}
        // Merge into matching tool_call item above
        const tcId = m.tool_call_id;
        let found = false;
        for (let i = items.length - 1; i >= 0; i--) {
          if (items[i].toolCallId === tcId) {
            items[i] = { ...items[i], content: output, status: 'done' };
            found = true;
            break;
          }
        }
        if (!found) {
          items.push({
            id: `tr${m.id}`, type: 'tool_call',
            label: 'Tool', content: output,
            toolCallId: tcId, status: 'done', time,
          });
        }
        break;
      }
      case 'error':
        items.push({
          id: `err${m.id}`, type: 'error', label: 'Error',
          content: m.content, status: 'fail', time,
        });
        break;
    }
  }
  return items;
}
