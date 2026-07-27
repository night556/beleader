import type { Page } from '../types';
import { t } from '../i18n';

interface Props {
  page: Page | 'admin';
  onPageChange: (page: Page | 'admin' | 'console') => void;
  onLogout?: () => void;
  scope?: string;
}

const TABS: { page: Page | 'admin' | 'console'; label: string; adminOnly?: boolean; consoleOnly?: boolean }[] = [
  { page: 'chat', label: 'Chat' },
  { page: 'agent', label: 'Agent', consoleOnly: true },
  { page: 'mcp', label: 'MCP', consoleOnly: true },
  { page: 'model', label: 'Model', consoleOnly: true },
  { page: 'pool', label: 'Pools', adminOnly: true },
];

export function TopNav({ page, onPageChange, onLogout, scope }: Props) {
  const tabs = scope === 'admin'
    ? [{ page: 'admin' as const, label: 'Admin' }]
    : scope === 'console'
    ? TABS.filter(t => !t.adminOnly)
    : [{ page: 'chat' as const, label: 'Chat' }];

  return (
    <nav className="topnav">
      <div className="topnav-brand">
        <span className="topnav-brand-dot" />
        {t('app.title')}
      </div>
      {tabs.map(tab => (
        <button
          key={tab.page}
          className={`topnav-tab ${page === tab.page ? 'active' : ''}`}
          onClick={() => onPageChange(tab.page)}
        >
          {tab.label}
        </button>
      ))}
      <div style={{ flex: 1 }} />
      {onLogout && (
        <button className="topnav-tab" onClick={onLogout} style={{ fontSize: 11 }}>Logout</button>
      )}
    </nav>
  );
}
