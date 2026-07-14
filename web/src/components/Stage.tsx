import { t } from '../i18n';
import { marked } from 'marked';

// Configure marked
marked.setOptions({ breaks: true, gfm: true });

interface Props {
  state: import('../types').AppState;
}

export function Stage({ state }: Props) {
  const { timeline, liveItem, hasModels } = state;

  if (timeline.length === 0 && !liveItem) {
    return (
      <main className="stage">
        <div className="idle-state">
          <div className="idle-glow">✦</div>
          <div className="idle-text">{t('idle.text')}</div>
          {!hasModels && (
            <div style={{ textAlign: 'center' }}>
              <div style={{ fontSize: 14, fontWeight: 600, marginBottom: 4 }}>{t('timeline.no_models_setup_title')}</div>
              <div style={{ fontSize: 12, color: 'var(--text-dim)', marginBottom: 10 }}>{t('timeline.no_models_setup_hint')}</div>
            </div>
          )}
          <div className="idle-hints">
            <button className="hint-chip" onClick={() => fillInput(t('idle.hint1'))}>{t('idle.hint1')}</button>
            <button className="hint-chip" onClick={() => fillInput(t('idle.hint2'))}>{t('idle.hint2')}</button>
            <button className="hint-chip" onClick={() => fillInput(t('idle.hint3'))}>{t('idle.hint3')}</button>
          </div>
        </div>
      </main>
    );
  }

  // Group timeline items into turns
  const items = [...timeline];
  if (liveItem) items.push(liveItem);

  return (
    <main className="stage">
      <div className="timeline">
        {items.map(item => (
          <TurnItem key={item.id} item={item} />
        ))}
      </div>
    </main>
  );
}

function TurnItem({ item }: { item: import('../types').TimelineItem }) {
  const content = item.type === 'agent' || item.type === 'user'
    ? renderMarkdown(item.content)
    : item.content;

  const isTool = item.type === 'tool_call';

  return (
    <div className="turn-item">
      <span className="turn-icon">{item.icon}</span>
      <div className="turn-body">
        <div className="turn-header">
          <span>{item.label}</span>
          {item.status === 'streaming' && <span className="agent-tool-chip">...</span>}
          {item.status === 'pending' && <span className="agent-tool-chip" style={{ color: '#b8840b' }}>pending</span>}
          {item.status === 'fail' && <span className="agent-tool-chip" style={{ color: 'var(--red)' }}>error</span>}
        </div>
        <div
          className="turn-content"
          {...(isTool ? {} : {})}
          dangerouslySetInnerHTML={item.type === 'agent' || item.type === 'user' ? { __html: content } : undefined}
        >
          {item.type === 'tool_call' || item.type === 'tool_result' || item.type === 'error' ? (
            <pre>{content}</pre>
          ) : !(item.type === 'agent' || item.type === 'user') ? (
            content
          ) : null}
        </div>
      </div>
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

function fillInput(text: string) {
  const el = document.getElementById('msg-input') as HTMLTextAreaElement;
  if (el) { el.value = text; el.focus(); }
}
