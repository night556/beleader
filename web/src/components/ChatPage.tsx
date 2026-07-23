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
  const { activeThreadId, threads, agents, activeAgentId, models, activeModelId, contextPct, totalTokens, workerParentId } = state;

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
      // New thread: reset token tracking and turn ID
      dispatch({ type: 'SET_CONTEXT_PCT', pct: 0 });
      turnIdRef.current = '';
      return;
    }
    contentAccRef.current = {};
    thinkingAccRef.current = {};
    turnIdRef.current = '';

    const threadId = activeThreadId;
    if (!threadId) {
      dispatch({ type: 'SET_CONTEXT_PCT', pct: 0 });
      return;
    }

    dispatch({ type: 'SET_CONTEXT_PCT', pct: 0 });

    client.getMessages(threadId).then(({ messages, has_more }) => {
      if (threadId !== activeThreadRef.current) return;
      dispatch({ type: 'LOAD_TIMELINE', items: messagesToTimeline(messages), hasMore: has_more });
    }).catch(err => {
      console.error('load messages:', err);
    });
  }, [activeThreadId]);

  // Gateway SSE — unified event stream for turns, items, and worker events.
  useEffect(() => {
    if (!activeThreadId || workerParentId) return;

    const ctrl = new AbortController();
    const url = `/api/sse?thread_id=${encodeURIComponent(activeThreadId)}`;

    fetch(url, { signal: ctrl.signal }).then(async (res) => {
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
                // Worker events: create/update worker cards.
                if (eventType === 'worker.dispatched') {
                  const wid = data.payload?.thread_id;
                  if (wid) {
                    dispatch({ type: 'UPDATE_TIMELINE_ITEM_BY_WORKER', workerThreadId: wid, updates: { workerThreadId: wid } });
                  }
                } else if (eventType === 'worker.completed') {
                  const wid = data.payload?.thread_id;
                  const st = data.payload?.status === 'stopped' ? 'stopped' : 'completed';
                  if (wid) {
                    dispatch({ type: 'UPDATE_TIMELINE_ITEM_BY_WORKER', workerThreadId: wid, updates: { workerStatus: st } });
                  }
                } else {
                  // Turn / item events: same processing as old chat SSE.
                  processSSEEvent(eventType, data, dispatch, timelineRef, contentAccRef, thinkingAccRef, turnIdRef);
                }
              } catch {}
              eventType = ''; dataBuf = '';
            }
          } else if (line.startsWith('event: ')) {
            eventType = line.slice(7);
          } else if (line.startsWith('data: ')) {
            dataBuf += (dataBuf ? '\n' : '') + line.slice(6);
          }
        }
      }
    }).catch(() => {});

    return () => ctrl.abort();
  }, [activeThreadId, workerParentId]);

  // Load threads list + runtimes on mount
  useEffect(() => {
    client.listThreads().then(ts => dispatch({ type: 'SET_THREADS', threads: ts })).catch(() => {});
  }, []);

  const newThread = () => {
    dispatch({ type: 'SET_ACTIVE_THREAD', threadId: null });
    dispatch({ type: 'CLEAR_TIMELINE' });
  };

  // Send message via POST /api/chat. Turn runs in background; events arrive via Gateway SSE.
  const handleSendMessage = useCallback(async (body: {
    message: string; images: string[]; agent_id: number; thread_id?: string; model_id?: string;
  }) => {
    contentAccRef.current = {};
    thinkingAccRef.current = {};

    // Immediately show user message in timeline
    dispatch({
      type: 'PUSH_TIMELINE_ITEM', item: {
        id: `pending_${Date.now()}`, type: 'user', label: 'You',
        content: body.message, status: 'done', time: Date.now(),
      },
    });

    if (!body.thread_id) {
      sendingNewRef.current = true;
    }

    const fullBody = { ...body, reasoning_effort: effortRef.current };
    try {
      const res = await client.sendChat(fullBody);
      if (!res.thread_id) return;

      if (!body.thread_id) {
        dispatch({ type: 'SET_ACTIVE_THREAD', threadId: res.thread_id });
        client.listThreads().then(threads => dispatch({ type: 'SET_THREADS', threads }));
      }
    } catch (err: any) {
      if (err?.name === 'AbortError') return;
      console.error('chat error:', err);
      dispatch({
        type: 'PUSH_TIMELINE_ITEM', item: {
          id: `err_${Date.now()}`, type: 'error', label: 'Error',
          content: err?.message || 'Failed to send message', status: 'fail', time: Date.now(),
        },
      });
    }
  }, []);

  const handleStop = useCallback(() => {
    if (!activeThreadId) return;
    client.pauseThread(activeThreadId).catch(() => {});
  }, [activeThreadId]);

  const loadMoreMessages = useCallback(() => {
    if (!activeThreadId || state.loadingMore || !state.hasMoreMessages) return;

    // Get the oldest message ID in timeline (format: "msg123" or "tc123_456")
    const firstItem = state.timeline[0];
    if (!firstItem) return;
    const m = firstItem.id.match(/\d+/);
    if (!m) return;
    const oldestId = parseInt(m[0], 10);
    if (!oldestId) return;

    dispatch({ type: 'SET_LOADING_MORE', loading: true });

    client.getMessages(activeThreadId, 0, 100).then(({ messages, has_more }) => {
      // Filter out messages we already have (id <= oldestId)
      const newMsgs = messages.filter(msg => msg.id < oldestId);
      if (newMsgs.length === 0) {
        dispatch({ type: 'SET_HAS_MORE', hasMore: false });
        dispatch({ type: 'SET_LOADING_MORE', loading: false });
        return;
      }
      const newItems = messagesToTimeline(newMsgs);
      dispatch({ type: 'PREPEND_TIMELINE_ITEMS', items: newItems });
      dispatch({ type: 'SET_HAS_MORE', hasMore: has_more });
      dispatch({ type: 'SET_LOADING_MORE', loading: false });

      // Preserve scroll position: adjust scrollTop by the height of added content
      requestAnimationFrame(() => {
        const el = document.querySelector('.stage') as HTMLElement;
        if (!el) return;
        // The new content was added at the top, so scrollHeight increased.
        // We need to keep the user's viewport at the same position.
        // This is handled by the Stage component's scroll preservation.
      });
    }).catch(() => {
      dispatch({ type: 'SET_LOADING_MORE', loading: false });
    });
  }, [activeThreadId, state.loadingMore, state.hasMoreMessages, state.timeline]);

  const switchThread = (threadId: string) => {
    if (threadId === activeThreadId) return;
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

        {workerParentId && (
          <button className="worker-back-btn" onClick={() => dispatch({ type: 'BACK_TO_PARENT' })}>
            ← Back to parent
          </button>
        )}

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
          <Stage state={state} onLoadMore={loadMoreMessages} />
          <InputArea onSendMessage={handleSendMessage} onStop={handleStop} />
        </div>
      </div>
    </div>
  );
}
