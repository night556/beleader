import { useState, useCallback } from 'react';
import { client } from '../api/client';
import type { MCPServer } from '../types';
import { t } from '../i18n';

const EMPTY_FORM = {
  name: '', type: 'stdio' as 'stdio' | 'http', enabled: true,
  command: '', args: '', env: '', url: '', headers: '',
};

export function MCPPage() {
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [search, setSearch] = useState('');
  const [showForm, setShowForm] = useState(false);
  const [editId, setEditId] = useState<number | null>(null);
  const [form, setForm] = useState({ ...EMPTY_FORM });
  const [testing, setTesting] = useState<number | null>(null);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(() => {
    client.listMCPServers().then(s => { setServers(s); setLoaded(true); }).catch(() => {});
  }, []);

  if (!loaded) load();

  const filtered = servers.filter(s =>
    !search || s.name.toLowerCase().includes(search.toLowerCase())
  );

  const openNew = () => {
    setEditId(null);
    setForm({ ...EMPTY_FORM });
    setShowForm(true);
  };

  const openEdit = (s: MCPServer) => {
    setEditId(s.id);
    setForm({
      name: s.name, type: s.type, enabled: s.enabled,
      command: s.command, args: s.args, env: s.env, url: s.url, headers: s.headers,
    });
    setShowForm(true);
  };

  const save = async () => {
    const body: Record<string, unknown> = {
      name: form.name, type: form.type, enabled: form.enabled,
      command: form.command, args: form.args, env: form.env, url: form.url, headers: form.headers,
    };
    if (editId) {
      const updated = await client.updateMCPServer(editId, body);
      setServers(prev => prev.map(s => s.id === editId ? updated : s));
    } else {
      const created = await client.createMCPServer(body);
      setServers(prev => [...prev, created]);
    }
    setShowForm(false);
    load();
  };

  const remove = async (id: number) => {
    if (!confirm('Delete this MCP server?')) return;
    await client.deleteMCPServer(id);
    setServers(prev => prev.filter(s => s.id !== id));
  };

  const testServer = async (id: number) => {
    setTesting(id);
    try {
      const result = await client.testMCPServer(id);
      const msg = result.success
        ? `Success! Found ${result.tool_count} tools: ${result.tools?.join(', ') || 'none'}`
        : `Error: ${result.error || 'Unknown'}`;
      alert(msg);
    } catch (e: any) {
      alert('Test failed: ' + (e.message || ''));
    } finally {
      setTesting(null);
    }
  };

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <div className="mgmt-page-head">
          <h2 className="mgmt-page-title">{t('mcp.title')}</h2>
          <button className="mgmt-new-btn" onClick={openNew}>{t('mcp.new')}</button>
        </div>

        <input
          className="mgmt-search"
          placeholder={t('mcp.search_placeholder')}
          value={search}
          onChange={e => setSearch(e.target.value)}
        />

        {filtered.length === 0 ? (
          <div className="mgmt-empty">No MCP servers.</div>
        ) : (
          filtered.map(s => (
            <div className="card" key={s.id}>
              <div className="card-header">
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className={`mcp-status-dot mcp-status-${s.status}`} />
                  <div>
                    <div className="card-title" style={{ fontSize: 13 }}>{s.name}</div>
                    <div className="card-subtitle">{s.type === 'stdio' ? 'STDIO' : 'HTTP'}</div>
                  </div>
                  <span className={`card-badge ${s.status === 'connected' ? 'connected' : s.status === 'error' ? 'error' : 'disconnected'}`}>
                    {s.status}
                  </span>
                </div>
                <div className="card-actions">
                  <button className="card-btn primary" onClick={() => testServer(s.id)} disabled={testing === s.id}>
                    {testing === s.id ? '...' : t('mcp.test')}
                  </button>
                  <button className="card-btn" onClick={() => openEdit(s)}>{t('mcp.edit')}</button>
                  <button className="card-btn danger" onClick={() => remove(s.id)}>{t('mcp.delete')}</button>
                </div>
              </div>
              {s.error && <div className="mcp-server-error">{s.error}</div>}
            </div>
          ))
        )}

        {showForm && (
          <div className="modal-backdrop" onClick={e => { if (e.target === e.currentTarget) setShowForm(false); }}>
            <div className="modal-dialog wide">
              <div className="modal-head">
                <h3>{editId ? 'Edit MCP Server' : 'New MCP Server'}</h3>
                <button className="modal-close" onClick={() => setShowForm(false)}>✕</button>
              </div>
              <div className="modal-body">
                <div className="form-group">
                  <label className="form-label">Name</label>
                  <input className="form-input" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} />
                </div>
                <div className="form-group">
                  <label className="form-label">Type</label>
                  <select className="form-select" value={form.type} onChange={e => setForm({ ...form, type: e.target.value as 'stdio' | 'http' })}>
                    <option value="stdio">STDIO</option>
                    <option value="http">HTTP</option>
                  </select>
                </div>
                {form.type === 'stdio' ? (
                  <>
                    <div className="form-group">
                      <label className="form-label">Command</label>
                      <input className="form-input" value={form.command} onChange={e => setForm({ ...form, command: e.target.value })} />
                    </div>
                    <div className="form-group">
                      <label className="form-label">Args</label>
                      <input className="form-input" value={form.args} onChange={e => setForm({ ...form, args: e.target.value })} />
                      <div className="form-hint">Space-separated arguments</div>
                    </div>
                    <div className="form-group">
                      <label className="form-label">Environment (KEY=VALUE, one per line)</label>
                      <textarea className="form-textarea" value={form.env} onChange={e => setForm({ ...form, env: e.target.value })} rows={4} />
                    </div>
                  </>
                ) : (
                  <>
                    <div className="form-group">
                      <label className="form-label">URL</label>
                      <input className="form-input" value={form.url} onChange={e => setForm({ ...form, url: e.target.value })} />
                    </div>
                    <div className="form-group">
                      <label className="form-label">Headers (KEY=VALUE, one per line)</label>
                      <textarea className="form-textarea" value={form.headers} onChange={e => setForm({ ...form, headers: e.target.value })} rows={4} />
                    </div>
                  </>
                )}
              </div>
              <div className="modal-foot">
                <button className="modal-btn" onClick={() => setShowForm(false)}>{t('modal.cancel')}</button>
                <button className="modal-btn primary" onClick={save}>{t('agents.save')}</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
