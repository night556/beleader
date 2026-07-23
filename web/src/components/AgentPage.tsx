import { useState, useEffect } from 'react';
import { useAppState } from '../context/AppContext';
import { client } from '../api/client';
import type { Agent, MCPServer } from '../types';
import { t } from '../i18n';

export function AgentPage() {
  const { state, dispatch } = useAppState();
  const { agents, tools, models } = state;
  const [search, setSearch] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState({ name: '', desc: '', system_prompt: '', tools: '[]', default_model_id: '', mcp_servers: '[]', worker_agents: '[]' });
  const [selectedTools, setSelectedTools] = useState<string[]>([]);
  const [selectedMCPServers, setSelectedMCPServers] = useState<string[]>([]);
  const [selectedWorkers, setSelectedWorkers] = useState<string[]>([]);
  const [toolSearch, setToolSearch] = useState('');
  const [mcpServers, setMCPServers] = useState<MCPServer[]>([]);

  const filtered = agents.filter(a =>
    !search || a.name.toLowerCase().includes(search.toLowerCase())
  );

  const openNew = () => {
    setEditId(null);
    setForm({ name: '', desc: '', system_prompt: '', tools: '[]', default_model_id: '', mcp_servers: '[]', worker_agents: '[]' });
    setSelectedTools([]);
    setSelectedMCPServers([]);
    setSelectedWorkers([]);
    setShowForm(true);
  };

  const openEdit = (a: Agent) => {
    setEditId(a.id);
    setForm({ name: a.name, desc: a.desc, system_prompt: a.system_prompt, tools: a.tools, default_model_id: a.default_model_id || '', mcp_servers: a.mcp_servers || '[]', worker_agents: a.worker_agents || '[]' });
    try {
      setSelectedTools(JSON.parse(a.tools || '[]'));
    } catch {
      setSelectedTools([]);
    }
    try {
      setSelectedMCPServers(JSON.parse(a.mcp_servers || '[]'));
    } catch {
      setSelectedMCPServers([]);
    }
    try {
      setSelectedWorkers(JSON.parse(a.worker_agents || '[]'));
    } catch {
      setSelectedWorkers([]);
    }
    setShowForm(true);
  };

  const save = async () => {
    if (!form.name.trim()) { alert('Name is required'); return; }
    if (!form.system_prompt.trim()) { alert('System prompt is required'); return; }
    const body = { ...form, tools: JSON.stringify(selectedTools), mcp_servers: JSON.stringify(selectedMCPServers), worker_agents: JSON.stringify(selectedWorkers) };
    try {
      if (editId) {
        const updated = await client.updateAgent(editId, body);
        dispatch({ type: 'SET_AGENTS', agents: agents.map(a => a.id === editId ? updated : a) });
      } else {
        const created = await client.createAgent(body);
        dispatch({ type: 'SET_AGENTS', agents: [...agents, created] });
      }
      setShowForm(false);
    } catch (err: any) {
      alert('Failed to save agent: ' + (err?.message || 'Unknown error'));
    }
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

  const toggleMCPServer = (name: string) => {
    setSelectedMCPServers(prev =>
      prev.includes(name) ? prev.filter(n => n !== name) : [...prev, name]
    );
  };

  const filteredTools = tools.filter(t =>
    !toolSearch || t.name.toLowerCase().includes(toolSearch.toLowerCase())
  );

  useEffect(() => {
    client.listTools().then(t => dispatch({ type: 'SET_TOOLS', tools: t })).catch(() => {});
    client.listMCPServers().then(s => setMCPServers(s)).catch(() => {});
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
                let mcpNames: string[] = [];
                try { mcpNames = JSON.parse(a.mcp_servers || '[]'); } catch {}
                if (toolNames.length === 0 && mcpNames.length === 0) return null;
                return (
                  <div className="card-chips">
                    {toolNames.map(tn => (
                      <span key={tn} className="card-chip">{tn}</span>
                    ))}
                    {mcpNames.map(mn => (
                      <span key={`mcp_${mn}`} className="card-chip mcp-chip">mcp: {mn}</span>
                    ))}
                  </div>
                );
              })()}
            </div>
          ))
        )}

        {showForm && (
          <div className="modal-backdrop">
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
                  <label className="form-label">Default Model</label>
                  <select
                    className="form-select"
                    value={form.default_model_id}
                    onChange={e => setForm({ ...form, default_model_id: e.target.value })}
                  >
                    <option value="">— First available —</option>
                    {models.map(m => (
                      <option key={m.id} value={m.id}>{m.id}</option>
                    ))}
                  </select>
                </div>
                <div className="form-group">
                  <label className="form-label">MCP Servers</label>
                  {mcpServers.length === 0 ? (
                    <span className="form-hint">No MCP servers configured. Add them in the MCP page.</span>
                  ) : (
                    <div className="tools-chips">
                      {selectedMCPServers.map(mn => (
                        <span key={mn} className="tool-chip">
                          {mn}
                          <button className="tool-chip-remove" onClick={() => toggleMCPServer(mn)}>×</button>
                        </span>
                      ))}
                      {selectedMCPServers.length === 0 && <span className="form-hint">None selected</span>}
                    </div>
                  )}
                  {mcpServers.length > 0 && (
                    <div className="tools-picker">
                      {mcpServers.map(s => (
                        <div
                          key={s.name}
                          className={`tool-pick-item ${selectedMCPServers.includes(s.name) ? 'selected' : ''}`}
                          onClick={() => toggleMCPServer(s.name)}
                        >
                          <span className="tool-pick-name">
                            {s.name}
                            <span className="tool-source-badge mcp">{s.type}</span>
                          </span>
                          <span className="tool-pick-desc">{s.status}</span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
                <div className="form-group">
                  <label className="form-label">Worker Agents</label>
                  <div className="tools-chips">
                    {selectedWorkers.map(wn => (
                      <span key={wn} className="tool-chip">
                        {wn}
                        <button className="tool-chip-remove" onClick={() => setSelectedWorkers(prev => prev.filter(n => n !== wn))}>×</button>
                      </span>
                    ))}
                    {selectedWorkers.length === 0 && <span className="form-hint">No workers selected</span>}
                  </div>
                  <div className="tools-picker">
                    {agents.filter(a => a.id !== editId).map(a => (
                      <div
                        key={a.id}
                        className={`tool-pick-item ${selectedWorkers.includes(a.name) ? 'selected' : ''}`}
                        onClick={() => setSelectedWorkers(prev => prev.includes(a.name) ? prev.filter(n => n !== a.name) : [...prev, a.name])}
                      >
                        <span className="tool-pick-name">{a.name}</span>
                        <span className="tool-pick-desc">{a.desc}</span>
                      </div>
                    ))}
                  </div>
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
