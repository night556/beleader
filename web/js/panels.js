// panels.js — History and Settings panels

function toggleSettings() {
  var panel = document.getElementById('settings-panel');
  if (panel.classList.contains('open')) { closePanels(); }
  else { openPanel('settings-panel'); loadSettings(); }
}

function openPanel(panelId) {
  document.getElementById(panelId).classList.add('open');
  document.getElementById('backdrop').classList.add('open');
}

function closePanels() {
  document.getElementById('settings-panel').classList.remove('open');
  document.getElementById('backdrop').classList.remove('open');
}

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') closePanels();
});

// ── Settings ──

var activeModelId = '';

var PROVIDERS = {
  openai:   { name: 'OpenAI',        base: 'https://api.openai.com/v1', keyLink: 'https://platform.openai.com/api-keys',
              models: ['gpt-5.5', 'gpt-5.4-mini', 'gpt-4.1', 'o4-mini', 'gpt-4o'] },
  google:   { name: 'Google Gemini', base: 'https://generativelanguage.googleapis.com/v1beta/openai', keyLink: 'https://aistudio.google.com/apikey',
              models: ['gemini-2.5-pro', 'gemini-2.5-flash', 'gemini-2.5-flash-lite'] },
  deepseek: { name: 'DeepSeek',      base: 'https://api.deepseek.com', keyLink: 'https://platform.deepseek.com/api_keys',
              models: ['deepseek-v4-pro', 'deepseek-v4-flash'] },
  groq:     { name: 'Groq',          base: 'https://api.groq.com/openai/v1', keyLink: 'https://console.groq.com/keys',
              models: ['meta-llama/llama-4-maverick-17b-128e-instruct', 'meta-llama/llama-4-scout-17b-16e-instruct', 'qwen/qwen3-32b', 'llama-3.1-8b-instant'] },
  ollama:   { name: 'Ollama',        base: 'http://localhost:11434/v1', keyLink: '',
              models: [] }
};

function onProviderChange(sel, idx) {
  var key = sel.value;
  var card = sel.closest('.model-card');
  var urlInput = card.querySelector('input[data-field="base_url"]');
  var modelSelect = card.querySelector('.model-preset-select');
  var keyLink = card.querySelector('.provider-key-link');

  if (key && PROVIDERS[key]) {
    var p = PROVIDERS[key];
    urlInput.value = p.base;
    modelSelect.innerHTML = '<option value="">Custom</option>';
    for (var i = 0; i < p.models.length; i++) {
      modelSelect.innerHTML += '<option value="' + escapeHtml(p.models[i]) + '">' + escapeHtml(p.models[i]) + '</option>';
    }
    if (p.keyLink) {
      if (!keyLink) {
        var link = document.createElement('a');
        link.className = 'provider-key-link';
        link.target = '_blank';
        link.textContent = '↗ keys';
        link.style.cssText = 'font-size:11px;color:var(--accent);margin-left:6px;text-decoration:none;white-space:nowrap';
        sel.parentNode.appendChild(link);
        keyLink = link;
      }
      keyLink.href = p.keyLink;
      keyLink.style.display = '';
    } else if (keyLink) {
      keyLink.style.display = 'none';
    }
  } else {
    urlInput.value = '';
    modelSelect.innerHTML = '<option value="">Custom</option>';
    if (keyLink) keyLink.style.display = 'none';
  }
}

function onModelPresetChange(sel, idx) {
  var card = sel.closest('.model-card');
  var modelInput = card.querySelector('input[data-field="model"]');
  if (sel.value) {
    modelInput.value = sel.value;
  }
}

function loadSettings() {
  fetch(SERVER_URL + '/api/settings')
    .then(function(r) { return r.json(); })
    .then(function(cfg) {
      document.getElementById('set-hc-max').value = cfg.hc && cfg.hc.max || 5;
      document.getElementById('set-context-pct').value = cfg.thresholds && cfg.thresholds.max_context_pct || 60;
      document.getElementById('set-headless').checked = cfg.browser && cfg.browser.headless || false;
      document.getElementById('set-speak-enabled').checked = cfg.speak_enabled !== false;
      if (cfg.llm) {
        var models = cfg.llm.models || [];
        activeModelId = cfg.llm.active || (models.length > 0 ? models[0].id : '');
        if (models.length === 0) {
          document.getElementById('model-list').innerHTML = '<div class="model-empty"><div class="model-empty-icon">⚡</div><div class="model-empty-text">' + t('timeline.no_models_setup_title') + '</div><div class="model-empty-hint">' + t('timeline.no_models_setup_hint') + '</div><button class="btn-add" onclick="addModel()" style="margin-top:8px">' + t('timeline.no_models_setup_btn') + '</button></div>';
        } else {
          renderModelList(models, activeModelId);
        }
      }
    })
    .catch(function(e) { console.error('loadSettings error:', e); });

  fetch(SERVER_URL + '/api/agents')
    .then(function(r) { return r.json(); })
    .then(function(agents) { renderAgentList(agents || []); })
    .catch(function(e) { console.error('loadAgents error:', e); });
}

var modelCounter = 0;

function renderModelList(models, activeId) {
  var container = document.getElementById('model-list');
  var html = '';
  modelCounter = 0;
  for (var i = 0; i < models.length; i++) {
    var m = models[i];
    html += modelRowHTML(m, modelCounter, m.id === activeId);
    modelCounter++;
  }
  container.innerHTML = html;
}

function modelRowHTML(m, idx, isActive) {
  var h = '';
  h += '<div class="model-card" data-model-idx="' + idx + '">';
  h += '  <div class="model-card-header">';
  h += '    <span class="model-name-display">' + escapeHtml(m.id || t('model.new')) + '</span>';
  h += '    <span class="model-card-actions">';
  if (isActive) {
    h += '      <span class="model-badge model-badge-active">active</span>';
  } else {
    h += '      <span class="model-badge" onclick="setActiveModel(\'' + escapeHtml(m.id) + '\')">set active</span>';
  }
  h += '      <span class="model-badge model-badge-delete" onclick="deleteModel(' + idx + ')">&times;</span>';
  h += '    </span>';
  h += '  </div>';
  h += '  <div class="model-card-body">';
  h += '    <div class="model-field"><label>' + t('model.id_label') + '</label><input value="' + escapeHtml(m.id || '') + '" data-model="' + idx + '" data-field="id" placeholder="' + t('model.id_placeholder') + '"></div>';
  h += '    <div class="model-field"><label>Provider</label><div class="provider-row"><select class="provider-select" onchange="onProviderChange(this,' + idx + ')"><option value="">Custom</option><option value="openai">OpenAI</option><option value="google">Google Gemini</option><option value="deepseek">DeepSeek</option><option value="groq">Groq</option><option value="ollama">Ollama</option></select></div></div>';
  h += '    <div class="model-field"><label>Base URL</label><input value="' + escapeHtml(m.base_url || '') + '" data-model="' + idx + '" data-field="base_url" placeholder="https://api.openai.com/v1"></div>';
  h += '    <div class="model-field"><label>API Key</label><input type="password" value="' + escapeHtml(m.api_key || '') + '" data-model="' + idx + '" data-field="api_key" placeholder="sk-..."></div>';
  h += '    <div class="model-field"><label>Model</label><div class="model-select-row"><select class="model-preset-select" onchange="onModelPresetChange(this,' + idx + ')"><option value="">Custom</option></select><input value="' + escapeHtml(m.model || '') + '" data-model="' + idx + '" data-field="model" placeholder="Enter model name..."></div></div>';
  h += '    <div class="model-field model-field-inline">';
  h += '      <label>Context Limit</label>';
  h += '      <input type="number" value="' + (m.context_limit || 128000) + '" data-model="' + idx + '" data-field="context_limit" min="4096" step="1024">';
  h += '    </div>';
  h += '    <div class="model-field model-field-inline">';
  h += '      <label>Vision</label>';
  h += '      <span class="toggle-switch">';
  h += '        <input type="checkbox" data-model="' + idx + '" data-field="vision"' + (m.vision ? ' checked' : '') + '>';
  h += '        <span class="toggle-slider"></span>';
  h += '      </span>';
  h += '    </div>';
  h += '  </div>';
  h += '</div>';
  return h;
}

function setActiveModel(id) {
  activeModelId = id;
  var models = collectModels();
  renderModelList(models, id);
}

function addModel() {
  var container = document.getElementById('model-list');
  var idx = modelCounter++;
  container.insertAdjacentHTML('beforeend', modelRowHTML({id: '', base_url: '', api_key: '', model: '', vision: false, context_limit: 128000}, idx, false));
}

function deleteModel(idx) {
  var card = document.querySelector('.model-card[data-model-idx="' + idx + '"]');
  if (!card) return;
  var idInput = card.querySelector('input[data-field="id"]');
  var modelId = idInput ? idInput.value : '';
  if (modelId && modelId === activeModelId) {
    alert(t('error.cannot_delete_active'));
    return;
  }
  card.remove();
}

function collectModels() {
  var cards = document.querySelectorAll('#model-list .model-card');
  var models = [];
  for (var i = 0; i < cards.length; i++) {
    var card = cards[i];
    var inputs = card.querySelectorAll('input');
    var d = {};
    for (var j = 0; j < inputs.length; j++) {
      var inp = inputs[j];
      var field = inp.getAttribute('data-field');
      if (!field) continue;
      if (inp.type === 'checkbox') {
        d[field] = inp.checked;
      } else if (inp.type === 'number') {
        d[field] = parseInt(inp.value) || 128000;
      } else {
        d[field] = inp.value;
      }
    }
    d.id = d.id || '';
    models.push(d);
  }
  return models;
}

function saveSettings() {
  var models = collectModels();
  if (models.length === 0) {
    alert(t('error.at_least_one_model'));
    return;
  }
  if (!activeModelId && models.length > 0) activeModelId = models[0].id;
  var body = {
    llm: { models: models, active: activeModelId },
    hc: { max: parseInt(document.getElementById('set-hc-max').value) || 5 },
    thresholds: { max_context_pct: parseInt(document.getElementById('set-context-pct').value) || 60 },
    browser: { headless: document.getElementById('set-headless').checked },
    speak_enabled: document.getElementById('set-speak-enabled').checked
  };
  fetch(SERVER_URL + '/api/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(function() {
    speakEnabled = document.getElementById('set-speak-enabled').checked;
    updateSpeakButton();
    if (!hasModels && models.length > 0) resetIdlePrompt();
    closePanels();
  })
    .catch(function(e) { console.error('saveSettings error:', e); });
}

function renderAgentList(agents) {
  var container = document.getElementById('agent-list');
  var html = '';
  for (var i = 0; i < agents.length; i++) {
    var a = agents[i];
    html += '<div class="form-row"><label>' + escapeHtml(a.name || a.id) + '</label><span style="font-size:10px;color:var(--text-dim)">' + escapeHtml((a.description || '').substring(0, 40)) + '</span></div>';
  }
  if (!agents.length) html = '<div style="font-size:12px;color:var(--text-dim);padding:8px 0;">No agents configured</div>';
  container.innerHTML = html;
}

