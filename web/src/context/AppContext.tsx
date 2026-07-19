import React, { createContext, useContext, useReducer, useCallback, useRef } from 'react';
import type { AppState, AppStateName, TimelineItem, Thread, Agent, ModelProfile, ToolDef, MCPServer, RuntimeEventRecord } from '../types';

// ── Actions ──

type Action =
  | { type: 'SET_PAGE'; page: import('../types').Page }
  | { type: 'SET_STATE'; state: AppStateName }
  | { type: 'PUSH_TIMELINE_ITEM'; item: TimelineItem }
  | { type: 'UPDATE_TIMELINE_ITEM'; id: string; updates: Partial<TimelineItem> }
  | { type: 'SET_LIVE_ITEM'; item: TimelineItem | null }
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
  | { type: 'LOAD_TIMELINE'; items: TimelineItem[] }
  | { type: 'CLEAR_TIMELINE' }
  | { type: 'SET_PENDING_IMAGES'; images: string[] }
  | { type: 'ADD_PENDING_IMAGE'; image: string }
  | { type: 'CLEAR_PENDING_IMAGES' };

let _seq = 0;
function newId() { return `ti${++_seq}_${Date.now()}`; }

function formatToolArgs(name: string, args: string): string {
  try {
    const obj = JSON.parse(args);
    const keys = Object.keys(obj);
    if (keys.length === 0) return `Running ${name}...`;
    const preview = keys.map(k => `${k}=${JSON.stringify(obj[k])}`).join(', ');
    return preview.length > 120 ? preview.slice(0, 117) + '...' : preview;
  } catch {
    return args || `Running ${name}...`;
  }
}

function initState(): AppState {
  return {
    page: 'chat',
    state: 'idle',
    timeline: [],
    liveItem: null,
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
      if (timeline.length > 200) timeline.shift();
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
    case 'LOAD_TIMELINE':
      return { ...state, timeline: action.items.slice(-200), liveItem: null };
    case 'CLEAR_TIMELINE':
      return { ...state, timeline: [], liveItem: null };
    case 'SET_PENDING_IMAGES':
      return { ...state, pendingImages: action.images };
    case 'ADD_PENDING_IMAGE':
      return { ...state, pendingImages: [...state.pendingImages, action.image] };
    case 'CLEAR_PENDING_IMAGES':
      return { ...state, pendingImages: [] };
    default:
      return state;
  }
}

// ── SSE Event Processing ──

// processSSEEvent handles a single SSE event from the Gateway.
export function processSSEEvent(
  eventType: string,
  data: RuntimeEventRecord,
  dispatch: React.Dispatch<Action>,
  timelineRef: React.MutableRefObject<TimelineItem[]>,
  contentAccRef: React.MutableRefObject<Record<string, string>>,
  thinkingAccRef: React.MutableRefObject<Record<string, string>>,
  turnIdRef: React.MutableRefObject<string>,
): boolean {
  const turnId = data.turn_id || '';

  // turn.started always processes to track the new turn ID.
  if (eventType === 'turn.started') {
    const turn = (data.payload as any)?.turn;
    turnIdRef.current = turn?.id || turnId;
    dispatch({ type: 'CLEAR_TIMELINE' });
    dispatch({ type: 'SET_STATE', state: 'thinking' });
    return false;
  }

  // Ignore events from stale turns (old SSE stream winding down).
  if (turnId && turnId !== turnIdRef.current) return false;

  switch (eventType) {
    case 'turn.completed':
      dispatch({ type: 'SET_STATE', state: 'idle' });
      return true;

    case 'item.started': {
      const item = data.payload.item;
      if (!item) break;

      switch (item.kind) {
        case 'user_message':
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
          const args = meta.arguments || '';
          dispatch({
            type: 'PUSH_TIMELINE_ITEM', item: {
              id: item.id, type: 'tool_call',
              label: meta.tool_name || 'Tool', content: args ? formatToolArgs(meta.tool_name, args) : `Running ${meta.tool_name || 'tool'}...`,
              status: 'pending', toolName: meta.tool_name, toolCallId: meta.tool_use_id, time: Date.now(),
            },
          });
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
      const kind = data.payload.kind || '';
      const delta = data.payload.delta || '';
      const itemId = data.item_id;
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
      const item = data.payload.item;
      if (!item) break;

      if (item.kind === 'tool_call') {
        const meta = item.metadata || {};
        let output = item.detail || '';
        try {
          const parsed = JSON.parse(output);
          output = parsed.content || parsed.error || output;
        } catch {}
        const toolCallId = meta.tool_use_id;
        dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: item.id, updates: { content: output, status: 'done' } });
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
        dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: item.id, updates: { status: 'done' } });
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
      const item = data.payload.item;
      dispatch({
        type: 'PUSH_TIMELINE_ITEM', item: {
          id: '', type: 'error', label: 'Error',
          content: item?.detail || data.payload.message || 'An error occurred',
          status: 'fail', time: Date.now(),
        },
      });
      break;
    }
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
  created_at: string;
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
          status: 'done', time,
        });
        break;
      case 'tool_call': {
        let tcs: Array<{ id?: string; function?: { name?: string; arguments?: string } }> = [];
        try { tcs = JSON.parse(m.tool_calls || '[]'); } catch {}
        for (const tc of tcs) {
          const toolName = tc.function?.name || 'Tool';
          const args = tc.function?.arguments || '';
          items.push({
            id: `tc${m.id}_${tc.id || ''}`, type: 'tool_call',
            label: toolName,
            content: formatToolArgs(toolName, args),
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
