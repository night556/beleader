import { useState, useEffect } from 'react';
import { client, getAPIKey, clearAPIKey } from '../api/client';

export function ConsolePage() {
  const [dashboard, setDashboard] = useState<any>(null);
  const [keys, setKeys] = useState<any[]>([]);
  const [usage, setUsage] = useState<any>(null);
  const [keyName, setKeyName] = useState('');
  const [keyScope, setKeyScope] = useState('api');

  const load = () => {
    client.getDashboard().then(setDashboard).catch(console.error);
    client.listKeys().then(setKeys).catch(console.error);
    client.getUsage().then(setUsage).catch(console.error);
  };

  useEffect(() => { load(); }, []);

  const createKey = async () => {
    if (!keyName) return;
    const k = await client.createKey({ name: keyName, scope: keyScope });
    setKeys([...keys, k]);
    setKeyName('');
  };

  const deleteKey = async (id: number) => {
    await client.deleteKey(id);
    setKeys(keys.filter(k => k.id !== id));
  };

  const logout = () => {
    clearAPIKey();
    window.location.reload();
  };

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <h2 className="mgmt-page-title">Console</h2>
          <button className="card-btn" onClick={logout}>Logout</button>
        </div>

        {dashboard && (
          <div style={{ display: 'flex', gap: 16, marginBottom: 24 }}>
            <div className="card" style={{ flex: 1, padding: 16 }}>
              <div style={{ fontSize: 12, color: 'var(--muted)' }}>Balance</div>
              <div style={{ fontSize: 24, fontWeight: 700 }}>${dashboard.tenant?.balance?.toFixed(2) || '0.00'}</div>
            </div>
            <div className="card" style={{ flex: 1, padding: 16 }}>
              <div style={{ fontSize: 12, color: 'var(--muted)' }}>Tokens (24h)</div>
              <div style={{ fontSize: 24, fontWeight: 700 }}>{dashboard.tokens_24h?.toLocaleString() || '0'}</div>
            </div>
            <div className="card" style={{ flex: 1, padding: 16 }}>
              <div style={{ fontSize: 12, color: 'var(--muted)' }}>API Key</div>
              <div style={{ fontSize: 11, fontFamily: 'monospace' }}>{getAPIKey().slice(0, 16)}...</div>
            </div>
          </div>
        )}

        <h3>API Keys</h3>
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <input className="form-input" placeholder="Key name" value={keyName} onChange={e => setKeyName(e.target.value)} />
          <select className="form-select" value={keyScope} onChange={e => setKeyScope(e.target.value)}>
            <option value="api">API</option>
            <option value="console">Console</option>
          </select>
          <button className="mgmt-new-btn" onClick={createKey}>Create</button>
        </div>

        {keys.map(k => (
          <div key={k.id} className="card" style={{ padding: '8px 12px', marginBottom: 4, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <div>
              <div style={{ fontSize: 13, fontWeight: 600 }}>{k.name || 'Unnamed'}</div>
              <code style={{ fontSize: 11 }}>{k.key}</code>
              <span style={{ fontSize: 10, color: 'var(--muted)', marginLeft: 8 }}>({k.scope})</span>
            </div>
            <button className="card-btn danger" onClick={() => deleteKey(k.id)}>Revoke</button>
          </div>
        ))}

        {usage && (
          <div style={{ marginTop: 24 }}>
            <h3>Usage (30d): {usage.total_tokens?.toLocaleString()} tokens</h3>
          </div>
        )}
      </div>
    </div>
  );
}