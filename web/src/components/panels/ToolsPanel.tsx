import { useState, useEffect } from 'react';
import { client } from '../../api/client';
import { t } from '../../i18n';
import type { ToolDef } from '../../types';

interface Props { onClose: () => void }

export function ToolsPanel({ onClose }: Props) {
  const [tools, setTools] = useState<ToolDef[]>([]);

  useEffect(() => {
    client.listTools().then(setTools).catch(() => {});
  }, []);

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{t('tools.title')}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        {tools.length === 0 ? (
          <div className="agents-empty">No tools registered</div>
        ) : (
          tools.map(t => {
            const hasParams = t.parameters?.properties && Object.keys(t.parameters.properties).length > 0;
            const reqProps = t.parameters?.required || [];
            return (
              <div key={t.name} className="tool-card" onClick={e => e.currentTarget.classList.toggle('open')}>
                <div className="tool-card-header">
                  <div className="tool-card-top">
                    <span className="tool-card-chevron">▶</span>
                    <span className="tool-card-name">{t.name}</span>
                    <span className={`tool-source-badge ${t.source}`}>{t.source}</span>
                    {hasParams && <span className="tool-card-params-hint">{Object.keys(t.parameters!.properties).length} params</span>}
                  </div>
                </div>
                {hasParams && (
                  <div className="tool-card-body">
                    <table className="tool-params-table">
                      <thead><tr><th>Param</th><th>Type</th><th>Required</th><th>Description</th></tr></thead>
                      <tbody>
                        {Object.entries(t.parameters!.properties).map(([k, p]) => (
                          <tr key={k}>
                            <td><code>{k}</code></td>
                            <td>{p.type}{p.enum ? ` (${p.enum.join('|')})` : ''}</td>
                            <td>{reqProps.includes(k) ? '✓' : ''}</td>
                            <td>{p.description || ''}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                )}
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
