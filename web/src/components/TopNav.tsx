import type { Page } from '../types';
import { t } from '../i18n';

interface Props {
  page: Page;
  onPageChange: (page: Page) => void;
}

const TABS: { page: Page; label: string }[] = [
  { page: 'chat', label: 'Chat' },
  { page: 'agent', label: 'Agent' },
  { page: 'mcp', label: 'MCP' },
  { page: 'model', label: 'Model' },
  { page: 'runtime', label: 'Runtime' },
];

export function TopNav({ page, onPageChange }: Props) {
  return (
    <nav className="topnav">
      <div className="topnav-brand">
        <span className="topnav-brand-dot" />
        {t('app.title')}
      </div>
      {TABS.map(tab => (
        <button
          key={tab.page}
          className={`topnav-tab ${page === tab.page ? 'active' : ''}`}
          onClick={() => onPageChange(tab.page)}
        >
          {tab.label}
        </button>
      ))}
    </nav>
  );
}
