import { useState, useEffect } from 'react';
import { marked } from 'marked';
import type { AppState, TimelineItem } from '../types';

marked.setOptions({ breaks: true, gfm: true });

interface Props {
  state: AppState;
}

export function Stage({ state }: Props) {
  const { timeline, liveItem, hasModels } = state;

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
    <main className="stage">
      <div className="timeline">
        {items.map(item => (
          <MessageCard key={item.id} item={item} />
        ))}
      </div>
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

  return (
    <div className={msgClass(item)}>
      <div className="msg-bubble">
        <div className="msg-header" onClick={() => setCollapsed(v => !v)} style={{ cursor: 'pointer' }}>
          <span className="msg-chevron">{collapsed ? '▶' : '▼'}</span>
          <span className="msg-label">{item.label}</span>
          {item.status === 'pending' && <span className="msg-badge pending">running</span>}
          {item.status === 'fail' && <span className="msg-badge error">error</span>}
        </div>
        {!collapsed && (
          <div className="msg-content">
            <pre>{item.content}</pre>
          </div>
        )}
      </div>
    </div>
  );
}

function MessageCard({ item }: { item: TimelineItem }) {
  const isTool = item.type === 'tool_call';
  const isError = item.type === 'error';

  const content = (item.type === 'agent' || item.type === 'user')
    ? renderMarkdown(item.content)
    : item.content;

  const hasThinking = item.type === 'agent' && item.thinking;

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
