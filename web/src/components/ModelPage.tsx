import { useState } from 'react';
import { useAppState } from '../context/AppContext';
import { client } from '../api/client';
import type { ModelProfile } from '../types';
import { t } from '../i18n';

const EMPTY_MODEL: ModelProfile = {
  id: '', base_url: '', api_key: '', model: '',
  vision: false, context_limit: 64000, reasoning_effort: 'off',
};

export function ModelPage() {
  const { state, dispatch } = useAppState();
  const { models, activeModelId } = state;
  const [showForm, setShowForm] = useState(false);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [form, setForm] = useState<ModelProfile>({ ...EMPTY_MODEL });

  const openNew = () => {
    setEditingId(null);
    setForm({ ...EMPTY_MODEL, id: '', model: '' });
    setShowForm(true);
  };

  const openEdit = (m: ModelProfile) => {
    setEditingId(m.id);
    setForm({ ...m });
    setShowForm(true);
  };

  const save = async () => {
    let updated: ModelProfile[];
    if (editingId) {
      updated = models.map(m => m.id === editingId ? form : m);
    } else {
      if (!form.id.trim()) return;
      if (models.find(m => m.id === form.id)) {
        alert('A model with this ID already exists.');
        return;
      }
      updated = [...models, form];
    }
    dispatch({ type: 'SET_MODELS', models: updated });
    if (!activeModelId && updated.length > 0) {
      dispatch({ type: 'SET_ACTIVE_MODEL', modelId: updated[0].id });
    }
    dispatch({ type: 'SET_HAS_MODELS', has: updated.length > 0 });
    await client.updateSettings({ llm: { models: updated } });
    setShowForm(false);
  };

  const remove = async (id: string) => {
    if (!confirm(`Delete model "${id}"?`)) return;
    const updated = models.filter(m => m.id !== id);
    dispatch({ type: 'SET_MODELS', models: updated });
    dispatch({ type: 'SET_HAS_MODELS', has: updated.length > 0 });
    await client.updateSettings({ llm: { models: updated } });
  };

  return (
    <div className="mgmt-page">
      <div className="mgmt-page-inner">
        <div className="mgmt-page-head">
          <h2 className="mgmt-page-title">{t('settings.models')}</h2>
          <button className="mgmt-new-btn" onClick={openNew}>{t('settings.add_model')}</button>
        </div>

        {models.length === 0 ? (
          <div className="mgmt-empty">{t('error.at_least_one_model')}</div>
        ) : (
          models.map(m => (
            <div className="card model-card" key={m.id}>
              <div className="card-header">
                <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                  <span className="card-title">{m.id}</span>
                </div>
                <div className="card-actions">
                  <button className="card-btn" onClick={() => openEdit(m)}>{t('agents.edit')}</button>
                  <button className="card-btn danger" onClick={() => remove(m.id)}>{t('agents.delete')}</button>
                </div>
              </div>
              <div className="card-body">
                <div className="card-kv">
                  <span className="card-kv-key">Base URL</span>
                  <span className="card-kv-val">{m.base_url || '—'}</span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">Model</span>
                  <span className="card-kv-val">{m.model || '—'}</span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">Context Limit</span>
                  <span className="card-kv-val">{m.context_limit?.toLocaleString() || '—'}</span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">Vision</span>
                  <span className="card-kv-val">{m.vision ? 'Yes' : 'No'}</span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">Reasoning Effort</span>
                  <span className="card-kv-val">{m.reasoning_effort || 'off'}</span>
                </div>
                <div className="card-kv">
                  <span className="card-kv-key">API Key</span>
                  <span className="card-kv-val">{m.api_key ? '••••••••' : '—'}</span>
                </div>
              </div>
            </div>
          ))
        )}

        {showForm && (
          <div className="modal-backdrop" onClick={e => { if (e.target === e.currentTarget) setShowForm(false); }}>
            <div className="modal-dialog wide">
              <div className="modal-head">
                <h3>{editingId ? 'Edit Model' : t('model.new')}</h3>
                <button className="modal-close" onClick={() => setShowForm(false)}>✕</button>
              </div>
              <div className="modal-body">
                <div className="form-row">
                  <div className="form-group">
                    <label className="form-label">{t('model.id_label')}</label>
                    <input
                      className="form-input"
                      placeholder={t('model.id_placeholder')}
                      value={form.id}
                      onChange={e => setForm({ ...form, id: e.target.value })}
                      disabled={!!editingId}
                    />
                  </div>
                  <div className="form-group">
                    <label className="form-label">{t('model.model_select')}</label>
                    <input
                      className="form-input"
                      placeholder={t('model.model_placeholder')}
                      value={form.model}
                      onChange={e => setForm({ ...form, model: e.target.value })}
                    />
                  </div>
                </div>
                <div className="form-group">
                  <label className="form-label">{t('model.base_url')}</label>
                  <input
                    className="form-input"
                    placeholder={t('model.base_url_placeholder')}
                    value={form.base_url}
                    onChange={e => setForm({ ...form, base_url: e.target.value })}
                  />
                </div>
                <div className="form-group">
                  <label className="form-label">{t('model.api_key')}</label>
                  <input
                    className="form-input"
                    type="password"
                    value={form.api_key}
                    onChange={e => setForm({ ...form, api_key: e.target.value })}
                  />
                </div>
                <div className="form-row">
                  <div className="form-group">
                    <label className="form-label">Context Limit</label>
                    <input
                      className="form-input"
                      type="number"
                      value={form.context_limit}
                      onChange={e => setForm({ ...form, context_limit: Number(e.target.value) })}
                    />
                  </div>
                  <div className="form-group">
                    <label className="form-label">Reasoning Effort</label>
                    <select
                      className="form-select"
                      value={form.reasoning_effort || 'off'}
                      onChange={e => setForm({ ...form, reasoning_effort: e.target.value })}
                    >
                      <option value="off">off</option>
                      <option value="low">low</option>
                      <option value="medium">medium</option>
                      <option value="high">high</option>
                      <option value="max">max</option>
                    </select>
                  </div>
                </div>
                <div className="form-check">
                  <input
                    type="checkbox"
                    id="model-vision"
                    checked={form.vision}
                    onChange={e => setForm({ ...form, vision: e.target.checked })}
                  />
                  <label htmlFor="model-vision" style={{ fontSize: 12 }}>Vision support</label>
                </div>
              </div>
              <div className="modal-foot">
                <button className="modal-btn" onClick={() => setShowForm(false)}>{t('modal.cancel')}</button>
                <button className="modal-btn primary" onClick={save}>{t('agents.save')}</button>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
