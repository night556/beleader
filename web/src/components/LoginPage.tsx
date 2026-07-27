import { useState } from 'react';
import { setAPIKey } from '../api/client';

function LoginPage({ onLogin }: { onLogin: (key: string) => void }) {
  const [mode, setMode] = useState<'console' | 'apikey' | 'admin'>('console');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [key, setKey] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const handleConsoleLogin = async () => {
    setError('');
    setLoading(true);
    try {
      const url = mode === 'admin'
        ? `${window.location.origin}/api/admin/login`
        : `${window.location.origin}/api/console/login`;
      const body = mode === 'admin'
        ? JSON.stringify({ username: email, password })
        : JSON.stringify({ app_key: email, app_secret: password });
      const r = await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body,
      });
      const data = await r.json();
      if (!r.ok) throw new Error(data.error);
      setAPIKey(data.token, mode === 'admin' ? 'admin' : 'console');
      onLogin(data.token);
    } catch (e: any) {
      setError(e.message);
    } finally {
      setLoading(false);
    }
  };

  const handleKeyLogin = () => {
    if (key) {
      const scope = mode === 'admin' ? 'admin' : 'console';
      setAPIKey(key, scope);
      onLogin(key);
    }
  };

  return (
    <div style={{
      display: 'flex', alignItems: 'center', justifyContent: 'center',
      height: '100vh', background: 'var(--page)',
    }}>
      <div style={{
        width: 400, padding: 40, background: 'var(--surface)',
        borderRadius: 16, border: '1px solid var(--border)',
        boxShadow: 'var(--shadow-lg)',
      }}>
        <div style={{ textAlign: 'center', marginBottom: 32 }}>
          <div style={{
            width: 48, height: 48, borderRadius: 12, background: 'var(--accent)',
            margin: '0 auto 16px', display: 'flex', alignItems: 'center', justifyContent: 'center',
          }}>
            <span style={{ color: '#fff', fontSize: 24, fontWeight: 700 }}>B</span>
          </div>
          <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>BeLeader</h1>
          <p style={{ color: 'var(--muted)', fontSize: 13, margin: '4px 0 0' }}>AI Collaboration Platform</p>
        </div>

        <div style={{ display: 'flex', marginBottom: 24, background: 'var(--elevated)', borderRadius: 8, padding: 3 }}>
          {(['console', 'apikey', 'admin'] as const).map(m => (
            <button
              key={m}
              onClick={() => setMode(m)}
              style={{
                flex: 1, padding: '8px 0', border: 'none', borderRadius: 6,
                background: mode === m ? 'var(--surface)' : 'transparent',
                color: mode === m ? 'var(--text)' : 'var(--muted)',
                fontWeight: mode === m ? 600 : 400,
                fontSize: 13, cursor: 'pointer',
                boxShadow: mode === m ? 'var(--shadow-sm)' : 'none',
              }}
            >
              {m === 'console' ? 'Console' : m === 'admin' ? 'Admin' : 'API Key'}
            </button>
          ))}
        </div>

        {mode === 'console' || mode === 'admin' ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div>
              <label className="form-label">{mode === 'admin' ? 'Username' : 'App Key'}</label>
              <input className="form-input" placeholder={mode === 'admin' ? 'admin' : 'ak_...'}
                value={email} onChange={e => setEmail(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleConsoleLogin()} />
            </div>
            <div>
              <label className="form-label">{mode === 'admin' ? 'Password' : 'App Secret'}</label>
              <input className="form-input" type="password" placeholder="••••••••"
                value={password} onChange={e => setPassword(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleConsoleLogin()} />
            </div>
            <button className="card-btn primary" onClick={handleConsoleLogin}
              disabled={loading}
              style={{ width: '100%', padding: '10px 0', fontSize: 14, justifyContent: 'center' }}>
              {loading ? 'Signing in...' : 'Sign In'}
            </button>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div>
              <label className="form-label">API Key</label>
              <input className="form-input" type="password" placeholder="bl_..."
                value={key} onChange={e => setKey(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleKeyLogin()} />
            </div>
            <button className="card-btn primary" onClick={handleKeyLogin}
              style={{ width: '100%', padding: '10px 0', fontSize: 14, justifyContent: 'center' }}>
              Sign In
            </button>
          </div>
        )}

        {error && (
          <div style={{
            marginTop: 16, padding: '10px 14px', borderRadius: 8,
            background: 'rgba(239,68,68,0.08)', color: 'var(--red)',
            fontSize: 13, textAlign: 'center',
          }}>{error}</div>
        )}
      </div>
    </div>
  );
}

export { LoginPage };