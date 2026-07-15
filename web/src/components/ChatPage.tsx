import { useEffect, useRef, useState, useCallback } from 'react';
import { useAppState, processSSEEvent, messagesToTimeline } from '../context/AppContext';
import { client } from '../api/client';
import { Stage } from './Stage';
import { InputArea } from './InputArea';
import { t } from '../i18n';

const EFFORT_CYCLE: string[] = ['off', 'low', 'medium', 'high', 'max'];

export function ChatPage() {
  const { state, dispatch } = useAppState();
  const { activeThreadId, threads, agents, activeAgentId, models, activeModelId, tools, contextPct, totalTokens } = state;

  const abortRef = useRef<AbortController | null>(null);
  const sendingNewRef = useRef(false);
  const activeThreadRef = useRef(activeThreadId);
  activeThreadRef.current = activeThreadId;

  const timelineRef = useRef(state.timeline);
  timelineRef.current = state.timeline;

  const contentAccRef = useRef<Record<string, string>>({});
  const thinkingAccRef = useRef<Record<string, string>>({});

  const [configOpen, setConfigOpen] = useState(false);

  const activeAgent = agents.find(a => a.id === activeAgentId);
  const activeModel = models.find(m => m.id === activeModelId);

  // Thread switch: load history via messages API.
  useEffect(() => {
    if (sendingNewRef.current) {
      sendingNewRef.current = false;
      return;
    }
    abortRef.current?.abort();
    abortRef.current = null;
    contentAccRef.current = {};
    thinkingAccRef.current = {};

    const threadId = activeThreadId;
    if (!threadId) return;

    client.getMessages(threadId).then(({ messages }) => {
      if (threadId !== activeThreadRef.current) return;
      dispatch({ type: 'LOAD_TIMELINE', items: messagesToTimeline(messages) });
    }).catch(err => {
      console.error('load messages:', err);
    });
  }, [activeThreadId]);

  // Load threads list on mount
  useEffect(() => {
    client.listThreads().then(ts => dispatch({ type: 'SET_THREADS', threads: ts })).catch(() => {});
  }, []);

  const newThread = () => {
    abortRef.current?.abort();
    abortRef.current = null;
    dispatch({ type: 'SET_ACTIVE_THREAD', threadId: null });
    dispatch({ type: 'CLEAR_TIMELINE' });
  };

  // Send message via POST /api/chat, which returns SSE stream directly.
  const handleSendMessage = useCallback(async (body: {
    message: string; images: string[]; agent_id: number; thread_id?: string;
  }) => {
    abortRef.current?.abort();

    const ctrl = new AbortController();
    abortRef.current = ctrl;

    contentAccRef.current = {};
    thinkingAccRef.current = {};

    if (!body.thread_id) {
      sendingNewRef.current = true;
    }
    try {
      const res = await client.sendChat(body, ctrl.signal);
      const threadId = res.headers.get('X-Thread-Id');
      if (!threadId) return;

      if (!body.thread_id) {
        dispatch({ type: 'SET_ACTIVE_THREAD', threadId });
        client.listThreads().then(threads => dispatch({ type: 'SET_THREADS', threads }));
      }

      const reader = res.body!.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let eventType = '';
      let dataBuf = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (let line of lines) {
          if (line.endsWith('\r')) line = line.slice(0, -1);
          if (line === '') {
            if (dataBuf) {
              try {
                const data = JSON.parse(dataBuf);
                const ended = processSSEEvent(eventType, data, dispatch, timelineRef, contentAccRef, thinkingAccRef);
                if (ended) return;
              } catch { /* skip malformed JSON */ }
              eventType = ''; dataBuf = '';
            }
          } else if (line.startsWith('event: ')) {
            eventType = line.slice(7);
          } else if (line.startsWith('data: ')) {
            dataBuf += (dataBuf ? '\n' : '') + line.slice(6);
          }
        }
      }
    } catch (err: any) {
      if (err?.name === 'AbortError') return;
      console.error('chat error:', err);
    }
  }, []);

  const handleStop = useCallback(() => {
    abortRef.current?.abort();
    abortRef.current = null;
    if (activeThreadId) {
      client.pauseThread(activeThreadId).catch(() => {});
    }
  }, [activeThreadId]);

  const switchThread = (threadId: string) => {
    if (threadId === activeThreadId) return;
    abortRef.current?.abort();
    abortRef.current = null;
    dispatch({ type: 'CLEAR_TIMELINE' });
    dispatch({ type: 'SET_ACTIVE_THREAD', threadId });
  };

  const deleteThread = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (!confirm('Delete this thread?')) return;
    client.deleteThread(id).then(() => {
      dispatch({ type: 'REMOVE_THREAD', threadId: id });
      if (activeThreadId === id) {
        dispatch({ type: 'SET_ACTIVE_THREAD', threadId: null });
        dispatch({ type: 'CLEAR_TIMELINE' });
      }
    }).catch(console.error);
  };

  const handleAgentChange = (agentId: number) => {
    dispatch({ type: 'SET_ACTIVE_AGENT', agentId });
  };

  const handleModelChange = (modelId: string) => {
    dispatch({ type: 'SET_ACTIVE_MODEL', modelId });
  };

  const handleEffortChange = async () => {
    const current = activeModel?.reasoning_effort || 'off';
    const idx = EFFORT_CYCLE.indexOf(current);
    const next = EFFORT_CYCLE[(idx + 1) % EFFORT_CYCLE.length];
    const updated = models.map(m =>
      m.id === activeModelId ? { ...m, reasoning_effort: next } : m
    );
    dispatch({ type: 'SET_MODELS', models: updated });
    await client.updateSettings({ llm: { models: updated, active: activeModelId } }).catch(() => {});
  };

  const updateSystemPrompt = async (prompt: string) => {
    if (!activeAgent) return;
    const updated = agents.map(a =>
      a.id === activeAgent.id ? { ...a, system_prompt: prompt } : a
    );
    dispatch({ type: 'SET_AGENTS', agents: updated });
    await client.updateAgent(activeAgent.id, {
      name: activeAgent.name,
      desc: activeAgent.desc,
      system_prompt: prompt,
      tools: activeAgent.tools,
    }).catch(() => {});
  };

  const toggleTool = async (toolName: string) => {
    if (!activeAgent) return;
    let toolList: string[] = [];
    try { toolList = JSON.parse(activeAgent.tools || '[]'); } catch {}
    const newTools = toolList.includes(toolName)
      ? toolList.filter(t => t !== toolName)
      : [...toolList, toolName];
    const toolsJson = JSON.stringify(newTools);
    const updated = agents.map(a =>
      a.id === activeAgent.id ? { ...a, tools: toolsJson } : a
    );
    dispatch({ type: 'SET_AGENTS', agents: updated });
    await client.updateAgent(activeAgent.id, {
      name: activeAgent.name,
      desc: activeAgent.desc,
      system_prompt: activeAgent.system_prompt,
      tools: toolsJson,
    }).catch(() => {});
  };

  const agentTools: string[] = (() => {
    try { return JSON.parse(activeAgent?.tools || '[]'); } catch { return []; }
  })();

  const effortLabel = activeModel?.reasoning_effort || 'off';

  const clearContext = () => {
    if (!activeThreadId) return;
    if (!confirm('Clear the conversation context for this thread?')) return;
    client.pauseThread(activeThreadId).then(() => {
      dispatch({ type: 'CLEAR_TIMELINE' });
    }).catch(err => alert('Clear failed: ' + err.message));
  };

  return (
    <div className="chat-page">
      <div className="chat-top">
        <select
          className="chat-agent-select"
          value={activeAgentId ?? ''}
          onChange={e => handleAgentChange(Number(e.target.value))}
        >
          {agents.map(a => (
            <option key={a.id} value={a.id}>{a.name}</option>
          ))}
        </select>
        <button className="chat-new-thread" onClick={newThread}>+ New Thread</button>
        <button
          className={`chat-config-toggle ${configOpen ? 'open' : ''}`}
          onClick={() => setConfigOpen(v => !v)}
        >
          {configOpen ? 'Hide Config' : 'Config'}
        </button>
      </div>

      <div className={`chat-config ${configOpen ? 'open' : ''}`}>
        <div className="chat-config-row">
          <div className="chat-config-field chat-config-prompt">
            <label>System Prompt</label>
            <textarea
              value={activeAgent?.system_prompt || ''}
              onChange={e => updateSystemPrompt(e.target.value)}
            />
          </div>
        </div>
        <div className="chat-config-row">
          <div className="chat-config-field chat-config-model">
            <label>Model</label>
            <select value={activeModelId} onChange={e => handleModelChange(e.target.value)}>
              {models.map(m => (
                <option key={m.id} value={m.id}>{m.id}</option>
              ))}
            </select>
          </div>
          <div className="chat-config-field chat-config-effort">
            <label>Reasoning</label>
            <button
              className="card-btn"
              onClick={handleEffortChange}
              title={`Reasoning effort: ${effortLabel}`}
              style={{ padding: '6px 10px', fontFamily: 'var(--font-mono)', fontSize: 11, textTransform: 'uppercase' }}
            >
              {effortLabel === 'off' ? '⊇' : '◈'} {effortLabel}
            </button>
          </div>
          <div className="chat-config-field" style={{ flex: 1 }}>
            <label>Context ({contextPct}% / {totalTokens} tokens)</label>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, height: 28 }}>
              <div style={{ flex: 1, height: 3, background: 'var(--border)', borderRadius: 2 }}>
                <div style={{ width: `${contextPct}%`, height: '100%', background: 'var(--accent)', borderRadius: 2, transition: 'width 0.3s' }} />
              </div>
              <button className="card-btn" onClick={clearContext} style={{ fontSize: 10 }}>
                {t('ctx.clear')}
              </button>
            </div>
          </div>
        </div>
        <div className="chat-config-row">
          <div className="chat-config-field chat-config-tools">
            <label>Tools</label>
            <div className="tools-chips">
              {agentTools.map(tn => (
                <span key={tn} className="tool-chip">
                  {tn}
                  <button className="tool-chip-remove" onClick={() => toggleTool(tn)}>×</button>
                </span>
              ))}
              {agentTools.length === 0 && <span style={{ fontSize: 10, color: 'var(--faint)' }}>No tools selected</span>}
            </div>
            <details style={{ marginTop: 4 }}>
              <summary style={{ fontSize: 10, color: 'var(--muted)', cursor: 'pointer' }}>Add tools</summary>
              <div className="tools-picker" style={{ marginTop: 4 }}>
                {tools.filter(t => !agentTools.includes(t.name)).map(t => (
                  <div
                    key={t.name}
                    className="tool-pick-item"
                    onClick={() => toggleTool(t.name)}
                  >
                    <span className="tool-pick-name">
                      {t.name}
                      <span className={`tool-source-badge ${t.source}`}>{t.source}</span>
                    </span>
                    <span className="tool-pick-desc">{t.description}</span>
                  </div>
                ))}
              </div>
            </details>
          </div>
        </div>
      </div>

      <div className="chat-body">
        <div className="thread-list">
          {threads.length === 0 ? (
            <div className="thread-list-empty">No threads yet</div>
          ) : (
            threads.map(th => (
              <div
                key={th.id}
                className={`thread-item ${th.id === activeThreadId ? 'active' : ''}`}
                onClick={() => switchThread(th.id)}
              >
                {th.title || th.id.slice(0, 8)}
                <button className="thread-item-del" onClick={e => deleteThread(th.id, e)}>×</button>
              </div>
            ))
          )}
        </div>
        <div className="chat-col">
          <Stage state={state} />
          <InputArea onSendMessage={handleSendMessage} onStop={handleStop} />
        </div>
      </div>
    </div>
  );
}
