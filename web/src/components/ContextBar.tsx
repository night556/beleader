import { t } from '../i18n';

interface Props {
  modelId: string;
  contextPct: number;
  tokens: number;
  onClear: () => void;
  onBookmarks: () => void;
}

export function ContextBar({ modelId, contextPct, tokens, onClear, onBookmarks }: Props) {
  return (
    <div className="ctx-bar">
      <div className="ctx-model" title={t('ctx.model_title')}>
        <span className="ctx-model-dot" />
        <span className="ctx-model-name">{modelId || '—'}</span>
      </div>
      <div className="ctx-progress-wrap">
        <div className="ctx-progress">
          <div className="ctx-progress-fill" style={{ width: `${contextPct}%` }} />
        </div>
        <span className="ctx-label">{contextPct}%</span>
      </div>
      <span className="ctx-tokens" title={t('ctx.tokens_title')}>{tokens}</span>
      <button className="ctx-clear" onClick={onClear}>{t('ctx.clear')}</button>
      <button className="ctx-bookmarks" onClick={onBookmarks}>☆</button>
    </div>
  );
}
