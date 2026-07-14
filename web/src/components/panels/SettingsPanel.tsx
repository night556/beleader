import { useState } from 'react';
import { useAppState } from '../../context/AppContext';
import { client } from '../../api/client';
import { t } from '../../i18n';
import type { ModelProfile } from '../../types';

interface Props { onClose: () => void }

const PROVIDERS: Record<string, { name: string; base: string; models: string[] }> = {
  openai:   { name: 'OpenAI', base: 'https://api.openai.com/v1', models: ['gpt-5.5', 'gpt-5.4-mini', 'gpt-4.1', 'o4-mini', 'gpt-4o'] },
  google:   { name: 'Google Gemini', base: 'https://generativelanguage.googleapis.com/v1beta/openai', models: ['gemini-2.5-pro', 'gemini-2.5-flash'] },
  deepseek: { name: 'DeepSeek', base: 'https://api.deepseek.com', models: ['deepseek-v4-pro', 'deepseek-v4-flash'] },
  groq:     { name: 'Groq', base: 'https://api.groq.com/openai/v1', models: ['meta-llama/llama-4-maverick-17b-128e-instruct', 'qwen/qwen3-32b'] },
  ollama:   { name: 'Ollama', base: 'http://localhost:11434/v1', models: [] },
};

export function SettingsPanel({ onClose }: Props) {
  const { state, dispatch } = useAppState();
  const [models, setModels] = useState<ModelProfile[]>(() =>
    state.models.map(m => ({ ...m }))
  );
  const [activeId, setActiveId] = useState(state.activeModelId);

  const addModel = () => {
    setModels(prev => [...prev, { id: '', base_url: '', api_key: '', model: '', vision: false, context_limit: 128000 }]);
  };

  const deleteModel = (idx: number) => {
    const m = models[idx];
    if (m.id === activeId) { alert(t('error.cannot_delete_active')); return; }
    setModels(prev => prev.filter((_, i) => i !== idx));
  };

  const updateModel = (idx: number, field: keyof ModelProfile, value: string | boolean | number) => {
    setModels(prev => prev.map((m, i) => i === idx ? { ...m, [field]: value } : m));
  };

  const setProvider = (idx: number, key: string) => {
    const p = PROVIDERS[key];
    if (p) {
      setModels(prev => prev.map((m, i) => i === idx ? { ...m, base_url: p.base } : m));
    }
  };

  const save = async () => {
    const valid = models.filter(m => m.id);
    if (valid.length === 0) { alert(t('error.at_least_one_model')); return; }
    const active = activeId || valid[0].id;
    await client.updateSettings({ llm: { models: valid, active } });
    dispatch({ type: 'SET_MODELS', models: valid });
    dispatch({ type: 'SET_ACTIVE_MODEL', modelId: active });
    dispatch({ type: 'SET_HAS_MODELS', has: valid.length > 0 });
    onClose();
  };

  return (
    <div className="panel open">
      <div className="panel-head">
        <h3>{t('settings.title')}</h3>
        <button className="panel-close" onClick={onClose}>✕</button>
      </div>
      <div className="panel-body">
        <div className="set-section">
          <h4>{t('settings.models')}</h4>
          <button className="btn-add" onClick={addModel} style={{ marginBottom: 10 }}>{t('settings.add_model')}</button>
          {models.map((m, i) => (
            <div key={i} className="model-card">
              <div className="model-card-header">
                <span className="model-name-display">{m.id || t('model.new')}</span>
                <span>
                  {m.id === activeId
                    ? <span className="model-badge model-badge-active">active</span>
                    : <span className="model-badge" onClick={() => setActiveId(m.id)}>set active</span>
                  }
                  <span className="model-badge model-badge-delete" onClick={() => deleteModel(i)}>×</span>
                </span>
              </div>
              <div className="model-field">
                <label>{t('model.id_label')}</label>
                <input value={m.id} onChange={e => updateModel(i, 'id', e.target.value)} placeholder={t('model.id_placeholder')} />
              </div>
              <div className="model-field">
                <label>Provider</label>
                <select onChange={e => setProvider(i, e.target.value)} defaultValue="">
                  <option value="">Custom</option>
                  {Object.entries(PROVIDERS).map(([k, v]) => (
                    <option key={k} value={k}>{v.name}</option>
                  ))}
                </select>
              </div>
              <div className="model-field">
                <label>{t('model.base_url')}</label>
                <input value={m.base_url} onChange={e => updateModel(i, 'base_url', e.target.value)} placeholder={t('model.base_url_placeholder')} />
              </div>
              <div className="model-field">
                <label>{t('model.api_key')}</label>
                <input type="password" value={m.api_key} onChange={e => updateModel(i, 'api_key', e.target.value)} placeholder="sk-..." />
              </div>
              <div className="model-field">
                <label>{t('model.model_select')}</label>
                <input value={m.model} onChange={e => updateModel(i, 'model', e.target.value)} placeholder={t('model.model_placeholder')} />
              </div>
              <div className="model-field model-field-inline">
                <label>Context Limit</label>
                <input type="number" value={m.context_limit} onChange={e => updateModel(i, 'context_limit', parseInt(e.target.value) || 128000)} min={4096} step={1024} />
              </div>
              <div className="model-field model-field-inline">
                <label>Vision</label>
                <input type="checkbox" checked={m.vision} onChange={e => updateModel(i, 'vision', e.target.checked)} />
              </div>
            </div>
          ))}
        </div>
        <button className="btn-save" onClick={save}>{t('settings.save')}</button>
      </div>
    </div>
  );
}
