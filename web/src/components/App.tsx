import { useEffect } from 'react';
import { AppProvider, useAppState } from '../context/AppContext';
import { client } from '../api/client';
import { TopNav } from './TopNav';
import { ChatPage } from './ChatPage';
import { AgentPage } from './AgentPage';
import { MCPPage } from './MCPPage';
import { ModelPage } from './ModelPage';
import { RuntimePage } from './RuntimePage';
import { Toaster } from './Toaster';
import type { Page } from '../types';

function AppInner() {
  const { state, dispatch } = useAppState();
  const { page } = state;

  // Startup: load initial data
  useEffect(() => {
    Promise.all([
      client.listThreads(),
      client.listAgents(),
      client.getSettings(),
      client.listTools(),
    ]).then(([threads, agents, settings, tools]) => {
      dispatch({ type: 'SET_THREADS', threads });
      dispatch({ type: 'SET_AGENTS', agents });
      dispatch({ type: 'SET_TOOLS', tools });
      const models = settings.llm?.models || [];
      dispatch({ type: 'SET_MODELS', models });
      dispatch({ type: 'SET_HAS_MODELS', has: models.length > 0 });
      const defaultAgent = agents.find(a => a.name === 'Default') || agents[0];
      if (defaultAgent) {
        // SET_ACTIVE_AGENT now also sets activeModelId from agent's default.
        dispatch({ type: 'SET_ACTIVE_AGENT', agentId: defaultAgent.id });
      } else {
        // No agent — just pick the first model.
        dispatch({ type: 'SET_ACTIVE_MODEL', modelId: models.length > 0 ? models[0].id : '' });
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
        {page === 'chat' && <ChatPage />}
        {page === 'agent' && <AgentPage />}
        {page === 'mcp' && <MCPPage />}
        {page === 'model' && <ModelPage />}
        {page === 'runtime' && <RuntimePage />}
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
