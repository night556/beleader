import { useEffect, useRef, useState } from 'react';
import { AppProvider, useAppState, createSSEConnection } from '../context/AppContext';
import { client } from '../api/client';
import { Sidebar } from './Sidebar';
import { Stage } from './Stage';
import { InputArea } from './InputArea';
import { ContextBar } from './ContextBar';
import { SettingsPanel } from './panels/SettingsPanel';
import { AgentsPanel } from './panels/AgentsPanel';
import { ToolsPanel } from './panels/ToolsPanel';
import { MCPPanel } from './panels/MCPPanel';
import { KnowledgePanel } from './panels/KnowledgePanel';
import { BookmarksPanel } from './panels/BookmarksPanel';
import { Toaster } from './Toaster';
import type { TimelineItem, SSEEvent } from '../types';

function AppInner() {
  const { state, dispatch, getActiveThreadId } = useAppState();

  const sseRef = useRef<ReturnType<typeof createSSEConnection> | null>(null);

  // Startup: load initial data
  useEffect(() => {
    Promise.all([
      client.listThreads(),
      client.listAgents(),
      client.getSettings(),
      client.listTools(),
    ]).then(([threads, agents, settings, tools]) => {
      dispatch({ type: 'SET_THREADS', threads });
      dispatch({ type: 'SET_AGENTS', agents });
      dispatch({ type: 'SET_TOOLS', tools });
      const models = settings.llm?.models || [];
      dispatch({ type: 'SET_MODELS', models });
      dispatch({ type: 'SET_HAS_MODELS', has: models.length > 0 });
      const active = settings.llm?.active || (models.length > 0 ? models[0].id : '');
      dispatch({ type: 'SET_ACTIVE_MODEL', modelId: active });
      // Pick default or first agent
      const defaultAgent = agents.find(a => a.name === 'Default') || agents[0];
      if (defaultAgent) {
        dispatch({ type: 'SET_ACTIVE_AGENT', agentId: defaultAgent.id });
      }
    }).catch(err => console.error('startup error:', err));

    // Start SSE
    sseRef.current = createSSEConnection(dispatch, getActiveThreadId);
    return () => { sseRef.current?.close(); };
  }, []);

  // SSE delta tracking: keep ref to last timeline item
  const timelineRef = useRef(state.timeline);
  timelineRef.current = state.timeline;

  useEffect(() => {
    const es = sseRef.current?.es;
    if (!es) return;

    const onDelta = (e: MessageEvent) => {
      const d: SSEEvent = JSON.parse(e.data);
      const t = timelineRef.current;
      if (t.length === 0) return;

      if (d.kind === 'command_execution') {
        for (let i = t.length - 1; i >= 0; i--) {
          if (t[i].status === 'streaming' && t[i].toolName === 'run_command') {
            dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: t[i].id, updates: { content: t[i].content + (d.delta || '') } });
            break;
          }
        }
      } else {
        const last = t[t.length - 1];
        if (last && last.status === 'streaming' && last.toolName !== 'run_command') {
          dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: last.id, updates: { content: last.content + (d.delta || '') } });
        }
      }
    };

    const onItemCompleted = (e: MessageEvent) => {
      const d: SSEEvent = JSON.parse(e.data);
      const item = d.item;
      if (!item) return;
      const t = timelineRef.current;

      if (item.kind === 'agent_message') {
        for (let i = t.length - 1; i >= 0; i--) {
          if (t[i].status === 'streaming' && t[i].toolName !== 'run_command') {
            const updates: Partial<TimelineItem> = { status: 'done' };
            // Only use detail if deltas didn't build content (edge case).
            if (!t[i].content && item.detail) updates.content = item.detail;
            dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: t[i].id, updates });
            break;
          }
        }
      } else if (item.kind === 'command_execution') {
        for (let i = t.length - 1; i >= 0; i--) {
          if (t[i].status === 'streaming' && t[i].toolName === 'run_command') {
            const meta = item.metadata || {};
            const cmd = meta.command || item.summary || 'run';
            const exitCode = meta.exit_code ?? 0;
            const label = exitCode === 0 ? cmd : `${cmd} (exit ${exitCode})`;
            dispatch({ type: 'UPDATE_TIMELINE_ITEM', id: t[i].id, updates: { status: 'done', label } });
            break;
          }
        }
      }
    };

    es.addEventListener('item.delta', onDelta);
    es.addEventListener('item.completed', onItemCompleted);
    return () => {
      es.removeEventListener('item.delta', onDelta);
      es.removeEventListener('item.completed', onItemCompleted);
    };
  }, []);

  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [activePanel, setActivePanel] = useState<string | null>(null);

  const closePanels = () => setActivePanel(null);
  const togglePanel = (name: string) => setActivePanel(p => p === name ? null : name);

  return (
    <>
      <div className="glow glow-1" />
      <div className="glow glow-2" />

      {/* Backdrop */}
      <div className={`backdrop ${activePanel ? 'open' : ''}`} onClick={closePanels} />

      {/* Hamburger */}
      <button className="hamburger" onClick={() => setSidebarOpen(v => !v)} title="Toggle Sidebar">☰</button>

      {/* Sidebar */}
      <Sidebar
        open={sidebarOpen}
        onClose={() => setSidebarOpen(false)}
        onPanelOpen={togglePanel}
      />

      {/* Connection Banner */}
      {/* SSE auto-reconnects; we can add state tracking later */}

      {/* Context Bar */}
      <ContextBar
        modelId={state.activeModelId}
        contextPct={state.contextPct}
        tokens={state.totalTokens}
        onClear={() => {/* TODO */}}
        onBookmarks={() => togglePanel('bookmarks')}
      />

      {/* Main Stage */}
      <Stage state={state} />

      {/* Input */}
      <InputArea />

      {/* Panels */}
      {activePanel === 'settings' && <SettingsPanel onClose={closePanels} />}
      {activePanel === 'agents' && <AgentsPanel onClose={closePanels} />}
      {activePanel === 'tools' && <ToolsPanel onClose={closePanels} />}
      {activePanel === 'mcp' && <MCPPanel onClose={closePanels} />}
      {activePanel === 'knowledge' && <KnowledgePanel onClose={closePanels} />}
      {activePanel === 'bookmarks' && <BookmarksPanel onClose={closePanels} />}

      {/* Toast */}
      <Toaster />
    </>
  );
}

export function App() {
  return (
    <AppProvider>
      <AppInner />
    </AppProvider>
  );
}
