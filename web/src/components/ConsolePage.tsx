import { useState, useEffect } from 'react';
import { client, getAPIKey, clearAPIKey } from '../api/client';

export function ConsolePage() {
  const [dashboard, setDashboard] = useState<any>(null);
  const [keys, setKeys] = useState<any[]>([]);
  const [usage, setUsage] = useState<any>(null);
  const [tab, setTab] = useState<'overview' | 'keys' | 'tokens'>('overview');
  const [keyName, setKeyName] = useState('');
  const [keyScope, setKeyScope] = useState('api');
  const [tokenUserID, setTokenUserID] = useState('');
  const [tokenExpiry, setTokenExpiry] = useState('24');
  const [generatedToken, setGeneratedToken] = useState('');

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

  const generateToken = async () => {
    if (!tokenUserID) return;
    const r = await fetch(`/api/console/generate-token`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${getAPIKey()}` },
      body: JSON.stringify({ user_id: tokenUserID, expiry: parseInt(tokenExpiry) }),
    });
    const data = await r.json();
    setGeneratedToken(data.token);
  };

  const rotateSecret = async () => {
    if (!confirm('Rotate signing key? All existing user tokens will be invalidated.')) return;
    await fetch(`/api/console/rotate-secret`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${getAPIKey()}` }
    });
    load();
  };

  const logout = () => { clearAPIKey(); window.location.reload(); };

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <div className="mgmt-page-head">
          <h2 className="mgmt-page-title">Console</h2>
          <button className="card-btn" onClick={logout}>Sign Out</button>
        </div>

        {dashboard && (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(200px, 1fr))', gap: 12, marginBottom: 24 }}>
            <div className="card" style={{ padding: 20 }}>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 4 }}>Balance</div>
              <div style={{ fontSize: 28, fontWeight: 700 }}>${dashboard.tenant?.balance?.toFixed(2) || '0.00'}</div>
            </div>
            <div className="card" style={{ padding: 20 }}>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 4 }}>Tokens (24h)</div>
              <div style={{ fontSize: 28, fontWeight: 700 }}>{dashboard.tokens_24h?.toLocaleString() || '0'}</div>
            </div>
            <div className="card" style={{ padding: 20 }}>
              <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 4 }}>Quota</div>
              <div style={{ fontSize: 28, fontWeight: 700 }}>{dashboard.tenant?.quota_tokens?.toLocaleString() || '∞'}</div>
            </div>
          </div>
        )}

        <div style={{ display: 'flex', gap: 0, marginBottom: 20, borderBottom: '1px solid var(--border)' }}>
          {(['overview', 'keys', 'tokens'] as const).map(t => (
            <button key={t} onClick={() => setTab(t)} style={{
              padding: '10px 20px', border: 'none', background: 'none',
              borderBottom: tab === t ? '2px solid var(--accent)' : '2px solid transparent',
              color: tab === t ? 'var(--accent)' : 'var(--muted)',
              fontWeight: tab === t ? 600 : 400, fontSize: 14, cursor: 'pointer',
            }}>
              {t === 'overview' ? 'Overview' : t === 'keys' ? 'API Keys' : 'User Tokens'}
            </button>
          ))}
        </div>

        {tab === 'overview' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div className="card" style={{ padding: 20, background: 'var(--wash)' }}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                <div>
                  <h4 style={{ margin: '0 0 4px' }}>Signing Key</h4>
                  <div style={{ fontSize: 12, color: 'var(--muted)' }}>Used to sign user tokens. Keep this secret.</div>
                </div>
                <button className="card-btn" onClick={rotateSecret}>Rotate</button>
              </div>
              <code style={{ fontSize: 12, wordBreak: 'break-all', display: 'block', padding: '10px', background: 'var(--surface)', borderRadius: 6, border: '1px solid var(--border)' }}>
                {dashboard?.signing_key || 'Loading...'}
              </code>
            </div>
          </div>
        )}

        {tab === 'keys' && (
          <div>
            <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
              <input className="form-input" placeholder="Key name" value={keyName}
                onChange={e => setKeyName(e.target.value)} style={{ flex: 1 }} />
              <select className="form-select" value={keyScope}
                onChange={e => setKeyScope(e.target.value)}>
                <option value="api">API</option>
                <option value="console">Console</option>
              </select>
              <button className="card-btn primary" onClick={createKey}>Create</button>
            </div>
            {keys.map(k => (
              <div key={k.id} className="card" style={{
                padding: '12px 16px', marginBottom: 8,
                display: 'flex', justifyContent: 'space-between', alignItems: 'center'
              }}>
                <div>
                  <div style={{ fontWeight: 600, fontSize: 14 }}>{k.name || 'Unnamed'}</div>
                  <code style={{ fontSize: 12 }}>{k.key}</code>
                  <span style={{ fontSize: 11, color: 'var(--muted)', marginLeft: 8 }}>{k.scope}</span>
                </div>
                <button className="card-btn danger" onClick={() => deleteKey(k.id)}>Revoke</button>
              </div>
            ))}
            {keys.length === 0 && (
              <div style={{ color: 'var(--muted)', textAlign: 'center', padding: 40 }}>No API keys yet</div>
            )}
          </div>
        )}

        {tab === 'tokens' && (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
            <div className="card" style={{ padding: 20 }}>
              <h4 style={{ margin: '0 0 12px' }}>Generate User Token</h4>
              <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                <input className="form-input" placeholder="User ID" value={tokenUserID}
                  onChange={e => setTokenUserID(e.target.value)} style={{ flex: 1 }} />
                <input className="form-input" type="number" placeholder="Hours" value={tokenExpiry}
                  onChange={e => setTokenExpiry(e.target.value)} style={{ width: 80 }} />
                <button className="card-btn primary" onClick={generateToken}>Generate</button>
              </div>
              {generatedToken && (
                <div style={{ padding: 12, background: 'var(--surface)', borderRadius: 6, border: '1px solid var(--border)' }}>
                  <div style={{ fontSize: 11, color: 'var(--muted)', marginBottom: 4 }}>Token (share with user):</div>
                  <code style={{ fontSize: 11, wordBreak: 'break-all' }}>{generatedToken}</code>
                </div>
              )}
            </div>

            <div className="card" style={{ padding: 20 }}>
              <h4 style={{ margin: '0 0 8px' }}>Usage (30d)</h4>
              <div style={{ fontSize: 28, fontWeight: 700 }}>{usage?.total_tokens?.toLocaleString() || '0'}</div>
              <div style={{ fontSize: 12, color: 'var(--muted)' }}>tokens</div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}