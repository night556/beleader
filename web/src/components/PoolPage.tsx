import { useEffect, useState } from 'react';
import { client } from '../api/client';
import type { Pool, ToolAgent } from '../types';

export function PoolPage() {
  const [pools, setPools] = useState<Pool[]>([]);
  const [agents, setAgents] = useState<ToolAgent[]>([]);

  const load = () => {
    client.listPools().then(setPools).catch(console.error);
    client.listToolAgents().then(setAgents).catch(console.error);
  };

  useEffect(() => { load(); }, []);

  const delAgent = async (id: number) => {
    if (!confirm('Remove this tool agent?')) return;
    await client.deleteToolAgent(id);
    load();
  };

  const delPool = async (id: number) => {
    if (!confirm('Delete this pool? Tool agents must be removed first.')) return;
    await client.deletePool(id);
    load();
  };

  const statusColor = (s: string) => s === 'active' ? 'var(--green)' : 'var(--muted)';

  const agentsByPool = (poolId: number) => agents.filter(a => a.pool_id === poolId);

  // Tool count from tool_defs
  const toolCount = (pool: Pool) => {
    try {
      const defs = JSON.parse(pool.tool_defs || '[]');
      return Array.isArray(defs) ? defs.length : 0;
    } catch { return 0; }
  };

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <div className="mgmt-page-head">
          <h2 className="mgmt-page-title">Pools & Tool Agents</h2>
          <button className="mgmt-new-btn" onClick={load}>Refresh</button>
        </div>

        {pools.length === 0 ? (
          <div className="mgmt-empty">No pools registered. Start a tool-agent with --gateway-url and --gateway-token.</div>
        ) : (
          pools.map(p => {
            const poolAgents = agentsByPool(p.id);
            return (
              <div className="card" key={p.id}>
                <div className="card-header">
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <span className="card-title">{p.name}</span>
                    {p.is_default && <span className="card-badge" style={{ background: 'var(--blue, #3b82f6)22', color: 'var(--blue, #3b82f6)' }}>default</span>}
                    <span className="card-badge" style={{ background: 'var(--muted)22', color: 'var(--muted)' }}>
                      {poolAgents.length} agent{poolAgents.length !== 1 ? 's' : ''}
                    </span>
                    <span className="card-badge" style={{ background: 'var(--muted)22', color: 'var(--muted)' }}>
                      {toolCount(p)} tools
                    </span>
                  </div>
                  <div className="card-actions">
                    <button className="card-btn danger" onClick={() => delPool(p.id)}>Delete</button>
                  </div>
                </div>
                <div className="card-body">
                  <div className="card-kv">
                    <span className="card-kv-key">Platform</span>
                    <span className="card-kv-val">{p.platform || '—'}</span>
                  </div>
                  <div className="card-kv">
                    <span className="card-kv-key">Shell</span>
                    <span className="card-kv-val">{p.shell || '—'}</span>
                  </div>
                  <div className="card-kv">
                    <span className="card-kv-key">Workspace</span>
                    <span className="card-kv-val">
                      {p.workspace_root || '—'}
                      {p.restrict_workspace
                        ? <span style={{ color: 'var(--amber, #d97706)', fontWeight: 500, marginLeft: 8 }}>Restricted</span>
                        : <span style={{ color: 'var(--green)', fontWeight: 500, marginLeft: 8 }}>Open</span>}
                    </span>
                  </div>

                  {poolAgents.length > 0 && (
                    <div style={{ marginTop: 12, borderTop: '1px solid var(--border)', paddingTop: 12 }}>
                      <div style={{ fontSize: 12, fontWeight: 600, color: 'var(--muted)', marginBottom: 8, textTransform: 'uppercase', letterSpacing: 0.5 }}>
                        Tool Agents
                      </div>
                      {poolAgents.map(a => (
                        <div key={a.id} style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 0' }}>
                          <span className="status-dot" style={{ background: statusColor(a.status) }} />
                          <span style={{ fontWeight: 500 }}>{a.name}</span>
                          <span style={{ fontFamily: 'var(--font-mono)', fontSize: 12, color: 'var(--muted)' }}>{a.url}</span>
                          <span style={{ color: 'var(--muted)', fontSize: 12 }}>{new Date(a.last_heartbeat).toLocaleTimeString()}</span>
                          <button className="card-btn danger" style={{ marginLeft: 'auto', padding: '2px 8px', fontSize: 11 }} onClick={() => delAgent(a.id)}>Remove</button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
