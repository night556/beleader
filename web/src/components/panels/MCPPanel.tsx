import { useState, useEffect } from 'react';
import { client } from '../../api/client';
import { t } from '../../i18n';
import type { MCPServer } from '../../types';

interface Props { onClose: () => void }

export function MCPPanel({ onClose }: Props) {
  const [servers, setServers] = useState<MCPServer[]>([]);
  const [search, setSearch] = useState('');
  const [editing, setEditing] = useState<MCPServer | null>(null);

  useEffect(() => { loadServers(); }, []);

  const loadServers = () => {
    client.listMCPServers().then(setServers).catch(() => {});
  };

  const filtered = servers.filter(s =>
    !search || s.name.toLowerCase().includes(search.toLowerCase())
  );

  const deleteServer = (id: number) => {
    if (!confirm('Delete this MCP server?')) return;
    client.deleteMCPServer(id).then(loadServers).catch(() => {});
  };

  if (editing !== undefined && editing !== null) {
    return <MCPEditor server={editing} onClose={() => setEditing(null)} onSaved={() => { loadServers(); setEditing(null); }} />;
  }

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{t('mcp.title')}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <input
            style={{ flex: 1, padding: '6px 8px', border: '1px solid var(--border)', borderRadius: 6, background: 'var(--bg3)', color: 'var(--text)', fontSize: 12 }}
            placeholder={t('mcp.search_placeholder')}
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <button className="btn-add" onClick={() => setEditing({} as MCPServer)}>{t('mcp.new')}</button>
        </div>
        {filtered.length === 0 ? (
          <div className="agents-empty">No MCP servers configured</div>
        ) : (
          filtered.map(s => {
            const statusCls = s.status === 'connected' ? 'mcp-status-connected' : s.status === 'error' ? 'mcp-status-error' : 'mcp-status-disconnected';
            const statusLabel = s.status === 'connected' ? 'Connected' : s.status === 'error' ? 'Error' : 'Disconnected';
            return (
              <div key={s.id} className="mcp-server-card">
                <div className="mcp-server-header">
                  <span className={`mcp-status-dot ${statusCls}`} title={statusLabel} />
                  <div className="mcp-server-info">
                    <span className="mcp-server-name">{s.name}</span>
                    <span className="mcp-server-type">{s.type}</span>
                  </div>
                  <div className="mcp-server-actions">
                    {(s.status === 'disconnected' || s.status === 'error') && (
                      <button className="mcp-btn mcp-btn-connect" onClick={() => client.connectMCPServer(s.id).then(loadServers)}>{t('mcp.connect')}</button>
                    )}
                    {s.status === 'connected' && (
                      <button className="mcp-btn mcp-btn-disconnect" onClick={() => client.disconnectMCPServer(s.id).then(loadServers)}>{t('mcp.disconnect')}</button>
                    )}
                    <button className="mcp-btn mcp-btn-test" onClick={() => {
                      client.testMCPServer(s.id).then(d => {
                        if (d.success) alert(`Connection OK\nTools: ${d.tool_count}\n${d.tools?.map(t => `• ${t}`).join('\n') || ''}`);
                        else alert('Test failed: ' + (d.error || 'unknown'));
                      }).catch(err => alert('Test error: ' + err));
                    }}>{t('mcp.test')}</button>
                    <button className="mcp-btn mcp-btn-edit" onClick={() => setEditing(s)}>{t('mcp.edit')}</button>
                    <button className="mcp-btn mcp-btn-del" onClick={() => deleteServer(s.id)}>{t('mcp.delete')}</button>
                  </div>
                </div>
                {s.error && <div className="mcp-server-error">{s.error}</div>}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}

// ── MCP Editor (inline) ──

function MCPEditor({ server, onClose, onSaved }: { server: MCPServer | null; onClose: () => void; onSaved: () => void }) {
  const isNew = !server || !server.id;
  const [name, setName] = useState(server?.name || '');
  const [srvType, setSrvType] = useState<'stdio' | 'http'>(server?.type || 'stdio');
  const [enabled, setEnabled] = useState(server?.enabled ?? true);
  const [command, setCommand] = useState(server?.command || '');
  const [args, setArgs] = useState(() => { try { return server?.args ? JSON.parse(server.args).join(' ') : ''; } catch { return server?.args || ''; } });
  const [url, setUrl] = useState(server?.url || '');
  const [envEntries, setEnvEntries] = useState<{ key: string; val: string }[]>(() => {
    try { const obj = JSON.parse(server?.env || '{}'); return Object.entries(obj).map(([k, v]) => ({ key: k, val: v as string })); } catch { return []; }
  });
  const [headerEntries, setHeaderEntries] = useState<{ key: string; val: string }[]>(() => {
    try { const obj = JSON.parse(server?.headers || '{}'); return Object.entries(obj).map(([k, v]) => ({ key: k, val: v as string })); } catch { return []; }
  });

  const collectKV = (entries: { key: string; val: string }[]) => {
    const obj: Record<string, string> = {};
    for (const e of entries) { if (e.key) obj[e.key] = e.val; }
    return JSON.stringify(obj);
  };

  const save = async () => {
    if (!name) { alert('Name required'); return; }
    const payload: Record<string, unknown> = { name, type: srvType, enabled };
    if (srvType === 'stdio') {
      payload.command = command;
      payload.args = JSON.stringify(args.split(/\s+/).filter(Boolean));
      payload.env = collectKV(envEntries);
    } else {
      payload.url = url;
      payload.headers = collectKV(headerEntries);
    }
    try {
      if (isNew) await client.createMCPServer(payload);
      else await client.updateMCPServer(server!.id, payload);
      onSaved();
    } catch (err: unknown) {
      console.error('save MCP error:', err);
    }
  };

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{isNew ? 'New MCP Server' : 'Edit MCP Server'}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        <div className="modal-field">
          <label>Name</label>
          <input className="modal-input" value={name} onChange={e => setName(e.target.value)} placeholder="e.g. my-server" />
        </div>
        <div className="modal-field" style={{ display: 'flex', gap: 12 }}>
          <div style={{ flex: 1 }}>
            <label>Type</label>
            <select className="modal-select" value={srvType} onChange={e => setSrvType(e.target.value as 'stdio' | 'http')}>
              <option value="stdio">stdio</option>
              <option value="http">HTTP</option>
            </select>
          </div>
          <div>
            <label>&nbsp;</label>
            <label style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 12 }}>
              <input type="checkbox" checked={enabled} onChange={e => setEnabled(e.target.checked)} /> Enable
            </label>
          </div>
        </div>

        {srvType === 'stdio' && (
          <>
            <div className="modal-field">
              <label>Command</label>
              <input className="modal-input" value={command} onChange={e => setCommand(e.target.value)} placeholder="e.g. npx or python" />
            </div>
            <div className="modal-field">
              <label>Args</label>
              <input className="modal-input" value={args} onChange={e => setArgs(e.target.value)} placeholder="e.g. -y @modelcontextprotocol/server-filesystem /tmp" />
              <div className="modal-field-hint">Space-separated arguments</div>
            </div>
            <div className="modal-field">
              <label>Environment Variables</label>
              <KVEditor entries={envEntries} onChange={setEnvEntries} />
            </div>
          </>
        )}

        {srvType === 'http' && (
          <>
            <div className="modal-field">
              <label>URL</label>
              <input className="modal-input" value={url} onChange={e => setUrl(e.target.value)} placeholder="https://example.com/mcp" />
            </div>
            <div className="modal-field">
              <label>Headers</label>
              <KVEditor entries={headerEntries} onChange={setHeaderEntries} />
            </div>
          </>
        )}

        <button className="btn-save" onClick={save}>Save</button>
      </div>
    </div>
  );
}

function KVEditor({ entries, onChange }: { entries: { key: string; val: string }[]; onChange: (e: { key: string; val: string }[]) => void }) {
  const update = (i: number, field: 'key' | 'val', value: string) => {
    const next = entries.map((e, j) => j === i ? { ...e, [field]: value } : e);
    onChange(next);
  };
  const remove = (i: number) => onChange(entries.filter((_, j) => j !== i));
  const add = () => onChange([...entries, { key: '', val: '' }]);

  return (
    <div>
      <div className="mcp-kv-list">
        {entries.map((e, i) => (
          <div key={i} className="mcp-kv-row">
            <input placeholder="Key" value={e.key} onChange={ev => update(i, 'key', ev.target.value)} />
            <input placeholder="Value" value={e.val} onChange={ev => update(i, 'val', ev.target.value)} />
            <button className="mcp-kv-remove" onClick={() => remove(i)}>×</button>
          </div>
        ))}
      </div>
      <button className="mcp-kv-add" onClick={add}>+ Add</button>
    </div>
  );
}
