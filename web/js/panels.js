// panels.js — History and Settings panels

function toggleSettings() {
  var panel = document.getElementById('settings-panel');
  if (panel.classList.contains('open')) { closePanels(); }
  else {
    var bp = document.getElementById('bookmarks-panel');
    var kp = document.getElementById('knowledge-panel');
    var ap = document.getElementById('agents-panel');
    var tp = document.getElementById('tools-panel');
    var mp = document.getElementById('mcp-panel');
    if (bp) bp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    if (ap) ap.classList.remove('open');
    if (tp) tp.classList.remove('open');
    if (mp) mp.classList.remove('open');
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
    var tp = document.getElementById('tools-panel');
    var mp = document.getElementById('mcp-panel');
    if (sp) sp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    if (ap) ap.classList.remove('open');
    if (tp) tp.classList.remove('open');
    if (mp) mp.classList.remove('open');
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
    var tp = document.getElementById('tools-panel');
    var mp = document.getElementById('mcp-panel');
    if (sp) sp.classList.remove('open');
    if (bp) bp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    if (tp) tp.classList.remove('open');
    if (mp) mp.classList.remove('open');
    openPanel('agents-panel');
    loadAgents();
  }
}

function toggleTools() {
  var panel = document.getElementById('tools-panel');
  if (panel.classList.contains('open')) { closePanels(); }
  else {
    var sp = document.getElementById('settings-panel');
    var bp = document.getElementById('bookmarks-panel');
    var kp = document.getElementById('knowledge-panel');
    var ap = document.getElementById('agents-panel');
    var mp = document.getElementById('mcp-panel');
    if (sp) sp.classList.remove('open');
    if (bp) bp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    if (ap) ap.classList.remove('open');
    if (mp) mp.classList.remove('open');
    openPanel('tools-panel');
    loadTools();
  }
}

function loadTools() {
  fetch(SERVER_URL + '/api/tools')
    .then(function(r) { return r.json(); })
    .then(function(tools) {
      var container = document.getElementById('tools-list');
      if (!container) return;
      if (!tools || tools.length === 0) {
        container.innerHTML = '<div class="agents-empty">No tools registered</div>';
        return;
      }
      var html = '';
      for (var i = 0; i < tools.length; i++) {
        var t = tools[i];
        var hasParams = t.parameters && t.parameters.properties && Object.keys(t.parameters.properties).length > 0;
        var reqProps = (t.parameters && t.parameters.required) || [];
        var srcBadge = '';
        if (t.source === 'mcp') {
          srcBadge = '<span class="tool-source-badge mcp">MCP</span>';
        } else if (t.source === 'builtin') {
          srcBadge = '<span class="tool-source-badge builtin">内置</span>';
        }
        html += '<div class="tool-card">' +
          '<div class="tool-card-header" onclick="this.parentElement.classList.toggle(\'open\')">' +
            '<div class="tool-card-top">' +
              '<span class="tool-card-chevron">▶</span>' +
              '<span class="tool-card-name">' + escapeHtml(t.name) + '</span>' +
              srcBadge +
              (hasParams ? '<span class="tool-card-params-hint">' + Object.keys(t.parameters.properties).length + ' params</span>' : '') +
            '</div>' +
            '<span class="tool-card-desc">' + escapeHtml(t.description || '') + '</span>' +
          '</div>';
        if (hasParams) {
          html += '<div class="tool-card-body"><table class="tool-params-table"><thead><tr><th>Param</th><th>Type</th><th>Required</th><th>Description</th></tr></thead><tbody>';
          var props = t.parameters.properties;
          var keys = Object.keys(props);
          for (var j = 0; j < keys.length; j++) {
            var p = props[keys[j]];
            var ptype = p.type || '';
            if (p.enum) ptype += ' (' + p.enum.join('|') + ')';
            html += '<tr><td><code>' + escapeHtml(keys[j]) + '</code></td><td>' + escapeHtml(ptype) + '</td><td>' + (reqProps.indexOf(keys[j]) >= 0 ? '✓' : '') + '</td><td>' + escapeHtml(p.description || '') + '</td></tr>';
          }
          html += '</tbody></table></div>';
        }
        html += '</div>';
      }
      container.innerHTML = html;
    })
    .catch(function(e) { console.error('loadTools error:', e); });
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
  var tp = document.getElementById('tools-panel');
  if (tp) tp.classList.remove('open');
  var mp = document.getElementById('mcp-panel');
  if (mp) mp.classList.remove('open');
  if (window.innerWidth <= 860) document.body.classList.add('sb-closed');
  document.getElementById('backdrop').classList.remove('open');
}

// ── MCP Panel ──

var _mcpServersCache = [];

function toggleMCP() {
  var panel = document.getElementById('mcp-panel');
  if (panel.classList.contains('open')) { closePanels(); }
  else {
    var sp = document.getElementById('settings-panel');
    var bp = document.getElementById('bookmarks-panel');
    var kp = document.getElementById('knowledge-panel');
    var ap = document.getElementById('agents-panel');
    var tp = document.getElementById('tools-panel');
    if (sp) sp.classList.remove('open');
    if (bp) bp.classList.remove('open');
    if (kp) kp.classList.remove('open');
    if (ap) ap.classList.remove('open');
    if (tp) tp.classList.remove('open');
    openPanel('mcp-panel');
    loadMCP();
  }
}

function loadMCP() {
  fetch(SERVER_URL + '/api/mcp/servers')
    .then(function(r) { return r.json(); })
    .then(function(servers) {
      _mcpServersCache = servers || [];
      renderMCPList(_mcpServersCache);
    })
    .catch(function(e) { console.error('loadMCP error:', e); });
}

function renderMCPListFiltered() {
  var q = (document.getElementById('mcp-search') || {}).value || '';
  if (!q) { renderMCPList(_mcpServersCache); return; }
  q = q.toLowerCase();
  var filtered = _mcpServersCache.filter(function(s) {
    return s.name.toLowerCase().indexOf(q) >= 0;
  });
  renderMCPList(filtered);
}

function renderMCPList(servers) {
  var container = document.getElementById('mcp-list');
  if (!container) return;
  if (!servers || servers.length === 0) {
    container.innerHTML = '<div class="agents-empty">No MCP servers configured</div>';
    return;
  }
  var html = '';
  for (var i = 0; i < servers.length; i++) {
    var s = servers[i];
    var statusCls = s.status === 'connected' ? 'mcp-status-connected' : (s.status === 'error' ? 'mcp-status-error' : 'mcp-status-disconnected');
    var statusLabel = s.status === 'connected' ? 'Connected' : (s.status === 'error' ? 'Error' : 'Disconnected');
    html += '<div class="mcp-server-card">' +
      '<div class="mcp-server-header">' +
        '<span class="mcp-status-dot ' + statusCls + '" title="' + escapeHtml(statusLabel) + '"></span>' +
        '<div class="mcp-server-info">' +
          '<span class="mcp-server-name">' + escapeHtml(s.name) + '</span>' +
          '<span class="mcp-server-type">' + escapeHtml(s.type) + '</span>' +
        '</div>' +
        '<div class="mcp-server-actions">';
    if (s.status === 'disconnected' || s.status === 'error') {
      html += '<button class="mcp-btn mcp-btn-connect" onclick="connectMCPServer(' + s.id + ')" data-i18n="mcp.connect">Connect</button>';
    } else if (s.status === 'connected') {
      html += '<button class="mcp-btn mcp-btn-disconnect" onclick="disconnectMCPServer(' + s.id + ')" data-i18n="mcp.disconnect">Disconnect</button>';
    }
    html += '<button class="mcp-btn mcp-btn-test" onclick="testMCPServer(' + s.id + ')" data-i18n="mcp.test">Test</button>' +
      '<button class="mcp-btn mcp-btn-edit" onclick="openMCPEditor(' + s.id + ')" data-i18n="mcp.edit">Edit</button>' +
      '<button class="mcp-btn mcp-btn-del" onclick="deleteMCPServer(' + s.id + ')" data-i18n="mcp.delete">Delete</button>' +
        '</div>' +
      '</div>';
    if (s.error) {
      html += '<div class="mcp-server-error">' + escapeHtml(s.error) + '</div>';
    }
    html += '</div>';
  }
  container.innerHTML = html;
}

// ── MCP Editor helpers ──

// parseArgs splits a command-line string into tokens respecting single and double quotes.
function parseArgs(raw) {
  var tokens = [];
  var current = '';
  var inSingle = false;
  var inDouble = false;
  for (var i = 0; i < raw.length; i++) {
    var ch = raw[i];
    if (inDouble) {
      if (ch === '"') { inDouble = false; }
      else { current += ch; }
    } else if (inSingle) {
      if (ch === "'") { inSingle = false; }
      else { current += ch; }
    } else {
      if (ch === '"') { inDouble = true; }
      else if (ch === "'") { inSingle = true; }
      else if (ch === ' ' || ch === '\t') {
        if (current) { tokens.push(current); current = ''; }
      } else { current += ch; }
    }
  }
  if (current) tokens.push(current);
  return tokens;
}

function mcpBuildKVRows(prefix, entries) {
  var html = '<div class="mcp-kv-list" id="' + prefix + '-list">';
  for (var i = 0; i < entries.length; i++) {
    html += '<div class="mcp-kv-row">' +
      '<input class="mcp-kv-key" placeholder="Key" value="' + escapeHtml(entries[i].key) + '">' +
      '<input class="mcp-kv-val" placeholder="Value" value="' + escapeHtml(entries[i].val) + '">' +
      '<button class="mcp-kv-remove" onclick="this.parentNode.remove()" title="Remove">&times;</button>' +
      '</div>';
  }
  html += '</div>' +
    '<button class="mcp-kv-add" onclick="mcpAddKvRow(\'' + prefix + '\')">+ Add</button>';
  return html;
}

function mcpAddKvRow(prefix) {
  var list = document.getElementById(prefix + '-list');
  var row = document.createElement('div');
  row.className = 'mcp-kv-row';
  row.innerHTML = '<input class="mcp-kv-key" placeholder="Key">' +
    '<input class="mcp-kv-val" placeholder="Value">' +
    '<button class="mcp-kv-remove" onclick="this.parentNode.remove()" title="Remove">&times;</button>';
  list.appendChild(row);
}

function mcpCollectKV(prefix) {
  var list = document.getElementById(prefix + '-list');
  if (!list) return '{}';
  var rows = list.querySelectorAll('.mcp-kv-row');
  var obj = {};
  for (var i = 0; i < rows.length; i++) {
    var k = rows[i].querySelector('.mcp-kv-key').value.trim();
    var v = rows[i].querySelector('.mcp-kv-val').value.trim();
    if (k) obj[k] = v;
  }
  return JSON.stringify(obj);
}

// ── MCP Editor ──

function openMCPEditor(id) {
  var isNew = (id === null || id === undefined);
  var server = null;
  if (!isNew) {
    for (var i = 0; i < _mcpServersCache.length; i++) {
      if (_mcpServersCache[i].id === id) { server = _mcpServersCache[i]; break; }
    }
  }
  var title = isNew ? 'New MCP Server' : 'Edit MCP Server';
  var defaultType = server ? server.type : 'stdio';

  // Parse env entries
  var envEntries = [];
  if (server) {
    try { var envObj = JSON.parse(server.env); for (var k in envObj) { envEntries.push({key: k, val: envObj[k]}); } } catch(e) {}
  }
  // Parse header entries
  var headerEntries = [];
  if (server) {
    try { var hdrsObj = JSON.parse(server.headers); for (var k in hdrsObj) { headerEntries.push({key: k, val: hdrsObj[k]}); } } catch(e) {}
  }
  // Parse args
  var argsStr = '';
  if (server) {
    try { argsStr = JSON.parse(server.args).join(' '); } catch(e) { argsStr = server.args || ''; }
  }

  var body =
    '<div class="mcp-editor">' +
      '<div class="mcp-editor-field">' +
        '<label>Name</label>' +
        '<input class="modal-input" id="mcp-edit-name" placeholder="e.g. my-server" value="' + escapeHtml(server ? server.name : '') + '">' +
      '</div>' +
      '<div class="mcp-editor-row">' +
        '<div class="mcp-editor-field mcp-editor-type">' +
          '<label>Type</label>' +
          '<select class="modal-select" id="mcp-edit-type" onchange="mcpEditTypeChange()">' +
            '<option value="stdio"' + (defaultType === 'stdio' ? ' selected' : '') + '>stdio</option>' +
            '<option value="http"' + (defaultType === 'http' ? ' selected' : '') + '>HTTP</option>' +
          '</select>' +
        '</div>' +
        '<div class="mcp-editor-field mcp-editor-enabled">' +
          '<label>&nbsp;</label>' +
          '<label class="mcp-checkbox-label"><input type="checkbox" id="mcp-edit-enabled"' + (server && server.enabled ? ' checked' : '') + '> Enable</label>' +
        '</div>' +
      '</div>' +

      '<div id="mcp-edit-stdio" class="mcp-editor-section" style="display:' + (defaultType === 'stdio' ? '' : 'none') + '">' +
        '<div class="mcp-editor-field">' +
          '<label>Command</label>' +
          '<input class="modal-input" id="mcp-edit-command" placeholder="e.g. npx or python" value="' + escapeHtml(server ? server.command : '') + '">' +
        '</div>' +
        '<div class="mcp-editor-field">' +
          '<label>Args</label>' +
          '<input class="modal-input" id="mcp-edit-args" placeholder="e.g. -y @modelcontextprotocol/server-filesystem /tmp" value="' + escapeHtml(argsStr) + '">' +
          '<div class="modal-field-hint">Space-separated arguments</div>' +
        '</div>' +
        '<div class="mcp-editor-field">' +
          '<label>Environment Variables</label>' +
          mcpBuildKVRows('mcp-env', envEntries) +
        '</div>' +
      '</div>' +

      '<div id="mcp-edit-http" class="mcp-editor-section" style="display:' + (defaultType === 'http' ? '' : 'none') + '">' +
        '<div class="mcp-editor-field">' +
          '<label>URL</label>' +
          '<input class="modal-input" id="mcp-edit-url" placeholder="https://example.com/mcp" value="' + escapeHtml(server ? server.url : '') + '">' +
        '</div>' +
        '<div class="mcp-editor-field">' +
          '<label>Headers</label>' +
          mcpBuildKVRows('mcp-hdrs', headerEntries) +
        '</div>' +
      '</div>' +
    '</div>';

  openModal({
    title: title,
    body: body,
    wide: true,
    confirmText: 'Save',
    onConfirm: function() {
      var name = document.getElementById('mcp-edit-name').value.trim();
      if (!name) { alert('Name required'); return false; }
      var srvType = document.getElementById('mcp-edit-type').value;
      var payload = {
        name: name,
        type: srvType,
        enabled: document.getElementById('mcp-edit-enabled').checked
      };
      if (srvType === 'stdio') {
        payload.command = document.getElementById('mcp-edit-command').value.trim();
        var argsRaw = document.getElementById('mcp-edit-args').value.trim();
        payload.args = argsRaw ? JSON.stringify(parseArgs(argsRaw)) : '[]';
        payload.env = mcpCollectKV('mcp-env');
      } else {
        payload.url = document.getElementById('mcp-edit-url').value.trim();
        payload.headers = mcpCollectKV('mcp-hdrs');
      }
      var url = SERVER_URL + '/api/mcp/servers';
      var method = 'POST';
      if (!isNew) {
        url += '/' + id;
        method = 'PUT';
      }
      fetch(url, { method: method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
        .then(function(r) { return r.json(); })
        .then(function() { closeModal(); loadMCP(); })
        .catch(function(e) { console.error('save MCP error:', e); });
      return false;
    }
  });

  window.mcpEditTypeChange = function() {
    var t = document.getElementById('mcp-edit-type').value;
    var stdioEl = document.getElementById('mcp-edit-stdio');
    var httpEl = document.getElementById('mcp-edit-http');
    if (stdioEl) stdioEl.style.display = (t === 'stdio' ? '' : 'none');
    if (httpEl) httpEl.style.display = (t === 'http' ? '' : 'none');
  };
}

function testMCPServer(id) {
  fetch(SERVER_URL + '/api/mcp/servers/' + id + '/test', { method: 'POST' })
    .then(function(r) { return r.json(); })
    .then(function(data) {
      if (data.success) {
        var msg = 'Connection OK\n\nTools found: ' + data.tool_count + '\n\n' + (data.tools || []).map(function(t) { return '• ' + t; }).join('\n');
        alert(msg);
      } else {
        alert('Test failed: ' + (data.error || 'unknown error'));
      }
    })
    .catch(function(e) { alert('Test error: ' + e); });
}

function connectMCPServer(id) {
  fetch(SERVER_URL + '/api/mcp/servers/' + id + '/connect', { method: 'POST' })
    .then(function(r) { return r.json(); })
    .then(function() { loadMCP(); })
    .catch(function(e) { console.error('connect MCP error:', e); });
}

function disconnectMCPServer(id) {
  fetch(SERVER_URL + '/api/mcp/servers/' + id + '/disconnect', { method: 'POST' })
    .then(function(r) { return r.json(); })
    .then(function() { loadMCP(); })
    .catch(function(e) { console.error('disconnect MCP error:', e); });
}

function deleteMCPServer(id) {
  if (!confirm('Delete this MCP server?')) return;
  fetch(SERVER_URL + '/api/mcp/servers/' + id, { method: 'DELETE' })
    .then(function(r) { return r.json(); })
    .then(function() { loadMCP(); })
    .catch(function(e) { console.error('delete MCP error:', e); });
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
  var confirmBtn = '<button class="modal-btn ' + (opts.danger ? 'danger' : 'primary') + '" id="modal-confirm-btn">' + (opts.confirmText || 'OK') + '</button>';
  var cancelBtn = '<button class="modal-btn" onclick="closeModal()">' + (opts.cancelText || t('modal.cancel')) + '</button>';
  // Danger modals: confirm on left so user reads it first
  html += opts.danger ? confirmBtn + cancelBtn : cancelBtn + confirmBtn;
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
  h += '    <div class="model-field"><label>' + t('model.base_url') + '</label><input value="' + escapeHtml(m.base_url || '') + '" data-model="' + idx + '" data-field="base_url" placeholder="' + t('model.base_url_placeholder') + '"></div>';
  h += '    <div class="model-field"><label>' + t('model.api_key') + '</label><input type="password" value="' + escapeHtml(m.api_key || '') + '" data-model="' + idx + '" data-field="api_key" placeholder="sk-..."></div>';
  h += '    <div class="model-field"><label>' + t('model.model_select') + '</label><div class="model-select-row"><select class="model-preset-select" onchange="onModelPresetChange(this,' + idx + ')"><option value="">Custom</option></select><input value="' + escapeHtml(m.model || '') + '" data-model="' + idx + '" data-field="model" placeholder="' + t('model.model_placeholder') + '"></div></div>';
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
    llm: { models: models, active: activeModelId }
  };
  fetch(SERVER_URL + '/api/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(function() {
    if (!hasModels && models.length > 0) resetIdlePrompt();
    closePanels();
  })
    .catch(function(e) { console.error('saveSettings error:', e); });
}

// ── Agents panel CRUD ──

var _toolsCache = [];

function loadAgents() {
  Promise.all([
    fetch(SERVER_URL + '/api/agents').then(function(r) { return r.json(); }),
    fetch(SERVER_URL + '/api/tools').then(function(r) { return r.json(); })
  ]).then(function(results) {
    _agentsCache = results[0] || [];
    _toolsCache = results[1] || [];
    renderAgentListFiltered();
  }).catch(function(e) { console.error('loadAgents error:', e); });
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

    // Parse tools
    var toolNames = [];
    try { if (a.tools) toolNames = JSON.parse(a.tools); } catch(e) {}

    // Build tool chips with descriptions
    var toolsHtml = '';
    for (var j = 0; j < toolNames.length; j++) {
      var desc = '';
      for (var k = 0; k < _toolsCache.length; k++) {
        if (_toolsCache[k].name === toolNames[j]) { desc = _toolsCache[k].description || ''; break; }
      }
      toolsHtml += '<span class="agent-tool-chip" title="' + escapeHtml(desc) + '">' + escapeHtml(toolNames[j]) + '</span>';
    }

    html += '<div class="agent-card" data-agent-id="' + a.id + '">';
    html += '<div class="agent-card-head">';
    html += '<span class="agent-card-name">' + escapeHtml(a.name || '') + '</span>';

    html += '<span class="agent-card-desc">' + escapeHtml(a.desc || '') + '</span>';
    html += '<span class="agent-card-actions">';
    html += '<button class="agent-card-btn" onclick="openAgentEditor(' + a.id + ')" title="' + t('agents.edit') + '">✎</button>';
    html += '<button class="agent-card-btn delete" onclick="deleteAgent(' + a.id + ',\'' + escapeHtml(a.name || '') + '\')" title="' + t('agents.delete') + '">✕</button>';
    html += '</span>';
    html += '</div>';
    if (toolsHtml) {
      html += '<div class="agent-card-tools">' + toolsHtml + '</div>';
    }
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
  var currentTools = [];
  try { if (agent && agent.tools) currentTools = JSON.parse(agent.tools); } catch(e) {}

  var body =
    '<div class="modal-field"><label>' + t('agents.name') + '</label>' +
    '<input type="text" id="agent-name-input" class="modal-input" value="' + escapeHtml(agent ? agent.name : '') + '"></div>' +
    '<div class="modal-field"><label>' + t('agents.desc') + '</label>' +
    '<input type="text" id="agent-desc-input" class="modal-input" value="' + escapeHtml(agent ? agent.desc : '') + '"></div>' +
    '<div class="modal-field"><label>' + t('agents.system_prompt') + '</label>' +
    '<textarea id="agent-system-prompt-input" class="modal-textarea">' + escapeHtml(agent ? agent.system_prompt : '') + '</textarea></div>' +
    '<div id="agent-tools-section">' +
    '<div class="modal-field"><label>' + t('agents.tools') + '</label>' +
    '<div id="agent-tools-chips" class="tools-chips"></div>' +
    '<input type="text" id="agent-tools-search" class="modal-input" placeholder="' + t('agents.tools_search') + '" style="margin-bottom:6px">' +
    '<div id="agent-tools-picker" class="tools-picker"><span class="modal-loading">Loading...</span></div></div>' +
    '</div>';


  openModal({
    title: title,
    body: body,
    wide: true,
    confirmText: t('agents.save'),
    onOpen: function() {
      // Shared selected sets for the modal (exposed on window for inline onclick access)
      window._modalSelectedTools = currentTools.slice();
      var allTools = [];

      window._renderToolChips = function() {
        var container = document.getElementById('agent-tools-chips');
        if (!container) return;
        var sel = window._modalSelectedTools;
        if (sel.length === 0) {
          container.innerHTML = '<span class="modal-hint">' + t('agents.no_tools') + '</span>';
          return;
        }
        var html = '';
        for (var i = 0; i < sel.length; i++) {
          html += '<span class="tool-chip">' + escapeHtml(sel[i]) + '<button class="tool-chip-remove" onclick="window._removeTool(\'' + escapeHtml(sel[i]) + '\')">×</button></span>';
        }
        container.innerHTML = html;
      };

      window._addTool = function(name) {
        if (window._modalSelectedTools.indexOf(name) < 0) {
          window._modalSelectedTools.push(name);
          window._renderToolChips();
          window._refreshToolPicker();
        }
      };

      window._removeTool = function(name) {
        var idx = window._modalSelectedTools.indexOf(name);
        if (idx >= 0) window._modalSelectedTools.splice(idx, 1);
        window._renderToolChips();
        window._refreshToolPicker();
      };

      window._refreshToolPicker = function() {
        var container = document.getElementById('agent-tools-picker');
        var query = (document.getElementById('agent-tools-search').value || '').toLowerCase();
        if (!container) return;
        var html = '';
        for (var i = 0; i < allTools.length; i++) {
          var t = allTools[i];
          if (query && t.name.toLowerCase().indexOf(query) < 0 && (t.description || '').toLowerCase().indexOf(query) < 0) continue;
          var sel = window._modalSelectedTools.indexOf(t.name) >= 0;
          html += '<div class="tool-pick-item' + (sel ? ' selected' : '') + '" onclick="' + (sel ? 'window._removeTool' : 'window._addTool') + '(\'' + escapeHtml(t.name) + '\')"><span class="tool-pick-name">' + escapeHtml(t.name) + (sel ? ' ✓' : '') + '</span><span class="tool-pick-desc">' + escapeHtml(t.description || '') + '</span></div>';
        }
        container.innerHTML = html || '<span class="modal-hint">No matching tools</span>';
      };

      fetch(SERVER_URL + '/api/tools')
        .then(function(r) { return r.json(); })
        .then(function(tools) {
          allTools = tools || [];
          window._renderToolChips();
          window._refreshToolPicker();
          var ts = document.getElementById('agent-tools-search');
          if (ts) ts.addEventListener('input', window._refreshToolPicker);
        }).catch(function() { var tp = document.getElementById('agent-tools-picker'); if (tp) tp.innerHTML = '<span class="modal-hint">Failed to load tools</span>'; });

      var ta = document.getElementById('agent-system-prompt-input');
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
      var systemPrompt = document.getElementById('agent-system-prompt-input').value;
      if (!name || !systemPrompt) {
        toast(name ? t('agents.system_prompt') + ' required' : t('agents.name') + ' required');
        return false;
      }
      var tools = window._modalSelectedTools || [];
      var payload = {name: name, desc: desc, system_prompt: systemPrompt, tools: JSON.stringify(tools)};
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

// Legacy stub for backward compat — no longer needed

// ── Bookmarks ──
  if (!activeThreadId) {
    document.getElementById('bookmarks-body').innerHTML = '<div class="bookmarks-empty">' + t('bookmark.home_hint') + '</div>';
    return;
  }
  fetch(SERVER_URL + '/api/messages/bookmarked?thread_id=' + encodeURIComponent(activeThreadId))
    .then(function(r) { return r.json(); })
    .then(function(msgs) {
      var body = document.getElementById('bookmarks-body');
      if (!Array.isArray(msgs) || msgs.length === 0) {
        body.innerHTML = '<div class="bookmarks-empty">' + t('bookmark.empty') + '</div>';
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
          '<button class="bkm-unstar" onclick="event.stopPropagation();toggleMessageBookmark(' + m.id + ', false)" title="' + t('bookmark.unstar_tip') + '">✕</button>' +
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
