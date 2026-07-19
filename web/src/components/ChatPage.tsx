import { useEffect, useRef, useCallback, useState } from 'react';
import { useAppState, processSSEEvent, messagesToTimeline } from '../context/AppContext';
import { client } from '../api/client';
import { Stage } from './Stage';
import { InputArea } from './InputArea';

const EFFORT_CYCLE: string[] = ['off', 'low', 'medium', 'high', 'max'];

const EFFORT_ICON: Record<string, string> = {
  off: '⊇', low: '◢', medium: '◈', high: '◉', max: '●',
};

export function ChatPage() {
  const { state, dispatch } = useAppState();
  const { activeThreadId, threads, agents, activeAgentId, models, activeModelId, contextPct, totalTokens } = state;

  const abortRef = useRef<AbortController | null>(null);
  const sendingNewRef = useRef(false);
  const activeThreadRef = useRef(activeThreadId);
  activeThreadRef.current = activeThreadId;

  const timelineRef = useRef(state.timeline);
  timelineRef.current = state.timeline;

  const contentAccRef = useRef<Record<string, string>>({});
  const thinkingAccRef = useRef<Record<string, string>>({});
  const turnIdRef = useRef<string>('');

  const activeModel = models.find(m => m.id === activeModelId);

  // Per-conversation effort override. Defaults to the model's setting, cycles locally.
  const [effort, setEffort] = useState<string>(() => activeModel?.reasoning_effort || 'off');
  const effortRef = useRef(effort);
  effortRef.current = effort;

  // Reset effort when model changes.
  useEffect(() => {
    const v = activeModel?.reasoning_effort || 'off';
    setEffort(v);
    effortRef.current = v;
  }, [activeModelId]);

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

  // Load threads list + runtimes on mount
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
    message: string; images: string[]; agent_id: number; thread_id?: string; model_id?: string;
  }) => {
    const ctrl = new AbortController();
    abortRef.current = ctrl;

    contentAccRef.current = {};
    thinkingAccRef.current = {};

    if (!body.thread_id) {
      sendingNewRef.current = true;
    }
    const fullBody = { ...body, reasoning_effort: effortRef.current };
    try {
      const res = await client.sendChat(fullBody, ctrl.signal);
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
                const ended = processSSEEvent(eventType, data, dispatch, timelineRef, contentAccRef, thinkingAccRef, turnIdRef);
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
    if (!activeThreadId) return;
    // Tell Gateway to cancel the turn. Runtime will exit RunLoop,
    // emit turn.completed (Interrupted), and the SSE stream will end cleanly.
    client.pauseThread(activeThreadId).catch(() => {});
    // Fallback: force abort if the stream hasn't ended within 5s.
    setTimeout(() => {
      if (abortRef.current) {
        abortRef.current.abort();
        abortRef.current = null;
      }
    }, 5000);
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

  const handleEffortChange = () => {
    const idx = EFFORT_CYCLE.indexOf(effort);
    setEffort(EFFORT_CYCLE[(idx + 1) % EFFORT_CYCLE.length]);
  };

  return (
    <div className="chat-page">
      <div className="chat-top">
        <div className="chat-top-controls">
          <div className="chat-top-field">
            <select
              className="chat-top-select"
              value={activeAgentId ?? ''}
              onChange={e => handleAgentChange(Number(e.target.value))}
            >
              {agents.map(a => (
                <option key={a.id} value={a.id}>{a.name}</option>
              ))}
            </select>
          </div>
          <div className="chat-top-field">
            <select
              className="chat-top-select"
              value={activeModelId}
              onChange={e => handleModelChange(e.target.value)}
            >
              {models.map(m => (
                <option key={m.id} value={m.id}>{m.id}</option>
              ))}
            </select>
          </div>
          <div className="chat-top-field">
            <button
              className="chat-top-effort"
              onClick={handleEffortChange}
              title={`Reasoning effort: ${effort}`}
            >
              <span className="chat-top-effort-icon">{EFFORT_ICON[effort] || EFFORT_ICON.off}</span>
              {effort}
            </button>
          </div>
        </div>

        <div className="chat-top-spacer" />

        {activeThreadId && (
          <div className="chat-top-context" title={`${contextPct}% / ${totalTokens} tokens`}>
            <div className="chat-top-context-bar">
              <div className="chat-top-context-fill" style={{ width: `${Math.min(contextPct, 100)}%` }} />
            </div>
            <span className="chat-top-context-pct">{contextPct}%</span>
          </div>
        )}
      </div>

      <div className="chat-body">
        <div className="thread-list">
          <div className="thread-list-head">
            <span className="thread-list-title">Threads</span>
            <button className="thread-new-btn" onClick={newThread} title="New Thread">+</button>
          </div>
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
