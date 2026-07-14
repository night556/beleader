import { useAppState } from '../context/AppContext';
import { client } from '../api/client';
import { t } from '../i18n';

interface Props {
  open: boolean;
  onClose: () => void;
  onPanelOpen: (name: string) => void;
}

export function Sidebar({ open, onClose, onPanelOpen }: Props) {
  const { state, dispatch } = useAppState();

  const newThread = () => {
    dispatch({ type: 'SET_ACTIVE_THREAD', threadId: null });
    dispatch({ type: 'CLEAR_TIMELINE' });
    onClose();
  };

  const switchThread = (threadId: string) => {
    dispatch({ type: 'SET_ACTIVE_THREAD', threadId });
    dispatch({ type: 'CLEAR_TIMELINE' });
    // Load messages
    client.getMessages(threadId).then(msgs => {
      for (const m of msgs) {
        const icon = kindIcon(m.kind);
        dispatch({
          type: 'PUSH_TIMELINE_ITEM',
          item: {
            id: `msg-${m.id}`, type: kindType(m.kind), icon, label: kindLabel(m.kind),
            content: m.content, status: 'done', time: new Date(m.created_at).getTime(),
          },
        });
      }
    }).catch(console.error);
    onClose();
  };

  const deleteThread = (id: string, e: React.MouseEvent) => {
    e.stopPropagation();
    if (!confirm('Delete this thread?')) return;
    client.deleteThread(id).then(() => {
      dispatch({ type: 'REMOVE_THREAD', threadId: id });
      if (state.activeThreadId === id) {
        dispatch({ type: 'SET_ACTIVE_THREAD', threadId: null });
        dispatch({ type: 'CLEAR_TIMELINE' });
      }
    }).catch(console.error);
  };

  return (
    <nav className={`sidebar ${open ? 'open' : ''}`}>
      <div className="sidebar-head">
        <span className="sidebar-logo">✦</span>
        <span className="sidebar-title">{t('app.title')}</span>
        <button className="sidebar-close" onClick={onClose}>✕</button>
      </div>
      <div className="sidebar-nav">
        {state.threads.map(th => (
          <div
            key={th.id}
            className={`sidebar-thread ${th.id === state.activeThreadId ? 'active' : ''}`}
            onClick={() => switchThread(th.id)}
          >
            {th.title || th.id.slice(0, 8)}
            <button className="sidebar-thread-del" onClick={e => deleteThread(th.id, e)}>✕</button>
          </div>
        ))}
      </div>
      <div className="sidebar-foot">
        <button className="sidebar-new-btn" onClick={newThread}>
          + <span>{t('sidebar.new_thread')}</span>
        </button>
      </div>
      <div className="sidebar-actions">
        <button className="sidebar-action" onClick={() => onPanelOpen('tools')}>{t('topbar.tools')}</button>
        <button className="sidebar-action" onClick={() => onPanelOpen('agents')}>{t('topbar.agents')}</button>
        <button className="sidebar-action" onClick={() => onPanelOpen('mcp')}>{t('topbar.mcp')}</button>
        <button className="sidebar-action" onClick={() => onPanelOpen('knowledge')}>{t('topbar.knowledge')}</button>
        <button className="sidebar-action" onClick={() => onPanelOpen('settings')}>{t('topbar.settings')}</button>
      </div>
    </nav>
  );
}

function kindIcon(kind: string): string {
  const map: Record<string, string> = {
    user_message: '👤', agent_message: '◆', tool_call: '🔧', tool_result: '📋', error: '⚠',
  };
  return map[kind] || '◆';
}

function kindLabel(kind: string): string {
  const map: Record<string, string> = {
    user_message: 'You', agent_message: 'AI', tool_call: 'Tool', tool_result: 'Tool Result', error: 'Error',
  };
  return map[kind] || 'AI';
}

function kindType(kind: string): 'user' | 'agent' | 'tool_call' | 'tool_result' | 'error' {
  if (kind === 'user_message') return 'user';
  if (kind === 'agent_message') return 'agent';
  if (kind === 'tool_call') return 'tool_call';
  if (kind === 'tool_result') return 'tool_result';
  return 'error';
}
