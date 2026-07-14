import { useState, useEffect } from 'react';
import { useAppState } from '../../context/AppContext';
import { client, type Message } from '../../api/client';
import { t } from '../../i18n';

interface Props { onClose: () => void }

export function BookmarksPanel({ onClose }: Props) {
  const { state } = useAppState();
  const [msgs, setMsgs] = useState<Message[]>([]);

  useEffect(() => {
    if (state.activeThreadId) {
      client.getBookmarks(state.activeThreadId).then(setMsgs).catch(() => setMsgs([]));
    }
  }, [state.activeThreadId]);

  if (!state.activeThreadId) {
    return (
      <div className="panel open">
        <div className="panel-head">
          <h3>{t('bookmark.title')}</h3>
          <button className="panel-close" onClick={onClose}>✕</button>
        </div>
        <div className="panel-body">
          <div className="bookmarks-empty">{t('bookmark.home_hint')}</div>
        </div>
      </div>
    );
  }

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{t('bookmark.title')}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        {msgs.length === 0 ? (
          <div className="bookmarks-empty">{t('bookmark.empty')}</div>
        ) : (
          msgs.map(m => {
            const roleLabels: Record<string, string> = { user_message: 'You', agent_message: 'AI', tool_call: 'Tool' };
            const preview = m.content.length > 200 ? m.content.slice(0, 200) + '...' : m.content;
            return (
              <div key={m.id} className="agent-card" style={{ cursor: 'pointer' }}>
                <div className="agent-card-head">
                  <span className="agent-card-name">{roleLabels[m.kind] || m.kind}</span>
                  <span className="agent-card-desc" style={{ fontSize: 10 }}>{new Date(m.created_at).toLocaleString()}</span>
                  <div style={{ fontSize: 11, marginTop: 4, wordBreak: 'break-word' }}>
                    {preview.replace(/</g, '&lt;')}
                  </div>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
