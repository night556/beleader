// panels.js — History and Settings panels

function toggleSettings() {
  var panel = document.getElementById('settings-panel');
  if (panel.classList.contains('open')) { closePanels(); }
  else {
    var bp = document.getElementById('bookmarks-panel');
    var kp = document.getElementById('knowledge-panel');
    var ap = document.getElementById('agents-panel');
    if (bp) bp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    if (ap) ap.classList.remove('open');
    openPanel('settings-panel');
    loadSettings();
  }
}

function toggleBookmarks() {
  var panel = document.getElementById('bookmarks-panel');
  if (panel.classList.contains('open')) { closePanels(); }
  else {
    var sp = document.getElementById('settings-panel');
    var kp = document.getElementById('knowledge-panel');
    var ap = document.getElementById('agents-panel');
    if (sp) sp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    if (ap) ap.classList.remove('open');
    openPanel('bookmarks-panel');
    loadBookmarks();
  }
}

function toggleAgents() {
  var panel = document.getElementById('agents-panel');
  if (panel.classList.contains('open')) { closePanels(); }
  else {
    var sp = document.getElementById('settings-panel');
    var bp = document.getElementById('bookmarks-panel');
    var kp = document.getElementById('knowledge-panel');
    if (sp) sp.classList.remove('open');
    if (bp) bp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    openPanel('agents-panel');
    loadAgents();
  }
}

function openPanel(panelId) {
  document.getElementById(panelId).classList.add('open');
  document.getElementById('backdrop').classList.add('open');
}

function closePanels() {
  document.getElementById('settings-panel').classList.remove('open');
  document.getElementById('bookmarks-panel').classList.remove('open');
  document.getElementById('knowledge-panel').classList.remove('open');
  var ap = document.getElementById('agents-panel');
  if (ap) ap.classList.remove('open');
  document.getElementById('backdrop').classList.remove('open');
}

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') {
    var mb = document.getElementById('modal-backdrop');
    if (mb && mb.style.display !== 'none') {
      closeModal();
    } else {
      closePanels();
    }
  }
});

// ── Modal component (shared) ──

var _modalOnConfirm = null;

function openModal(opts) {
  var backdrop = document.getElementById('modal-backdrop');
  var dialog = document.getElementById('modal-dialog');
  document.getElementById('modal-title').textContent = opts.title || '';
  document.getElementById('modal-body').innerHTML = opts.body || '';
  dialog.classList.toggle('wide', !!opts.wide);

  var foot = document.getElementById('modal-foot');
  var html = '';
  html += '<button class="modal-btn" onclick="closeModal()">' + (opts.cancelText || t('modal.cancel')) + '</button>';
  html += '<button class="modal-btn ' + (opts.danger ? 'danger' : 'primary') + '" id="modal-confirm-btn">' + (opts.confirmText || 'OK') + '</button>';
  foot.innerHTML = html;

  _modalOnConfirm = opts.onConfirm || null;
  var confirmBtn = document.getElementById('modal-confirm-btn');
  confirmBtn.onclick = function() {
    if (_modalOnConfirm) {
      var result = _modalOnConfirm();
      if (result !== false) closeModal();
    } else {
      closeModal();
    }
  };

  backdrop.style.display = 'flex';
  setTimeout(function() { backdrop.classList.add('open'); }, 10);

  if (opts.onOpen) setTimeout(opts.onOpen, 50);
}

function closeModal() {
  var backdrop = document.getElementById('modal-backdrop');
  backdrop.classList.remove('open');
  setTimeout(function() { backdrop.style.display = 'none'; }, 200);
  _modalOnConfirm = null;
}

document.getElementById('modal-backdrop').addEventListener('click', function(e) {
  if (e.target === this) closeModal();
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
      renderPortMapList(cfg.port_maps || []);
      if (cfg.llm) {
        var models = cfg.llm.models || [];
        activeModelId = cfg.llm.active || (models.length > 0 ? models[0].id : '');
        updateContextModel(activeModelId);
        if (models.length === 0) {
          document.getElementById('model-list').innerHTML = '<div class="model-empty"><div class="model-empty-icon">⚡</div><div class="model-empty-text">' + t('timeline.no_models_setup_title') + '</div><div class="model-empty-hint">' + t('timeline.no_models_setup_hint') + '</div><button class="btn-add" onclick="addModel()" style="margin-top:8px">' + t('timeline.no_models_setup_btn') + '</button></div>';
        } else {
          renderModelList(models, activeModelId);
        }
      }
    })
    .catch(function(e) { console.error('loadSettings error:', e); });
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
  updateContextModel(id);
}

function addModel() {
  var container = document.getElementById('model-list');
  var empty = container.querySelector('.model-empty');
  if (empty) container.innerHTML = '';
  var idx = modelCounter++;
  container.insertAdjacentHTML('afterbegin', modelRowHTML({id: '', base_url: '', api_key: '', model: '', vision: false, context_limit: 128000}, idx, false));
  var firstInput = container.querySelector('.model-card input[data-field="id"]');
  if (firstInput) firstInput.focus();
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
    speak_enabled: document.getElementById('set-speak-enabled').checked,
    port_maps: collectPortMaps()
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

// ── Agents panel CRUD ──

function loadAgents() {
  fetch(SERVER_URL + '/api/agents')
    .then(function(r) { return r.json(); })
    .then(function(agents) {
      _agentsCache = agents || [];
      renderAgentListFiltered();
    })
    .catch(function(e) { console.error('loadAgents error:', e); });
}

function renderAgentListFiltered() {
  var container = document.getElementById('agents-list');
  if (!container) return;
  var searchInput = document.getElementById('agents-search');
  var q = searchInput ? searchInput.value.trim().toLowerCase() : '';
  var filtered = _agentsCache.filter(function(a) {
    if (!q) return true;
    return (a.name || '').toLowerCase().indexOf(q) >= 0 || (a.desc || '').toLowerCase().indexOf(q) >= 0;
  });

  if (filtered.length === 0) {
    container.innerHTML = '<div class="agents-empty">' + t('agents.empty') + '</div>';
    return;
  }

  var html = '';
  for (var i = 0; i < filtered.length; i++) {
    var a = filtered[i];
    var preview = (a.content || '').substring(0, 200);
    html += '<div class="agent-card" data-agent-id="' + a.id + '">';
    html += '<div class="agent-card-head">';
    html += '<span class="agent-card-name">' + escapeHtml(a.name || '') + '</span>';
    html += '<span class="agent-card-desc">' + escapeHtml(a.desc || '') + '</span>';
    html += '<span class="agent-card-actions">';
    html += '<button class="agent-card-btn" onclick="openAgentEditor(' + a.id + ')" title="' + t('agents.edit') + '">✎</button>';
    html += '<button class="agent-card-btn delete" onclick="deleteAgent(' + a.id + ',\'' + escapeHtml(a.name || '') + '\')" title="' + t('agents.delete') + '">✕</button>';
    html += '</span>';
    html += '</div>';
    html += '<div class="agent-card-preview">' + escapeHtml(preview) + '</div>';
    html += '</div>';
  }
  container.innerHTML = html;
}

function openAgentEditor(id) {
  var agent = null;
  if (id != null) {
    for (var i = 0; i < _agentsCache.length; i++) {
      if (_agentsCache[i].id === id) { agent = _agentsCache[i]; break; }
    }
  }

  var title = agent ? t('agents.edit_title') : t('agents.new_title');
  var body =
    '<div class="modal-field"><label>' + t('agents.name') + '</label>' +
    '<input type="text" id="agent-name-input" class="modal-input" value="' + escapeHtml(agent ? agent.name : '') + '"></div>' +
    '<div class="modal-field"><label>' + t('agents.desc') + '</label>' +
    '<input type="text" id="agent-desc-input" class="modal-input" value="' + escapeHtml(agent ? agent.desc : '') + '"></div>' +
    '<div class="modal-field"><label>' + t('agents.content') + '</label>' +
    '<textarea id="agent-content-input" class="modal-textarea">' + escapeHtml(agent ? agent.content : '') + '</textarea></div>';

  openModal({
    title: title,
    body: body,
    wide: true,
    confirmText: t('agents.save'),
    onOpen: function() {
      var ta = document.getElementById('agent-content-input');
      if (ta) {
        ta.addEventListener('keydown', function(e) {
          if (e.key === 'Tab') {
            e.preventDefault();
            var start = ta.selectionStart, end = ta.selectionEnd;
            ta.value = ta.value.substring(0, start) + '  ' + ta.value.substring(end);
            ta.selectionStart = ta.selectionEnd = start + 2;
          }
        });
      }
    },
    onConfirm: function() {
      var name = document.getElementById('agent-name-input').value.trim();
      var desc = document.getElementById('agent-desc-input').value.trim();
      var content = document.getElementById('agent-content-input').value;
      if (!name || !content) {
        toast(name ? t('agents.content') + ' required' : t('agents.name') + ' required');
        return false;
      }
      var payload = {name: name, desc: desc, content: content};
      if (agent) {
        fetch(SERVER_URL + '/api/agents/' + agent.id, {
          method: 'PUT',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(payload)
        }).then(function(r) { return r.json(); })
          .then(function() { loadAgents(); toast(t('agents.edit') + ' ✓'); })
          .catch(function(e) { console.error('update agent error:', e); toast('Error: ' + e.message); });
      } else {
        fetch(SERVER_URL + '/api/agents', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify(payload)
        }).then(function(r) { return r.json(); })
          .then(function() { loadAgents(); toast(t('agents.new') + ' ✓'); })
          .catch(function(e) { console.error('create agent error:', e); toast('Error: ' + e.message); });
      }
      return true;
    }
  });
}

function deleteAgent(id, name) {
  openModal({
    title: t('agents.delete'),
    body: '<div class="modal-confirm-text"><p>' + t('agents.delete_confirm').replace('$1', escapeHtml(name)) + '</p></div>',
    confirmText: t('agents.delete_btn'),
    danger: true,
    onConfirm: function() {
      fetch(SERVER_URL + '/api/agents/' + id, {method: 'DELETE'})
        .then(function(r) { return r.json(); })
        .then(function() { loadAgents(); toast(t('agents.delete') + ' ✓'); })
        .catch(function(e) { console.error('delete agent error:', e); toast('Error: ' + e.message); });
      return true;
    }
  });
}

// Legacy stub for backward compat — no longer renders into settings
function renderAgentList(agents) { _agentsCache = agents || []; }

// ── Port Maps ──

var _portMapCounter = 0;

function renderPortMapList(portMaps) {
  var container = document.getElementById('port-map-list');
  if (!portMaps || portMaps.length === 0) {
    container.innerHTML = '<div style="font-size:12px;color:var(--text-dim);padding:8px 0">No port maps configured</div>';
    return;
  }
  _portMapCounter = portMaps.length;
  var html = '';
  for (var i = 0; i < portMaps.length; i++) {
    var pm = portMaps[i];
    var port = pm.local_port || '';
    var name = pm.name || '';
    var directUrl = 'http://127.0.0.1:' + port;
    html += '<div class="form-row" style="display:flex;align-items:center;gap:6px;margin-bottom:6px">';
    html += '<input placeholder="Name" value="' + escapeHtml(name) + '" data-pm="' + i + '" data-field="pm-name" style="flex:1;min-width:0">';
    html += '<span style="color:var(--text-dim)">:</span>';
    html += '<input type="number" placeholder="Port" value="' + port + '" data-pm="' + i + '" data-field="pm-port" min="1" max="65535" style="width:80px;min-width:0">';
    html += '<a href="' + directUrl + '" target="_blank" title="Open in browser" style="color:var(--primary);text-decoration:none;font-size:13px">↗</a>';
    html += '<span class="model-badge model-badge-delete" onclick="this.closest(\'.form-row\').remove()">&times;</span>';
    html += '</div>';
  }
  container.innerHTML = html;
}

function addPortMap() {
  var container = document.getElementById('port-map-list');
  var idx = _portMapCounter++;
  var html = '<div class="form-row" style="display:flex;align-items:center;gap:6px;margin-bottom:6px">';
  html += '<input placeholder="Name" data-pm="' + idx + '" data-field="pm-name" style="flex:1;min-width:0">';
  html += '<span style="color:var(--text-dim)">:</span>';
  html += '<input type="number" placeholder="Port" data-pm="' + idx + '" data-field="pm-port" min="1" max="65535" style="width:80px;min-width:0">';
  html += '<span class="model-badge model-badge-delete" onclick="this.closest(\'.form-row\').remove()">&times;</span>';
  html += '</div>';
  container.insertAdjacentHTML('beforeend', html);
}

function collectPortMaps() {
  var rows = document.querySelectorAll('#port-map-list .form-row');
  var maps = [];
  for (var i = 0; i < rows.length; i++) {
    var nameInput = rows[i].querySelector('[data-field="pm-name"]');
    var portInput = rows[i].querySelector('[data-field="pm-port"]');
    var name = (nameInput && nameInput.value || '').trim();
    var port = parseInt(portInput && portInput.value || '0') || 0;
    if (name && port > 0 && port <= 65535) {
      maps.push({ name: name, local_port: port });
    }
  }
  return maps;
}

function loadBookmarks() {
  var projectId = currentView === 'home' ? null : currentView;
  if (!projectId) {
    document.getElementById('bookmarks-body').innerHTML = '<div class="bookmarks-empty">请在项目中收藏消息</div>';
    return;
  }
  fetch(SERVER_URL + '/api/messages/bookmarked?project_id=' + encodeURIComponent(projectId))
    .then(function(r) { return r.json(); })
    .then(function(msgs) {
      var body = document.getElementById('bookmarks-body');
      if (!Array.isArray(msgs) || msgs.length === 0) {
        body.innerHTML = '<div class="bookmarks-empty">暂无收藏</div>';
        return;
      }
      var roleLabels = { user: 'You', assistant: 'AI', tool: 'Tool' };
      var html = '';
      for (var i = 0; i < msgs.length; i++) {
        var m = msgs[i];
        var text = (m.content || '').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        var preview = text.length > 200 ? text.substring(0, 200) + '...' : text;
        var time = new Date(m.created_at).toLocaleString();
        html += '<div class="bkm-item" onclick="scrollToMessage(' + m.id + ')">' +
          '<span class="bkm-star">★</span>' +
          '<div class="bkm-body">' +
            '<div class="bkm-role">' + (roleLabels[m.role] || m.role) + ' · ' + time + '</div>' +
            '<div class="bkm-text">' + preview + '</div>' +
          '</div>' +
          '<button class="bkm-unstar" onclick="event.stopPropagation();toggleMessageBookmark(' + m.id + ', false)" title="取消收藏">✕</button>' +
        '</div>';
      }
      body.innerHTML = html;
    })
    .catch(function(err) { console.error('load bookmarks error:', err); });
}

function toggleMessageBookmark(msgId, bookmarked) {
  fetch(SERVER_URL + '/api/messages/' + msgId + '/bookmark', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ bookmarked: bookmarked })
  }).then(function() {
    // Update star icon in timeline if message is visible
    var stars = document.querySelectorAll('.msg-star[data-msg-id="' + msgId + '"]');
    for (var i = 0; i < stars.length; i++) {
      if (bookmarked) {
        stars[i].classList.add('bookmarked');
        stars[i].textContent = '★';
      } else {
        stars[i].classList.remove('bookmarked');
        stars[i].textContent = '☆';
      }
    }
  }).catch(function(err) { console.error('bookmark error:', err); });
}

function scrollToMessage(msgId) {
  // Try to find message in current timeline, or suggest loading history
  closePanels();
  // Seek the turn that contains this message
  var el = document.querySelector('.msg-star[data-msg-id="' + msgId + '"]');
  if (el) {
    el.scrollIntoView({ behavior: 'smooth', block: 'center' });
    el.style.transition = 'transform 0.3s';
    el.style.transform = 'scale(1.4)';
    setTimeout(function() { el.style.transform = ''; }, 600);
  }
}
