import { useState, useEffect, useRef, useCallback, memo, useMemo, Fragment } from 'react';
import { marked } from 'marked';
import hljs from 'highlight.js/lib/common';
import type { AppState, TimelineItem, TokenUsage } from '../types';
import { useAppState } from '../context/AppContext';

// Configure marked with custom code block rendering
const renderer = new marked.Renderer();
renderer.code = function({ text, lang }: { text: string; lang?: string }) {
  const language = lang && hljs.getLanguage(lang) ? lang : 'plaintext';
  try {
    const highlighted = hljs.highlight(text, { language }).value;
    return `<pre><code class="hljs language-${language}">${highlighted}</code></pre>`;
  } catch {
    return `<pre><code class="hljs">${text}</code></pre>`;
  }
};
marked.setOptions({ breaks: true, gfm: true, renderer });

interface Props {
  state: AppState;
  onLoadMore?: () => void;
}

export function Stage({ state, onLoadMore }: Props) {
  const { timeline, liveItem, hasModels, hasMoreMessages, loadingMore } = state;
  const scrollRef = useRef<HTMLDivElement>(null);
  const atBottomRef = useRef(true);
  const prevLenRef = useRef(timeline.length);
  const prevScrollHeightRef = useRef(0);
  const [showScrollBtn, setShowScrollBtn] = useState(false);

  const scrollToBottom = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    atBottomRef.current = true;
    setShowScrollBtn(false);
  }, []);

  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
    atBottomRef.current = dist < 50;
    setShowScrollBtn(dist >= 50);

    if (el.scrollTop < 50 && hasMoreMessages && !loadingMore && onLoadMore) {
      prevScrollHeightRef.current = el.scrollHeight;
      onLoadMore();
    }
  }, [hasMoreMessages, loadingMore, onLoadMore]);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    // When timeline grows, scroll to bottom if we were at bottom
    if (timeline.length > prevLenRef.current) {
      if (prevScrollHeightRef.current > 0 && el.scrollHeight > prevScrollHeightRef.current) {
        const addedHeight = el.scrollHeight - prevScrollHeightRef.current;
        el.scrollTop = addedHeight;
        prevScrollHeightRef.current = 0;
      } else {
        scrollToBottom();
      }
    } else if (atBottomRef.current) {
      el.scrollTop = el.scrollHeight;
    } else {
      // Timeline replaced (thread switch / replay) — force scroll to bottom
      // since we can't track per-thread scroll position
      scrollToBottom();
    }
    prevLenRef.current = timeline.length;
  }, [timeline, liveItem, scrollToBottom]);

  const items = [...timeline];
  if (liveItem) items.push(liveItem);
  const grouped = useMemo(() => groupByTurn(items), [items]);

  if (timeline.length === 0 && !liveItem) {
    return (
      <main className="stage">
        <div className="idle-state">
          <div className="idle-glow">✦</div>
          <div className="idle-text">Start a conversation</div>
          {hasModels ? (
            <div className="idle-hints">
              <div className="idle-hint">💡 Ask me to write code, analyze files, or search the web</div>
              <div className="idle-hint">💡 Use spawn_worker for parallel tasks</div>
            </div>
          ) : (
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 4 }}>No models configured</div>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 10 }}>Add a model in the Model tab to get started.</div>
            </div>
          )}
        </div>
      </main>
    );
  }

  return (
    <main className="stage" ref={scrollRef} onScroll={handleScroll}>
      <div className="timeline">
        {hasMoreMessages && (
          <div className="load-more-indicator">
            {loadingMore ? 'Loading...' : '↑ Scroll up to load more'}
          </div>
        )}
        {grouped.map(g => {
          if (Array.isArray(g)) {
            return <TurnBubble key={g[0].turnId || g[0].id} items={g} />;
          }
          return <MessageCard key={g.id} item={g} />;
        })}
      </div>
      {showScrollBtn && (
        <button className="scroll-bottom-btn" onClick={scrollToBottom} title="Scroll to bottom">
          ↓
        </button>
      )}
    </main>
  );
}

// Group consecutive AI items with the same turnId into one bubble.
function groupByTurn(items: TimelineItem[]): Array<TimelineItem | TimelineItem[]> {
  const result: Array<TimelineItem | TimelineItem[]> = [];
  let i = 0;
  while (i < items.length) {
    const item = items[i];
    if (item.turnId && (item.type === 'agent' || item.type === 'tool_call' || item.type === 'worker')) {
      const group: TimelineItem[] = [item];
      const turnId = item.turnId;
      i++;
      while (i < items.length && items[i].turnId === turnId) {
        group.push(items[i]);
        i++;
      }
      result.push(group);
    } else {
      result.push(item);
      i++;
    }
  }
  return result;
}

const TurnBubble = memo(function TurnBubble({ items }: { items: TimelineItem[] }) {
  const hasStreaming = items.some(i => i.status === 'streaming');
  const allContent = items.filter(i => i.type === 'agent').map(i => i.content).join('');
  const usage = items.find(i => i.type === 'agent' && i.usage)?.usage;
  const usageText = usage ? formatUsage(usage) : '';

  return (
    <div className="msg msg-agent">
      <div className="msg-bubble turn-bubble">
        <div className="msg-header">
          <span className="msg-label">AI</span>
          {hasStreaming && <span className="msg-badge streaming">...</span>}
          {!hasStreaming && <CopyButton text={allContent} />}
        </div>
        {items.map(item => {
          if (item.type === 'agent') {
            const html = renderMarkdown(item.content);
            return (
              <Fragment key={item.id}>
                {item.thinking && <ThinkingBlock thinking={item.thinking} streaming={item.status === 'streaming'} />}
                {item.content && <div className="msg-content" dangerouslySetInnerHTML={{ __html: html }} />}
              </Fragment>
            );
          }
          if (item.type === 'tool_call') {
            return <Fragment key={item.id}><ToolInlineItem item={item} /></Fragment>;
          }
          if (item.type === 'worker') {
            return <Fragment key={item.id}><WorkerInlineItem item={item} /></Fragment>;
          }
          return null;
        })}
        {usageText && <div className="msg-usage">{usageText}</div>}
      </div>
    </div>
  );
});

function ToolInlineItem({ item }: { item: TimelineItem }) {
  const isDone = item.status === 'done' || item.status === 'fail';
  const [open, setOpen] = useState(!isDone);

  useEffect(() => {
    setOpen(item.status === 'pending' || item.status === 'streaming');
  }, [item.status]);

  const params = formatArgs(item.args || '');

  return (
    <div className="tools-inline">
      <div className="tools-inline-header" onClick={() => setOpen(v => !v)}>
        <span className="tools-inline-chevron">{open ? '▼' : '▶'}</span>
        <span className="tools-inline-item-name">{item.label}</span>
        {params && <span className="tools-inline-item-args">{params}</span>}
        {item.status === 'pending' && <span className="tools-inline-item-badge running">running</span>}
        {item.status === 'fail' && <span className="tools-inline-item-badge error">error</span>}
      </div>
      {open && item.content && (
        <div className="tools-inline-body">
          <div className="tools-inline-item done">
            <pre className="tools-inline-item-result">{item.content}</pre>
          </div>
        </div>
      )}
    </div>
  );
}

function WorkerInlineItem({ item }: { item: TimelineItem }) {
  const { dispatch, state } = useAppState();
  const isDone = item.workerStatus === 'completed';
  const isStopped = item.workerStatus === 'stopped';
  const [open, setOpen] = useState(!isDone && !isStopped);

  return (
    <div className="tools-inline">
      <div className="tools-inline-header" onClick={() => setOpen(v => !v)}>
        <span className="tools-inline-chevron">{open ? '▼' : '▶'}</span>
        <span className="tools-inline-item-name">{item.workerAgent || item.label}</span>
        <span className="tools-inline-item-args">{item.workerTask || item.content}</span>
        {item.status === 'pending' && <span className="tools-inline-item-badge running">running</span>}
        {isDone && <span className="tools-inline-item-badge done">done</span>}
        {isStopped && <span className="tools-inline-item-badge error">stopped</span>}
      </div>
      {open && (
        <div className="tools-inline-body">
          <div className="tools-inline-item done">
            {item.workerThreadId && (
              <button className="worker-view-btn" onClick={() => dispatch({ type: 'VIEW_WORKER', threadId: item.workerThreadId!, parentId: state.activeThreadId || '' })}>
                View thread →
              </button>
            )}
          </div>
        </div>
      )}
    </div>
  );
}

function msgClass(item: TimelineItem): string {
  const base = 'msg';
  switch (item.type) {
    case 'user': return `${base} msg-user`;
    case 'agent': return `${base} msg-agent`;
    case 'tool_call': return `${base} msg-tool`;
    case 'worker': return `${base} msg-worker`;
    case 'error': return `${base} msg-error`;
    case 'system': return `${base} msg-system`;
    default: return base;
  }
}

function CopyButton({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);
  const copy = useCallback((e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(text).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    });
  }, [text]);
  return (
    <button className="msg-copy-btn" onClick={copy} title="Copy">
      {copied ? '✓' : '⎘'}
    </button>
  );
}

function formatArgs(args: string): string {
  if (!args) return '';
  try {
    const obj = JSON.parse(args);
    const keys = Object.keys(obj);
    if (keys.length === 0) return '';
    return keys.map(k => `${k}: ${JSON.stringify(obj[k])}`).join('  ');
  } catch {
    return args.length > 80 ? args.slice(0, 77) + '...' : args;
  }
}

function formatUsage(u: TokenUsage): string {
  const parts = [`${u.total.toLocaleString()} tokens`];
  parts.push(`in: ${u.prompt.toLocaleString()}`);
  parts.push(`out: ${u.completion.toLocaleString()}`);
  if (u.cached > 0) parts.push(`cache: ${u.cached.toLocaleString()}`);
  return parts.join(' · ');
}

const MessageCard = memo(function MessageCard({ item }: { item: TimelineItem }) {
  const isError = item.type === 'error';
  const isSystem = item.type === 'system';

  const content = useMemo(() => {
    return (item.type === 'agent' || item.type === 'user')
      ? renderMarkdown(item.content)
      : item.content;
  }, [item.type, item.content]);

  if (isSystem) {
    return (
      <div className={msgClass(item)}>
        <div className="msg-system-banner">
          <span className="msg-system-icon">⚡</span>
          <span>{item.content}</span>
        </div>
      </div>
    );
  }

  if (isError) {
    return (
      <div className={msgClass(item)}>
        <div className="msg-bubble">
          <div className="msg-header">
            <span className="msg-label">{item.label}</span>
          </div>
          <div className="msg-content">
            <pre>{content}</pre>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className={msgClass(item)}>
      <div className="msg-bubble">
        <div className="msg-header">
          <span className="msg-label">{item.label}</span>
          {item.status === 'streaming' && <span className="msg-badge streaming">...</span>}
          {item.status === 'done' && item.type === 'agent' && <CopyButton text={item.content} />}
        </div>
        {item.type === 'agent' && item.thinking && <ThinkingBlock thinking={item.thinking} streaming={item.status === 'streaming'} />}
        <div
          className="msg-content"
          dangerouslySetInnerHTML={
            (item.type === 'agent' || item.type === 'user')
              ? { __html: content }
              : undefined
          }
        >
          {(item.type !== 'agent' && item.type !== 'user') ? (
            <pre>{content}</pre>
          ) : null}
        </div>
        {item.type === 'agent' && item.usage && <div className="msg-usage">{formatUsage(item.usage)}</div>}
      </div>
    </div>
  );
});

function ThinkingBlock({ thinking, streaming }: { thinking: string; streaming: boolean }) {
  const [collapsed, setCollapsed] = useState(true);

  useEffect(() => {
    if (streaming) {
      setCollapsed(false);
    } else {
      setCollapsed(true);
    }
  }, [streaming]);

  return (
    <div className={`thinking-block ${streaming ? 'thinking-streaming' : ''}`}>
      <div className="thinking-header" onClick={() => setCollapsed(v => !v)}>
        <span className="thinking-chevron">{collapsed ? '▶' : '▼'}</span>
        <span>{streaming ? 'Thinking...' : 'Thought Process'}</span>
      </div>
      {!collapsed && (
        <div className="thinking-body">
          <pre>{thinking}</pre>
        </div>
      )}
    </div>
  );
}

function renderMarkdown(text: string): string {
  try {
    return marked.parse(text) as string;
  } catch {
    return text.replace(/</g, '&lt;').replace(/>/g, '&gt;');
  }
}
