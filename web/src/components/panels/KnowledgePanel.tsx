import { useState, useEffect } from 'react';
import { client } from '../../api/client';
import { t } from '../../i18n';
import type { Knowledge } from '../../types';

interface Props { onClose: () => void }

export function KnowledgePanel({ onClose }: Props) {
  const [items, setItems] = useState<Knowledge[]>([]);
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(0);

  useEffect(() => {
    client.listKnowledge(20, page * 20).then(setItems).catch(() => {});
  }, [page]);

  const search = () => {
    if (!query.trim()) {
      client.listKnowledge().then(setItems).catch(() => {});
      return;
    }
    client.searchKnowledge(query).then(setItems).catch(() => {});
  };

  const deleteItem = (id: number) => {
    if (!confirm('Delete this knowledge entry?')) return;
    client.deleteKnowledge(id).then(() => setItems(prev => prev.filter(i => i.id !== id))).catch(() => {});
  };

  const editItem = (item: Knowledge) => {
    const title = prompt('Title:', item.title);
    if (title === null) return;
    const content = prompt('Content:', item.content);
    if (content === null) return;
    client.updateKnowledge(item.id, { title: title || undefined, content: content || undefined }).then(() => {
      setItems(prev => prev.map(i => i.id === item.id ? { ...i, title: title || i.title, content: content || i.content } : i));
    }).catch(() => {});
  };

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{t('knowledge.title')}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <input
            style={{ flex: 1, padding: '6px 8px', border: '1px solid var(--border)', borderRadius: 6, background: 'var(--bg3)', color: 'var(--text)', fontSize: 12 }}
            placeholder={t('knowledge.search_placeholder')}
            value={query}
            onChange={e => setQuery(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') search(); }}
          />
          <button className="btn-add" onClick={search}>Search</button>
        </div>
        {items.length === 0 ? (
          <div className="agents-empty">No entries</div>
        ) : (
          items.map(item => (
            <div key={item.id} className="agent-card">
              <div className="agent-card-head">
                <span className="agent-card-name">{item.title}</span>
                <span className="agent-card-desc">{item.source}</span>
                <span className="agent-card-actions">
                  <button className="agent-card-btn" onClick={() => editItem(item)}>{t('knowledge.edit')}</button>
                  <button className="agent-card-btn delete" onClick={() => deleteItem(item.id)}>{t('knowledge.delete')}</button>
                </span>
              </div>
            </div>
          ))
        )}
        <div style={{ display: 'flex', gap: 8, justifyContent: 'center', marginTop: 12 }}>
          <button className="btn-add" onClick={() => setPage(p => Math.max(0, p - 1))} disabled={page === 0}>Previous</button>
          <button className="btn-add" onClick={() => setPage(p => p + 1)}>Next</button>
        </div>
      </div>
    </div>
  );
}
