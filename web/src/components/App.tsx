import { useEffect, useState } from 'react';
import { AppProvider, useAppState } from '../context/AppContext';
import { client, getAPIKey, setAPIKey, getKeyScope, clearAPIKey } from '../api/client';
import { LoginPage } from './LoginPage';
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
    dispatch({ type: 'SET_PAGE', page: p as Page });
  };

  return (
    <div className="app-shell">
      <TopNav page={scope === 'admin' ? 'admin' : scope === 'console' ? page : page} onPageChange={handlePageChange} onLogout={handleLogout} scope={scope} />
      <div className="page">
        {scope === 'admin' ? (
          <AdminPage />
        ) : scope === 'console' ? (
          page === 'chat' ? (
            <div style={{ display: 'flex', flex: 1, flexDirection: 'column', overflow: 'hidden' }}>
              <ChatPage />
            </div>
          ) : (
            <ConsolePage />
          )
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
