import { useState, useEffect } from 'react';
import { client, getAPIKey } from '../api/client';

export function AdminPage() {
  const [tenants, setTenants] = useState<any[]>([]);
  const [selected, setSelected] = useState<any>(null);
  const [keys, setKeys] = useState<any[]>([]);
  const [usage, setUsage] = useState<any>(null);
  const [signingKey, setSigningKey] = useState('');
  const [showCreate, setShowCreate] = useState(false);
  const [newName, setNewName] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [resetPassword, setResetPassword] = useState('');
  const [keyName, setKeyName] = useState('');
  const [keyScope, setKeyScope] = useState('api');
  const [rechargeAmount, setRechargeAmount] = useState('');

  const loadTenants = () => {
    client.listTenants().then(setTenants).catch(console.error);
  };

  useEffect(() => { loadTenants(); }, []);

  const selectTenant = (t: any) => {
    setSelected(t);
    client.listTenantKeys(t.id).then(setKeys).catch(console.error);
    client.getTenantUsage(t.id).then(setUsage).catch(console.error);
    fetch(`/api/admin/tenants/${t.id}/secret`, {
      headers: { 'Authorization': `Bearer ${getAPIKey()}` }
    }).then(r => r.json()).then(d => setSigningKey(d.signing_key || '')).catch(() => {});
  };

  const createTenant = async () => {
    if (!newName) return;
    const r = await fetch('/api/admin/tenants', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${getAPIKey()}` },
      body: JSON.stringify({ name: newName, password: newPassword || undefined }),
    });
    const data = await r.json();
    setNewName(''); setNewPassword(''); setShowCreate(false);
    // Show the generated credentials
    alert(`Tenant created!\n\nApp Key: ${data.app_key}\nApp Secret: ${data.app_secret}\n\nSave these credentials — the secret won't be shown again.`);
    loadTenants();
  };

  const createKey = async () => {
    if (!selected || !keyName) return;
    const k = await client.createTenantKey(selected.id, { name: keyName, scope: keyScope });
    setKeys([...keys, k]);
    setKeyName('');
  };

  const deleteKey = async (kid: number) => {
    if (!selected) return;
    await client.deleteTenantKey(selected.id, kid);
    setKeys(keys.filter(k => k.id !== kid));
  };

  const recharge = async () => {
    if (!selected || !rechargeAmount) return;
    const r = await client.rechargeTenant(selected.id, Number(rechargeAmount));
    setSelected({ ...selected, balance: r.balance });
    setRechargeAmount('');
    loadTenants();
  };

  const updateField = async (field: string, value: any) => {
    if (!selected) return;
    await client.updateTenant(selected.id, { [field]: value });
    setSelected({ ...selected, [field]: value });
  };

  const rotateSecret = async () => {
    if (!selected) return;
    const r = await fetch(`/api/admin/tenants/${selected.id}/rotate-secret`, {
      method: 'POST',
      headers: { 'Authorization': `Bearer ${getAPIKey()}` }
    });
    const d = await r.json();
    setSigningKey(d.signing_key);
  };

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <div className="mgmt-page-head">
          <h2 className="mgmt-page-title">Admin</h2>
          <button className="card-btn primary" onClick={() => setShowCreate(true)}>+ New Tenant</button>
        </div>

        {showCreate && (
          <div className="modal-backdrop" onClick={() => setShowCreate(false)}>
            <div className="modal-dialog" onClick={e => e.stopPropagation()} style={{ maxWidth: 440 }}>
              <div className="modal-head">
                <h3>Create Tenant</h3>
                <button className="modal-close" onClick={() => setShowCreate(false)}>×</button>
              </div>
              <div className="modal-body" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div>
                  <label className="form-label">Name *</label>
                  <input className="form-input" placeholder="e.g. Acme Corp" value={newName}
                    onChange={e => setNewName(e.target.value)} />
                </div>
                <div>
                  <label className="form-label">App Secret (optional, auto-generated if empty)</label>
                  <input className="form-input" type="password" placeholder="Leave empty for random"
                    value={newPassword} onChange={e => setNewPassword(e.target.value)} />
                </div>
              </div>
              <div className="modal-foot">
                <button className="card-btn" onClick={() => setShowCreate(false)}>Cancel</button>
                <button className="card-btn primary" onClick={createTenant}>Create</button>
              </div>
            </div>
          </div>
        )}

        <div style={{ display: 'flex', gap: 20 }}>
          <div style={{ width: 280, flexShrink: 0 }}>
            {tenants.map(t => (
              <div key={t.id}
                onClick={() => selectTenant(t)}
                style={{
                  padding: '12px 14px', cursor: 'pointer', borderRadius: 8, marginBottom: 4,
                  background: selected?.id === t.id ? 'var(--wash-strong)' : 'transparent',
                  border: selected?.id === t.id ? '1px solid var(--accent)' : '1px solid transparent',
                }}>
                <div style={{ fontWeight: 600, fontSize: 14 }}>{t.name}</div>
                <div style={{ fontSize: 11, color: 'var(--muted)', marginTop: 2 }}>
                  ${t.balance?.toFixed(2)} · {t.status}
                </div>
              </div>
            ))}
            {tenants.length === 0 && (
              <div style={{ color: 'var(--muted)', fontSize: 13, padding: 20, textAlign: 'center' }}>
                No tenants yet
              </div>
            )}
          </div>

          <div style={{ flex: 1 }}>
            {!selected ? (
              <div style={{ padding: 60, textAlign: 'center', color: 'var(--muted)' }}>
                Select a tenant to view details
              </div>
            ) : (
              <div style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
                <div className="card" style={{ padding: 20 }}>
                  <h3 style={{ margin: '0 0 12px' }}>{selected.name}</h3>
                  <div style={{ display: 'flex', gap: 12, marginBottom: 16, flexWrap: 'wrap' }}>
                    <input className="form-input" type="number" placeholder="Amount"
                      value={rechargeAmount} onChange={e => setRechargeAmount(e.target.value)}
                      style={{ width: 120 }} />
                    <button className="card-btn primary" onClick={recharge}>Recharge</button>
                  </div>
                  <div style={{ fontSize: 13, color: 'var(--muted)', marginBottom: 12 }}>
                    App Key: <code>{selected.app_key}</code>
                  </div>
                  <div style={{ display: 'flex', gap: 8, marginBottom: 16 }}>
                    <input className="form-input" type="password" placeholder="New app secret"
                      value={resetPassword} onChange={e => setResetPassword(e.target.value)}
                      style={{ width: 180 }} />
                    <button className="card-btn" onClick={async () => {
                      if (!resetPassword) return;
                      await client.updateTenant(selected.id, { password: resetPassword });
                      setResetPassword('');
                      alert('App secret updated');
                    }}>Reset Secret</button>
                  </div>
                  <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, fontSize: 13 }}>
                    <div>
                      <span style={{ color: 'var(--muted)' }}>Quota Tokens</span>
                      <input className="form-input" type="number" value={selected.quota_tokens || ''}
                        onChange={e => updateField('quota_tokens', Number(e.target.value))}
                        style={{ width: '100%', marginTop: 4 }} />
                    </div>
                    <div>
                      <span style={{ color: 'var(--muted)' }}>Status</span>
                      <select className="form-select" value={selected.status}
                        onChange={e => updateField('status', e.target.value)}
                        style={{ width: '100%', marginTop: 4 }}>
                        <option value="active">Active</option>
                        <option value="suspended">Suspended</option>
                      </select>
                    </div>
                  </div>
                </div>

                <div className="card" style={{ padding: 20 }}>
                  <h4 style={{ margin: '0 0 12px' }}>API Keys</h4>
                  <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
                    <input className="form-input" placeholder="Key name" value={keyName}
                      onChange={e => setKeyName(e.target.value)} />
                    <select className="form-select" value={keyScope}
                      onChange={e => setKeyScope(e.target.value)}>
                      <option value="api">API</option>
                      <option value="console">Console</option>
                    </select>
                    <button className="card-btn primary" onClick={createKey}>Create</button>
                  </div>
                  {keys.map(k => (
                    <div key={k.id} style={{
                      display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                      padding: '8px 0', borderBottom: '1px solid var(--border)', fontSize: 12
                    }}>
                      <div>
                        <div style={{ fontWeight: 600 }}>{k.name || 'Unnamed'}</div>
                        <code style={{ fontSize: 11 }}>{k.key}</code>
                        <span style={{ color: 'var(--muted)', marginLeft: 8 }}>({k.scope})</span>
                      </div>
                      <button className="card-btn danger" onClick={() => deleteKey(k.id)}>Revoke</button>
                    </div>
                  ))}
                </div>

                <div className="card" style={{ padding: 20, background: 'var(--wash)' }}>
                  <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                    <h4 style={{ margin: 0 }}>Signing Key</h4>
                    <button className="card-btn" onClick={rotateSecret}>Rotate</button>
                  </div>
                  <code style={{ fontSize: 11, wordBreak: 'break-all' }}>{signingKey || 'Loading...'}</code>
                </div>

                {usage && (
                  <div className="card" style={{ padding: 20 }}>
                    <h4 style={{ margin: '0 0 8px' }}>Usage (30d)</h4>
                    <div style={{ fontSize: 28, fontWeight: 700 }}>{usage.total_tokens?.toLocaleString()}</div>
                    <div style={{ fontSize: 12, color: 'var(--muted)' }}>tokens</div>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}