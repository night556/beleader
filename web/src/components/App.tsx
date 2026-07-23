import { useEffect } from 'react';
import { AppProvider, useAppState } from '../context/AppContext';
import { client } from '../api/client';
import { TopNav } from './TopNav';
import { ChatPage } from './ChatPage';
import { AgentPage } from './AgentPage';
import { MCPPage } from './MCPPage';
import { ModelPage } from './ModelPage';
import { PoolPage } from './PoolPage';
import { Toaster } from './Toaster';
import type { Page } from '../types';

function AppInner() {
  const { state, dispatch } = useAppState();
  const { page } = state;

  useEffect(() => {
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
      // Set default pool
      const defaultPool = pools.find(p => p.is_default) || pools[0];
      if (defaultPool) {
        dispatch({ type: 'SET_ACTIVE_POOL', poolId: defaultPool.id });
      }
      const defaultAgent = agents.find(a => a.name === 'Default') || agents[0];
      if (defaultAgent) {
        dispatch({ type: 'SET_ACTIVE_AGENT', agentId: defaultAgent.id });
      }
      // Pick initial model from agent's default or first available
      const modelId = defaultAgent?.default_model_id || (models.length > 0 ? models[0].id : '');
      if (modelId) {
        dispatch({ type: 'SET_ACTIVE_MODEL', modelId });
      }
    }).catch(err => console.error('startup error:', err));
  }, []);

  const handlePageChange = (p: Page) => {
    dispatch({ type: 'SET_PAGE', page: p });
  };

  return (
    <div className="app-shell">
      <TopNav page={page} onPageChange={handlePageChange} />
      <div className="page">
        <div style={{ display: page === 'chat' ? 'flex' : 'none', flex: 1, flexDirection: 'column', overflow: 'hidden' }}>
          <ChatPage />
        </div>
        {page === 'agent' && <AgentPage />}
        {page === 'mcp' && <MCPPage />}
        {page === 'model' && <ModelPage />}
        {page === 'pool' && <PoolPage />}
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
