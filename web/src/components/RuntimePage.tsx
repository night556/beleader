import { useEffect, useState } from 'react';
import { client } from '../api/client';
import type { Runtime } from '../types';

export function RuntimePage() {
  const [runtimes, setRuntimes] = useState<Runtime[]>([]);

  const load = () => {
    client.listRuntimes().then(setRuntimes).catch(console.error);
  };

  useEffect(() => { load(); }, []);

  const delRuntime = async (id: number) => {
    if (!confirm('Remove this runtime?')) return;
    await client.deleteRuntime(id);
    load();
  };

  const statusColor = (s: string) => s === 'active' ? 'var(--green)' : 'var(--muted)';

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        {/* ── Registered Runtimes ── */}
        <div className="mgmt-page-head">
          <h2 className="mgmt-page-title">Runtimes</h2>
          <button className="mgmt-new-btn" onClick={load}>Refresh</button>
        </div>

        {runtimes.length === 0 ? (
          <div className="mgmt-empty">No runtimes registered. Start a runtime with --gateway-url and --gateway-token.</div>
        ) : (
          runtimes.map(r => (
            <div className="card" key={r.id}>
              <div className="card-header">
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className="status-dot" style={{ background: statusColor(r.status) }} />
                  <span className="card-title">{r.name}</span>
                  <span className="card-badge" style={{ background: statusColor(r.status) + '22', color: statusColor(r.status) }}>
                    {r.status}
                  </span>
                </div>
                <div className="card-actions">
                  <button className="card-btn danger" onClick={() => delRuntime(r.id)}>Remove</button>
                </div>
              </div>
              <div className="card-body">
                <div className="card-kv">
                  <span className="card-kv-key">URL</span>
                  <span className="card-kv-val" style={{ fontFamily: 'var(--font-mono)', fontSize: 12 }}>{r.url}</span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">Workspace</span>
                  <span className="card-kv-val">
                    {r.restrict_workspace
                      ? <span style={{ color: 'var(--amber, #d97706)', fontWeight: 500 }}>Restricted</span>
                      : <span style={{ color: 'var(--green)', fontWeight: 500 }}>Open</span>}
                  </span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">Heartbeat</span>
                  <span className="card-kv-val">{new Date(r.last_heartbeat).toLocaleString()}</span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">Registered</span>
                  <span className="card-kv-val">{new Date(r.created_at).toLocaleString()}</span>
                </div>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  );
}
