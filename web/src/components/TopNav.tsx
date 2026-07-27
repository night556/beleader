import type { Page } from '../types';
import { t } from '../i18n';

interface Props {
  page: Page | 'admin';
  onPageChange: (page: Page | 'admin' | 'console') => void;
  onLogout?: () => void;
  scope?: string;
}

const TABS: { page: Page | 'admin' | 'console'; label: string; adminOnly?: boolean }[] = [
  { page: 'chat', label: 'Chat' },
  { page: 'agent', label: 'Agent' },
  { page: 'mcp', label: 'MCP' },
  { page: 'model', label: 'Model' },
  { page: 'pool', label: 'Pools', adminOnly: true },
];

export function TopNav({ page, onPageChange, onLogout, scope }: Props) {
  const tabs = scope === 'admin'
    ? [{ page: 'admin' as const, label: 'Admin' }]
    : TABS.filter(t => !t.adminOnly || scope === 'admin');

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
