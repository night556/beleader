import { useState, useEffect } from 'react';
import { useAppState } from '../context/AppContext';
import { client } from '../api/client';
import type { Agent } from '../types';
import { t } from '../i18n';

export function AgentPage() {
  const { state, dispatch } = useAppState();
  const { agents, tools } = state;
  const [search, setSearch] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState({ name: '', desc: '', system_prompt: '', tools: '[]' });
  const [selectedTools, setSelectedTools] = useState<string[]>([]);
  const [toolSearch, setToolSearch] = useState('');

  const filtered = agents.filter(a =>
    !search || a.name.toLowerCase().includes(search.toLowerCase())
  );

  const openNew = () => {
    setEditId(null);
    setForm({ name: '', desc: '', system_prompt: '', tools: '[]' });
    setSelectedTools([]);
    setShowForm(true);
  };

  const openEdit = (a: Agent) => {
    setEditId(a.id);
    setForm({ name: a.name, desc: a.desc, system_prompt: a.system_prompt, tools: a.tools });
    try {
      setSelectedTools(JSON.parse(a.tools || '[]'));
    } catch {
      setSelectedTools([]);
    }
    setShowForm(true);
  };

  const save = async () => {
    const body = { ...form, tools: JSON.stringify(selectedTools) };
    if (editId) {
      const updated = await client.updateAgent(editId, body);
      dispatch({ type: 'SET_AGENTS', agents: agents.map(a => a.id === editId ? updated : a) });
    } else {
      const created = await client.createAgent(body);
      dispatch({ type: 'SET_AGENTS', agents: [...agents, created] });
    }
    setShowForm(false);
  };

  const remove = async (id: number) => {
    if (!confirm(t('agents.delete_confirm', { $1: agents.find(a => a.id === id)?.name || '' }))) return;
    await client.deleteAgent(id);
    dispatch({ type: 'SET_AGENTS', agents: agents.filter(a => a.id !== id) });
  };

  const toggleTool = (name: string) => {
    setSelectedTools(prev =>
      prev.includes(name) ? prev.filter(t => t !== name) : [...prev, name]
    );
  };

  const filteredTools = tools.filter(t =>
    !toolSearch || t.name.toLowerCase().includes(toolSearch.toLowerCase())
  );

  useEffect(() => {
    client.listTools().then(t => dispatch({ type: 'SET_TOOLS', tools: t })).catch(() => {});
  }, []);

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <div className="mgmt-page-head">
          <h2 className="mgmt-page-title">{t('agents.title')}</h2>
          <button className="mgmt-new-btn" onClick={openNew}>{t('agents.new')}</button>
        </div>

        <input
          className="mgmt-search"
          placeholder={t('agents.search_placeholder')}
          value={search}
          onChange={e => setSearch(e.target.value)}
        />

        {filtered.length === 0 ? (
          <div className="mgmt-empty">{t('agents.empty')}</div>
        ) : (
          filtered.map(a => (
            <div className="card" key={a.id}>
              <div className="card-header">
                <div>
                  <div className="card-title">{a.name}</div>
                  <div className="card-subtitle">{a.desc}</div>
                </div>
                <div className="card-actions">
                  <button className="card-btn" onClick={() => openEdit(a)}>{t('agents.edit')}</button>
                  <button className="card-btn danger" onClick={() => remove(a.id)}>{t('agents.delete')}</button>
                </div>
              </div>
              {(() => {
                let toolNames: string[] = [];
                try { toolNames = JSON.parse(a.tools || '[]'); } catch {}
                if (toolNames.length === 0) return null;
                return (
                  <div className="card-chips">
                    {toolNames.map(tn => (
                      <span key={tn} className="card-chip">{tn}</span>
                    ))}
                  </div>
                );
              })()}
            </div>
          ))
        )}

        {showForm && (
          <div className="modal-backdrop" onClick={e => { if (e.target === e.currentTarget) setShowForm(false); }}>
            <div className="modal-dialog wide">
              <div className="modal-head">
                <h3>{editId ? t('agents.edit_title') : t('agents.new_title')}</h3>
                <button className="modal-close" onClick={() => setShowForm(false)}>✕</button>
              </div>
              <div className="modal-body">
                <div className="form-group">
                  <label className="form-label">{t('agents.name')}</label>
                  <input className="form-input" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
                </div>
                <div className="form-group">
                  <label className="form-label">{t('agents.desc')}</label>
                  <input className="form-input" value={form.desc} onChange={e => setForm({ ...form, desc: e.target.value })} />
                </div>
                <div className="form-group">
                  <label className="form-label">{t('agents.system_prompt')}</label>
                  <textarea className="form-textarea" value={form.system_prompt} onChange={e => setForm({ ...form, system_prompt: e.target.value })} />
                </div>
                <div className="form-group">
                  <label className="form-label">{t('agents.tools')}</label>
                  <div className="tools-chips">
                    {selectedTools.map(tn => (
                      <span key={tn} className="tool-chip">
                        {tn}
                        <button className="tool-chip-remove" onClick={() => toggleTool(tn)}>×</button>
                      </span>
                    ))}
                    {selectedTools.length === 0 && <span className="form-hint">{t('agents.no_tools')}</span>}
                  </div>
                  <input
                    className="form-input"
                    placeholder={t('agents.tools_search')}
                    value={toolSearch}
                    onChange={e => setToolSearch(e.target.value)}
                    style={{ marginBottom: 6 }}
                  />
                  <div className="tools-picker">
                    {filteredTools.map(t => (
                      <div
                        key={t.name}
                        className={`tool-pick-item ${selectedTools.includes(t.name) ? 'selected' : ''}`}
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
                </div>
              </div>
              <div className="modal-foot">
                <button className="modal-btn" onClick={() => setShowForm(false)}>{t('modal.cancel')}</button>
                {editId && <button className="modal-btn danger" onClick={() => { remove(editId); setShowForm(false); }}>{t('agents.delete_btn')}</button>}
                <button className="modal-btn primary" onClick={save}>{t('agents.save')}</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
