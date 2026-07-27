import { useEffect, useState } from 'react';
import { AppProvider, useAppState } from '../context/AppContext';
import { client, getAPIKey, setAPIKey, getKeyScope, clearAPIKey } from '../api/client';
import { TopNav } from './TopNav';
import { ChatPage } from './ChatPage';
import { AgentPage } from './AgentPage';
import { MCPPage } from './MCPPage';
import { ModelPage } from './ModelPage';
import { PoolPage } from './PoolPage';
import { AdminPage } from './AdminPage';
import { ConsolePage } from './ConsolePage';
import { Toaster } from './Toaster';
import type { Page } from '../types';

function LoginPage({ onLogin }: { onLogin: (key: string) => void }) {
  const [mode, setMode] = useState<'console' | 'apikey'>('console');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [key, setKey] = useState('');
  const [error, setError] = useState('');

  const handleConsoleLogin = async () => {
    setError('');
    try {
      const r = await fetch(`${window.location.origin}/api/console/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, password }),
      });
      const data = await r.json();
      if (!r.ok) throw new Error(data.error);
      setAPIKey(data.token);
      onLogin(data.token);
    } catch (e: any) {
      setError(e.message);
    }
  };

  const handleKeyLogin = () => {
    if (key) {
      setAPIKey(key);
      onLogin(key);
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16 }}>
      <div style={{ fontSize: 24, fontWeight: 700 }}>BeLeader</div>
      <div style={{ display: 'flex', gap: 0, marginBottom: 8 }}>
        <button className={`topnav-tab ${mode === 'console' ? 'active' : ''}`} onClick={() => setMode('console')}>Console</button>
        <button className={`topnav-tab ${mode === 'apikey' ? 'active' : ''}`} onClick={() => setMode('apikey')}>API Key</button>
      </div>

      {mode === 'console' ? (
        <>
          <input className="form-input" type="email" placeholder="Email" value={email}
                 onChange={e => setEmail(e.target.value)} style={{ width: 300 }}
                 onKeyDown={e => e.key === 'Enter' && handleConsoleLogin()} />
          <input className="form-input" type="password" placeholder="Password" value={password}
                 onChange={e => setPassword(e.target.value)} style={{ width: 300 }}
                 onKeyDown={e => e.key === 'Enter' && handleConsoleLogin()} />
          <button className="mgmt-new-btn" onClick={handleConsoleLogin}>Login</button>
        </>
      ) : (
        <>
          <input className="form-input" type="password" placeholder="bl_..." value={key}
                 onChange={e => setKey(e.target.value)} style={{ width: 360 }}
                 onKeyDown={e => e.key === 'Enter' && handleKeyLogin()} />
          <button className="mgmt-new-btn" onClick={handleKeyLogin}>Login</button>
        </>
      )}
      {error && <div style={{ color: 'var(--red)', fontSize: 13 }}>{error}</div>}
    </div>
  );
}

function AppInner() {
  const { state, dispatch } = useAppState();
  const { page } = state;
  const [loggedIn, setLoggedIn] = useState(false);
  const scope = getKeyScope();

  const handleLogin = (key: string) => {
    setAPIKey(key);
    setLoggedIn(true);
  };

  const handleLogout = () => {
    clearAPIKey();
    setLoggedIn(false);
  };

  // Check if already logged in
  useEffect(() => {
    if (getAPIKey()) {
      setLoggedIn(true);
    }
  }, []);

  useEffect(() => {
    if (!loggedIn) return;
    Promise.all([
      client.listThreads(),
      client.listAgents(),
      client.listModels(),
      client.listTools(),
      client.listPools(),
    ]).then(([threads, agents, models, tools, pools]) => {
      dispatch({ type: 'SET_THREADS', threads });
      dispatch({ type: 'SET_AGENTS', agents });
      dispatch({ type: 'SET_TOOLS', tools });
      dispatch({ type: 'SET_MODELS', models });
      dispatch({ type: 'SET_HAS_MODELS', has: models.length > 0 });
      dispatch({ type: 'SET_POOLS', pools });
      const defaultPool = pools.find(p => p.is_default) || pools[0];
      if (defaultPool) {
        dispatch({ type: 'SET_ACTIVE_POOL', poolId: defaultPool.id });
      }
      const defaultAgent = agents.find(a => a.name === 'Default') || agents[0];
      if (defaultAgent) {
        dispatch({ type: 'SET_ACTIVE_AGENT', agentId: defaultAgent.id });
      }
      const modelId = defaultAgent?.default_model_id || (models.length > 0 ? models[0].id : '');
      if (modelId) {
        dispatch({ type: 'SET_ACTIVE_MODEL', modelId });
      }
    }).catch(err => console.error('startup error:', err));
  }, [loggedIn]);

  if (!loggedIn) {
    return <LoginPage onLogin={handleLogin} />;
  }

  const handlePageChange = (p: Page | 'admin' | 'console') => {
    if (p === 'admin' || p === 'console') {
      dispatch({ type: 'SET_PAGE', page: 'chat' });
    } else {
      dispatch({ type: 'SET_PAGE', page: p });
    }
  };

  return (
    <div className="app-shell">
      <TopNav page={scope === 'admin' ? 'admin' : page} onPageChange={handlePageChange} onLogout={handleLogout} scope={scope} />
      <div className="page">
        {scope === 'admin' ? (
          <AdminPage />
        ) : (
          <>
            <div style={{ display: page === 'chat' ? 'flex' : 'none', flex: 1, flexDirection: 'column', overflow: 'hidden' }}>
              <ChatPage />
            </div>
            {page === 'agent' && <AgentPage />}
            {page === 'mcp' && <MCPPage />}
            {page === 'model' && <ModelPage />}
            {page === 'pool' && <PoolPage />}
          </>
        )}
      </div>
      <Toaster />
    </div>
  );
}

export function App() {
  return (
    <AppProvider>
      <AppInner />
    </AppProvider>
  );
}
