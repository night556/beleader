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
      const r = await fetch(`${window.location.origin}/api/console/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });
      const data = await r.json();
      if (!r.ok) throw new Error(data.error);
      setAPIKey(data.token, 'console');
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

        {mode === 'console' ? (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            <div>
              <label className="form-label">Email</label>
              <input className="form-input" type="email" placeholder="tenant@example.com"
                value={email} onChange={e => setEmail(e.target.value)}
                onKeyDown={e => e.key === 'Enter' && handleConsoleLogin()} />
            </div>
            <div>
              <label className="form-label">Password</label>
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
              <label className="form-label">
                {mode === 'admin' ? 'Admin Key' : 'API Key'}
              </label>
              <input className="form-input" type="password"
                placeholder={mode === 'admin' ? 'bl_admin_...' : 'bl_...'}
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