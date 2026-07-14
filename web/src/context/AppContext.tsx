import React, { createContext, useContext, useReducer, useCallback, useRef } from 'react';
import type { AppState, AppStateName, TimelineItem, Thread, Agent, ModelProfile, ToolDef, MCPServer, SSEEvent } from '../types';

// ── Actions ──

type Action =
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
  | { type: 'CLEAR_TIMELINE' }
  | { type: 'SET_PENDING_IMAGES'; images: string[] }
  | { type: 'ADD_PENDING_IMAGE'; image: string }
  | { type: 'CLEAR_PENDING_IMAGES' };

let _seq = 0;
function newId() { return `ti${++_seq}_${Date.now()}`; }

function initState(): AppState {
  return {
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
    historyOpen: false,
    settingsOpen: false,
    pendingImages: [],
  };
}

function reducer(state: AppState, action: Action): AppState {
  switch (action.type) {
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
    case 'SET_ACTIVE_AGENT':
      return { ...state, activeAgentId: action.agentId };
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

// ── SSE Hook ──

interface SSEConnection {
  es: EventSource;
  close: () => void;
}

export function createSSEConnection(
  dispatch: React.Dispatch<Action>,
  _getActiveThreadId: () => string | null
): SSEConnection {
  const es = new EventSource('/api/sse');

  // ── Turn events ──

  es.addEventListener('turn.started', () => {
    dispatch({ type: 'SET_STATE', state: 'thinking' });
  });

  es.addEventListener('turn.completed', () => {
    dispatch({ type: 'SET_STATE', state: 'idle' });
  });

  // ── Item started ──

  es.addEventListener('item.started', (e: MessageEvent) => {
    const d: SSEEvent = JSON.parse(e.data);
    const item = d.item;
    if (!item) return;

    switch (item.kind) {
      case 'agent_message':
        dispatch({ type: 'SET_STATE', state: 'thinking' });
        dispatch({
          type: 'PUSH_TIMELINE_ITEM', item: {
            id: item.id, icon: '◆', type: 'agent', label: 'AI',
            content: '', status: 'streaming', time: Date.now(),
          },
        });
        break;

      case 'tool_call': {
        dispatch({ type: 'SET_STATE', state: 'tool_calls' });
        const meta = item.metadata || {};
        dispatch({
          type: 'PUSH_TIMELINE_ITEM', item: {
            id: item.id, icon: '🔧', type: 'tool_call',
            label: meta.tool_name || 'Tool', content: '',
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
            id: item.id, icon: '>_', type: 'tool_call',
            label: cmd, content: cmd,
            status: 'streaming', toolName: 'run_command', time: Date.now(),
          },
        });
        break;
      }
    }
  });

  // ── Item delta ──

  es.addEventListener('item.delta', (_e: MessageEvent) => {
    JSON.parse(_e.data);
    dispatch({ type: 'SET_STATE', state: 'responding' });
    // Delta appended via ref in App.tsx
  });

  // ── Item completed ──

  es.addEventListener('item.completed', (e: MessageEvent) => {
    const d: SSEEvent = JSON.parse(e.data);
    const item = d.item;
    if (!item) return;

    dispatch({ type: 'SET_STATE', state: 'idle' });

    if (item.kind === 'tool_call') {
      const meta = item.metadata || {};
      let output = item.detail || '';
      try {
        const parsed = JSON.parse(output);
        output = parsed.content || parsed.error || output;
      } catch { /* not JSON, use raw detail */ }
      dispatch({
        type: 'PUSH_TIMELINE_ITEM', item: {
          id: '', icon: '📋', type: 'tool_result',
          label: meta.tool_name || 'Tool', content: output,
          status: 'done', toolName: meta.tool_name, toolCallId: meta.tool_use_id, time: Date.now(),
        },
      });
    }
    // agent_message & command_execution finalized via ref in App.tsx
  });

  // ── Item failed ──

  es.addEventListener('item.failed', (e: MessageEvent) => {
    const d: SSEEvent = JSON.parse(e.data);
    dispatch({ type: 'SET_STATE', state: 'error' });
    dispatch({
      type: 'PUSH_TIMELINE_ITEM', item: {
        id: '', icon: '⚠', type: 'error', label: 'Error',
        content: d.item?.detail || d.message || 'An error occurred',
        status: 'fail', time: Date.now(),
      },
    });
  });

  // ── Error (connection-level) ──

  es.addEventListener('error', (e: MessageEvent) => {
    dispatch({ type: 'SET_STATE', state: 'error' });
    let msg = 'An error occurred';
    try {
      const d: SSEEvent = JSON.parse(e.data);
      if (d.message) msg = d.message;
    } catch { /* native EventSource error (connection loss) — use default */ }
    dispatch({
      type: 'PUSH_TIMELINE_ITEM', item: {
        id: '', icon: '⚠', type: 'error', label: 'Error',
        content: msg, status: 'fail', time: Date.now(),
      },
    });
  });

  es.onerror = () => {
    // EventSource will auto-reconnect
  };

  return {
    es,
    close: () => es.close(),
  };
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
