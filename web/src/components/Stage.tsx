import { useState, useEffect, useRef, useCallback } from 'react';
import { marked } from 'marked';
import type { AppState, TimelineItem, TokenUsage } from '../types';

marked.setOptions({ breaks: true, gfm: true });

interface Props {
  state: AppState;
}

export function Stage({ state }: Props) {
  const { timeline, liveItem, hasModels } = state;
  const scrollRef = useRef<HTMLDivElement>(null);
  const atBottomRef = useRef(true);
  const prevLenRef = useRef(timeline.length);
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
  }, []);

  // Auto-scroll when content changes.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    if (timeline.length > prevLenRef.current) {
      // New message was added → always scroll to bottom.
      scrollToBottom();
    } else if (atBottomRef.current) {
      // Streaming update + already at bottom → auto-scroll.
      el.scrollTop = el.scrollHeight;
    }
    prevLenRef.current = timeline.length;
  }, [timeline, liveItem, scrollToBottom]);

  if (timeline.length === 0 && !liveItem) {
    return (
      <main className="stage">
        <div className="idle-state">
          <div className="idle-glow">✦</div>
          <div className="idle-text">Start a conversation</div>
          {!hasModels && (
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 4 }}>No models configured</div>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 10 }}>Add a model in the Model tab to get started.</div>
            </div>
          )}
        </div>
      </main>
    );
  }

  const items = [...timeline];
  if (liveItem) items.push(liveItem);

  return (
    <main className="stage" ref={scrollRef} onScroll={handleScroll}>
      <div className="timeline">
        {items.map(item => (
          <MessageCard key={item.id} item={item} />
        ))}
      </div>
      {showScrollBtn && (
        <button className="scroll-bottom-btn" onClick={scrollToBottom} title="Scroll to bottom">
          ↓
        </button>
      )}
    </main>
  );
}

function msgClass(item: TimelineItem): string {
  const base = 'msg';
  switch (item.type) {
    case 'user': return `${base} msg-user`;
    case 'agent': return `${base} msg-agent`;
    case 'tool_call': return `${base} msg-tool`;
    case 'error': return `${base} msg-error`;
    default: return base;
  }
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

function ToolCard({ item }: { item: TimelineItem }) {
  const isDone = item.status === 'done' || item.status === 'fail';
  const [collapsed, setCollapsed] = useState(isDone);

  useEffect(() => {
    if (item.status === 'pending' || item.status === 'streaming') {
      setCollapsed(false);
    } else {
      setCollapsed(true);
    }
  }, [item.status]);

  const params = formatArgs(item.args || '');

  return (
    <div className={msgClass(item)}>
      <div className="msg-bubble">
        <div className="msg-header" onClick={() => setCollapsed(v => !v)} style={{ cursor: 'pointer' }}>
          <span className="msg-chevron">{collapsed ? '▶' : '▼'}</span>
          <span className="msg-label">{item.label}</span>
          {params && <span className="tool-params">{params}</span>}
          {item.status === 'pending' && <span className="msg-badge pending">running</span>}
          {item.status === 'fail' && <span className="msg-badge error">error</span>}
        </div>
        {!collapsed && item.content && (
          <div className="msg-content">
            <pre>{item.content}</pre>
          </div>
        )}
      </div>
    </div>
  );
}

function formatUsage(u: TokenUsage): string {
  const parts = [`${u.total.toLocaleString()} tokens`];
  parts.push(`in: ${u.prompt.toLocaleString()}`);
  parts.push(`out: ${u.completion.toLocaleString()}`);
  if (u.cached > 0) parts.push(`cache: ${u.cached.toLocaleString()}`);
  return parts.join(' · ');
}

function MessageCard({ item }: { item: TimelineItem }) {
  const isTool = item.type === 'tool_call';
  const isError = item.type === 'error';

  const content = (item.type === 'agent' || item.type === 'user')
    ? renderMarkdown(item.content)
    : item.content;

  const hasThinking = item.type === 'agent' && item.thinking;
  const usageText = item.type === 'agent' && item.usage ? formatUsage(item.usage) : '';

  if (isTool) {
    return <ToolCard item={item} />;
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
        </div>
        {hasThinking && <ThinkingBlock thinking={item.thinking!} streaming={item.status === 'streaming'} />}
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
        {usageText && <div className="msg-usage">{usageText}</div>}
      </div>
    </div>
  );
}

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
