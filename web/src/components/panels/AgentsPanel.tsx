import { useState, useEffect } from 'react';
import { useAppState } from '../../context/AppContext';
import { client } from '../../api/client';
import { t } from '../../i18n';
import type { Agent, ToolDef } from '../../types';

interface Props { onClose: () => void }

export function AgentsPanel({ onClose }: Props) {
  const { state, dispatch } = useAppState();
  const [search, setSearch] = useState('');
  const [editing, setEditing] = useState<Agent | null>(null);
  const [showEditor, setShowEditor] = useState(false);
  const [allTools, setAllTools] = useState<ToolDef[]>([]);

  useEffect(() => {
    client.listTools().then(setAllTools).catch(() => {});
  }, []);

  const filtered = state.agents.filter(a =>
    !search || a.name.toLowerCase().includes(search.toLowerCase()) || (a.desc || '').toLowerCase().includes(search.toLowerCase())
  );

  const openEditor = (agent?: Agent) => {
    setEditing(agent || null);
    setShowEditor(true);
  };

  const deleteAgent = (id: number) => {
    if (!confirm(t('agents.delete_confirm').replace('$1', ''))) return;
    client.deleteAgent(id).then(() => {
      dispatch({ type: 'SET_AGENTS', agents: state.agents.filter(a => a.id !== id) });
    }).catch(err => alert(err.message));
  };

  if (showEditor) {
    return <AgentEditor
      agent={editing}
      tools={allTools}
      onClose={() => setShowEditor(false)}
      onSaved={(agents) => {
        dispatch({ type: 'SET_AGENTS', agents });
        setShowEditor(false);
      }}
    />;
  }

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{t('agents.title')}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        <div style={{ display: 'flex', gap: 8, marginBottom: 12 }}>
          <input
            style={{ flex: 1, padding: '6px 8px', border: '1px solid var(--border)', borderRadius: 6, background: 'var(--bg3)', color: 'var(--text)', fontSize: 12 }}
            placeholder={t('agents.search_placeholder')}
            value={search}
            onChange={e => setSearch(e.target.value)}
          />
          <button className="btn-add" onClick={() => openEditor()}>{t('agents.new')}</button>
        </div>

        {filtered.length === 0 ? (
          <div className="agents-empty">{t('agents.empty')}</div>
        ) : (
          filtered.map(a => {
            let toolNames: string[] = [];
            try { toolNames = JSON.parse(a.tools || '[]'); } catch {}
            return (
              <div key={a.id} className="agent-card">
                <div className="agent-card-head">
                  <span className="agent-card-name">{a.name}</span>
                  <span className="agent-card-desc">{a.desc}</span>
                  <span className="agent-card-actions">
                    <button className="agent-card-btn" onClick={() => openEditor(a)} title={t('agents.edit')}>✎</button>
                    <button className="agent-card-btn delete" onClick={() => deleteAgent(a.id)} title={t('agents.delete')}>✕</button>
                  </span>
                </div>
                {toolNames.length > 0 && (
                  <div className="agent-card-tools">
                    {toolNames.map(tn => {
                      const td = allTools.find(t => t.name === tn);
                      return (
                        <span key={tn} className="agent-tool-chip" title={td?.description}>{tn}</span>
                      );
                    })}
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

// ── Agent Editor (inline) ──

function AgentEditor({ agent, tools, onClose, onSaved }: {
  agent: Agent | null;
  tools: ToolDef[];
  onClose: () => void;
  onSaved: (agents: Agent[]) => void;
}) {
  const [name, setName] = useState(agent?.name || '');
  const [desc, setDesc] = useState(agent?.desc || '');
  const [systemPrompt, setSystemPrompt] = useState(agent?.system_prompt || '');
  const [selectedTools, setSelectedTools] = useState<string[]>(() => {
    try { return agent ? JSON.parse(agent.tools) : []; } catch { return []; }
  });
  const [toolSearch, setToolSearch] = useState('');

  const filteredTools = tools.filter(t =>
    !toolSearch || t.name.toLowerCase().includes(toolSearch.toLowerCase()) || (t.description || '').toLowerCase().includes(toolSearch.toLowerCase())
  );

  const toggleTool = (name: string) => {
    setSelectedTools(prev =>
      prev.includes(name) ? prev.filter(t => t !== name) : [...prev, name]
    );
  };

  const save = async () => {
    if (!name || !systemPrompt) {
      alert(name ? t('agents.system_prompt') + ' required' : t('agents.name') + ' required');
      return;
    }
    const payload = { name, desc, system_prompt: systemPrompt, tools: JSON.stringify(selectedTools) };
    try {
      if (agent) {
        await client.updateAgent(agent.id, payload);
      } else {
        await client.createAgent(payload);
      }
      const agents = await client.listAgents();
      onSaved(agents);
    } catch (err: unknown) {
      alert('Error: ' + (err instanceof Error ? err.message : err));
    }
  };

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{agent ? t('agents.edit_title') : t('agents.new_title')}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        <div className="modal-field">
          <label>{t('agents.name')}</label>
          <input className="modal-input" value={name} onChange={e => setName(e.target.value)} />
        </div>
        <div className="modal-field">
          <label>{t('agents.desc')}</label>
          <input className="modal-input" value={desc} onChange={e => setDesc(e.target.value)} />
        </div>
        <div className="modal-field">
          <label>{t('agents.system_prompt')}</label>
          <textarea className="modal-textarea" value={systemPrompt} onChange={e => setSystemPrompt(e.target.value)}
            onKeyDown={e => {
              if (e.key === 'Tab') { e.preventDefault(); const ta = e.currentTarget; const s = ta.selectionStart; ta.value = ta.value.substring(0, s) + '  ' + ta.value.substring(ta.selectionEnd); ta.selectionStart = ta.selectionEnd = s + 2; }
            }}
          />
        </div>
        <div className="modal-field">
          <label>{t('agents.tools')}</label>
          <div className="tools-chips">
            {selectedTools.length === 0 && <span className="modal-hint">{t('agents.no_tools')}</span>}
            {selectedTools.map(tn => (
              <span key={tn} className="tool-chip">
                {tn}
                <button className="tool-chip-remove" onClick={() => toggleTool(tn)}>×</button>
              </span>
            ))}
          </div>
          <input className="modal-input" placeholder={t('agents.tools_search')} value={toolSearch} onChange={e => setToolSearch(e.target.value)} style={{ marginBottom: 4 }} />
          <div className="tools-picker">
            {filteredTools.map(t => {
              const sel = selectedTools.includes(t.name);
              return (
                <div key={t.name} className={`tool-pick-item ${sel ? 'selected' : ''}`} onClick={() => toggleTool(t.name)}>
                  <span className="tool-pick-name">{t.name}{sel ? ' ✓' : ''}</span>
                  <span className="tool-pick-desc">{t.description}</span>
                </div>
              );
            })}
          </div>
        </div>
        <button className="btn-save" onClick={save}>{t('agents.save')}</button>
      </div>
    </div>
  );
}
