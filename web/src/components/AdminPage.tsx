import { useState, useEffect } from 'react';
import { client, setAPIKey, getAPIKey, clearAPIKey, getKeyScope } from '../api/client';

export function AdminPage() {
  const [tenants, setTenants] = useState<any[]>([]);
  const [selectedTenant, setSelectedTenant] = useState<any>(null);
  const [keys, setKeys] = useState<any[]>([]);
  const [usage, setUsage] = useState<any>(null);
  const [signingKey, setSigningKey] = useState('');
  const [newName, setNewName] = useState('');
  const [newEmail, setNewEmail] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [keyName, setKeyName] = useState('');
  const [keyScope, setKeyScope] = useState('api');
  const [rechargeAmount, setRechargeAmount] = useState('');

  const loadTenants = () => {
    client.listTenants().then(setTenants).catch(console.error);
  };

  useEffect(() => { loadTenants(); }, []);

  const createTenant = async () => {
    if (!newName) return;
    await client.createTenant({ name: newName, email: newEmail, password: newPassword });
    setNewName('');
    setNewEmail('');
    setNewPassword('');
    loadTenants();
  };

  const selectTenant = (t: any) => {
    setSelectedTenant(t);
    client.listTenantKeys(t.id).then(setKeys).catch(console.error);
    client.getTenantUsage(t.id).then(setUsage).catch(console.error);
    // Fetch signing key
    fetch(`${window.location.origin}/api/admin/tenants/${t.id}/secret`, {
      headers: { 'Authorization': `Bearer ${getAPIKey()}` }
    }).then(r => r.json()).then(d => setSigningKey(d.signing_key || '')).catch(() => {});
  };

  const createKey = async () => {
    if (!selectedTenant || !keyName) return;
    const k = await client.createTenantKey(selectedTenant.id, { name: keyName, scope: keyScope });
    setKeys([...keys, k]);
    setKeyName('');
  };

  const deleteKey = async (kid: number) => {
    if (!selectedTenant) return;
    await client.deleteTenantKey(selectedTenant.id, kid);
    setKeys(keys.filter(k => k.id !== kid));
  };

  const recharge = async () => {
    if (!selectedTenant || !rechargeAmount) return;
    const r = await client.rechargeTenant(selectedTenant.id, Number(rechargeAmount));
    setSelectedTenant({ ...selectedTenant, balance: r.balance });
    setRechargeAmount('');
  };

  const updateTenant = async (field: string, value: any) => {
    if (!selectedTenant) return;
    await client.updateTenant(selectedTenant.id, { [field]: value });
    setSelectedTenant({ ...selectedTenant, [field]: value });
  };

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <h2 className="mgmt-page-title">Admin: Tenants</h2>

        <div style={{ display: 'flex', gap: 8, marginBottom: 16, flexWrap: 'wrap' }}>
          <input className="form-input" placeholder="Tenant name" value={newName} onChange={e => setNewName(e.target.value)} />
          <input className="form-input" type="email" placeholder="Email (optional)" value={newEmail} onChange={e => setNewEmail(e.target.value)} />
          <input className="form-input" type="password" placeholder="Password (optional)" value={newPassword} onChange={e => setNewPassword(e.target.value)} />
          <button className="mgmt-new-btn" onClick={createTenant}>Create Tenant</button>
        </div>

        <div style={{ display: 'flex', gap: 16 }}>
          <div style={{ flex: '0 0 280px' }}>
            {tenants.map(t => (
              <div key={t.id} className={`thread-item ${selectedTenant?.id === t.id ? 'active' : ''}`}
                   onClick={() => selectTenant(t)}
                   style={{ cursor: 'pointer', padding: '8px 12px', borderBottom: '1px solid var(--border)' }}>
                <div style={{ fontWeight: 600 }}>{t.name}</div>
                <div style={{ fontSize: 11, color: 'var(--muted)' }}>
                  Balance: ${t.balance?.toFixed(2)} | Tokens: {t.quota_tokens?.toLocaleString()} | {t.status}
                </div>
              </div>
            ))}
          </div>

          {selectedTenant && (
            <div style={{ flex: 1 }}>
              <h3>{selectedTenant.name}</h3>
              <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                <input className="form-input" type="number" placeholder="Amount" value={rechargeAmount}
                       onChange={e => setRechargeAmount(e.target.value)} style={{ width: 120 }} />
                <button className="mgmt-new-btn" onClick={recharge}>Recharge</button>
              </div>
              <div className="card-kv">
                <span className="card-kv-key">Quota Tokens</span>
                <input className="form-input" type="number" value={selectedTenant.quota_tokens || ''}
                       onChange={e => updateTenant('quota_tokens', Number(e.target.value))} style={{ width: 150 }} />
              </div>

              <h4 style={{ marginTop: 16 }}>API Keys</h4>
              <div style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
                <input className="form-input" placeholder="Key name" value={keyName} onChange={e => setKeyName(e.target.value)} />
                <select className="form-select" value={keyScope} onChange={e => setKeyScope(e.target.value)}>
                  <option value="api">API</option>
                  <option value="console">Console</option>
                </select>
                <button className="mgmt-new-btn" onClick={createKey}>Create Key</button>
              </div>
              {keys.map(k => (
                <div key={k.id} style={{ display: 'flex', gap: 8, alignItems: 'center', padding: '4px 0', fontSize: 12 }}>
                  <code>{k.key}</code>
                  <span style={{ color: 'var(--muted)' }}>({k.scope})</span>
                  <button className="card-btn danger" onClick={() => deleteKey(k.id)}>Revoke</button>
                </div>
              ))}

              {signingKey && (
                <div style={{ marginTop: 16, padding: 12, background: 'var(--wash)', borderRadius: 6 }}>
                  <div style={{ fontSize: 12, color: 'var(--muted)', marginBottom: 4 }}>Signing Key</div>
                  <code style={{ fontSize: 11, wordBreak: 'break-all' }}>{signingKey}</code>
                  <div style={{ marginTop: 8 }}>
                    <button className="card-btn" onClick={async () => {
                      const r = await fetch(`/api/admin/tenants/${selectedTenant.id}/rotate-secret`, {
                        method: 'POST',
                        headers: { 'Authorization': `Bearer ${getAPIKey()}` }
                      });
                      const d = await r.json();
                      setSigningKey(d.signing_key);
                    }}>Rotate Key</button>
                  </div>
                </div>
              )}

              {usage && (
                <div style={{ marginTop: 16 }}>
                  <h4>Usage (30d): {usage.total_tokens?.toLocaleString()} tokens</h4>
                </div>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}