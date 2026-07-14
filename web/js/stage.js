// stage.js — Timeline + Accordion rendering

// ── Status target ──
var _statusTarget = '';

// ── Visibility ──

function hideIdle() {
  document.getElementById('idle-state').style.display = 'none';
  document.getElementById('content-area').classList.remove('hidden');
}

function showIdle() {
  document.getElementById('content-area').classList.add('hidden');
  document.getElementById('idle-state').style.display = '';
  document.getElementById('timeline').innerHTML = '';
}

// ── Stage management ──

function setLiveStage(item) {
  if (!item.id) item.id = newItemId();
  if (currentStage && currentStage.item && currentStage.item.id === item.id) {
    updateExpandContent(item);
    return;
  }
  currentStage = { live: true, item: item };
  renderAll();
}

function setHistoryStage(item) {
  currentStage = { live: false, item: item };
}

// ── Render All ──

var _renderedTurnCount = 0;

function renderAll(forceFull) {
  var tl = document.getElementById('timeline');
  var wasAtBottom = tl.scrollTop + tl.clientHeight >= tl.scrollHeight - 50;
  renderTimeline(forceFull);
  if (forceFull || ((!currentStage || currentStage.live) && wasAtBottom)) {
    tl.scrollTop = tl.scrollHeight;
  }
}

// Prepend older messages (pagination), preserve scroll position
function prependMessages(msgs) {
  var tl = document.getElementById('timeline');
  var oldHeight = tl.scrollHeight;
  var oldScrollTop = tl.scrollTop;
  appendMessagesInternal(msgs, true);
  renderAll(true);
  var newHeight = tl.scrollHeight;
  tl.scrollTop = oldScrollTop + (newHeight - oldHeight);
}

function showTopLoader(show) {
  var loader = document.getElementById('top-loader');
  if (loader) loader.style.display = show ? 'block' : 'none';
}

// ── Timeline Rendering ──

function renderTimeline(forceFull) {
  var tl = document.getElementById('timeline');
  var turns = buildTurns();

  if (forceFull || _renderedTurnCount === 0 || turns.length < _renderedTurnCount) {
    var h = '<div class="timeline-inner">';
    for (var i = 0; i < turns.length; i++) {
      h += renderTurn(turns[i], i);
    }
    h += '</div>';
    tl.innerHTML = h;
    _renderedTurnCount = turns.length;
    return;
  }

  var inner = tl.querySelector('.timeline-inner');
  if (!inner) return;

  var domCount = inner.children.length;

  // Last turn changed? Re-render it in-place
  if (turns.length === domCount && turns.length > 0) {
    var lastEl = inner.children[domCount - 1];
    if (lastEl) {
      var openIds = [];
      var openCards = lastEl.querySelectorAll('.tool-card.open');
      for (var oi = 0; oi < openCards.length; oi++) {
        var tcid = openCards[oi].getAttribute('data-tool-call-id');
        if (tcid) openIds.push(tcid);
      }
      lastEl.outerHTML = renderTurn(turns[turns.length - 1], turns.length - 1);
      var newLastEl = inner.children[domCount - 1];
      for (var oj = 0; oj < openIds.length; oj++) {
        var card = newLastEl.querySelector('.tool-card[data-tool-call-id="' + openIds[oj] + '"]');
        if (card) {
          card.classList.add('open');
          var body = card.querySelector('.tool-card-body');
          var item = findTimelineItemByToolCallId(openIds[oj]);
          if (body && item && body.innerHTML === '') {
            body.innerHTML = renderToolBody(item);
          }
        }
      }
    }
    _renderedTurnCount = turns.length;
    return;
  }

  // New turns: append only
  for (var i = domCount; i < turns.length; i++) {
    inner.insertAdjacentHTML('beforeend', renderTurn(turns[i], i));
  }
  _renderedTurnCount = turns.length;
}

// ── Targeted DOM update for streaming items ──

function updateStreamingContent(item) {
  var tl = document.getElementById('timeline');
  var wasAtBottom = tl.scrollTop + tl.clientHeight >= tl.scrollHeight - 50;

  if (item.type === 'reply' && item.status === 'running') {
    var aiBubbles = tl.querySelectorAll('.msg-ai');
    if (aiBubbles.length > 0) {
      var lastAI = aiBubbles[aiBubbles.length - 1];
      var mdBodies = lastAI.querySelectorAll('.md-body');
      if (mdBodies.length > 0) {
        var lastMd = mdBodies[mdBodies.length - 1];
        try { lastMd.innerHTML = marked.parse(item.content || ''); } catch(e) {}
        if (wasAtBottom) tl.scrollTop = tl.scrollHeight;
        return true;
      }
    }
  }

  if (item.type === 'tool' && item.status === 'running' && item.tool_call_id) {
    var cards = tl.querySelectorAll('.tool-card');
    for (var ci = 0; ci < cards.length; ci++) {
      if (cards[ci].getAttribute('data-tool-call-id') === item.tool_call_id) {
        var resultEl = cards[ci].querySelector('.tool-result');
        if (resultEl) {
          var fullContent = item.content || '';
          var renderedLen = parseInt(resultEl.getAttribute('data-rendered-len') || '0');
          if (renderedLen >= fullContent.length) {
            if (wasAtBottom) tl.scrollTop = tl.scrollHeight;
            return true;
          }

          var newPart = fullContent.substring(renderedLen);
          var lastNewline = newPart.lastIndexOf('\n');
          var toRender, newRendered;
          if (lastNewline >= 0) {
            toRender = newPart.substring(0, lastNewline + 1);
            newRendered = renderedLen + toRender.length;
          } else {
            if (wasAtBottom) tl.scrollTop = tl.scrollHeight;
            return true;
          }

          var lines = toRender.split('\n');
          var html = '';
          for (var li = 0; li < lines.length; li++) {
            var line = lines[li];
            if (line === '') { html += '\n'; continue; }
            if (/^\$ /.test(line)) html += '<span style="color:var(--green)">' + escapeHtml(line) + '</span>\n';
            else if (/^Error:/.test(line)) html += '<span style="color:#c48a82">' + escapeHtml(line) + '</span>\n';
            else html += escapeHtml(line) + '\n';
          }

          var cursorEl = resultEl.querySelector('.stream-cursor');
          if (cursorEl) cursorEl.remove();
          resultEl.insertAdjacentHTML('beforeend', html);
          resultEl.insertAdjacentHTML('beforeend', '<span class="stream-cursor"></span>');
          resultEl.setAttribute('data-rendered-len', newRendered);

          if (wasAtBottom) tl.scrollTop = tl.scrollHeight;
          return true;
        }
      }
    }
  }

  // Tool finished — flush final partial line, remove cursor
  if (item.type === 'tool' && item.status !== 'running' && item.tool_call_id) {
    var cards2 = tl.querySelectorAll('.tool-card');
    for (var cj = 0; cj < cards2.length; cj++) {
      if (cards2[cj].getAttribute('data-tool-call-id') === item.tool_call_id) {
        var resEl = cards2[cj].querySelector('.tool-result');
        if (resEl) {
          var full = item.content || '';
          var rendered = parseInt(resEl.getAttribute('data-rendered-len') || '0');
          if (rendered < full.length) {
            var tail = full.substring(rendered);
            if (/^\$ /.test(tail)) resEl.insertAdjacentHTML('beforeend', '<span style="color:var(--green)">' + escapeHtml(tail) + '</span>');
            else if (/^Error:/.test(tail)) resEl.insertAdjacentHTML('beforeend', '<span style="color:#c48a82">' + escapeHtml(tail) + '</span>');
            else resEl.insertAdjacentHTML('beforeend', escapeHtml(tail));
            resEl.setAttribute('data-rendered-len', full.length);
          }
          var cur = resEl.querySelector('.stream-cursor');
          if (cur) cur.remove();
        }
        var dot = cards2[cj].querySelector('.tool-dot');
        if (dot) { dot.className = 'tool-dot ' + (item.error ? 'error' : 'done'); }
        return true;
      }
    }
  }

  return false;
}

// ── Turn building ──

function buildTurns() {
  var turns = [];
  var curAI = null;

  function flushAI() {
    if (curAI && curAI.items.length > 0) {
      turns.push(curAI);
      curAI = null;
    }
  }

  for (var i = 0; i < timelineItems.length; i++) {
    var item = timelineItems[i];

    if (item.type === 'user') {
      flushAI();
      turns.push({ type: 'user', item: item });
    } else if (item.type === 'error' || item.type === 'notice') {
      flushAI();
      turns.push({ type: 'notice', item: item });
    } else {
      // reply, tool → group into AI turn
      if (!curAI) curAI = { type: 'ai', items: [] };
      curAI.items.push(item);
    }
  }
  flushAI();
  return turns;
}

function renderTurn(turn, idx) {
  var tid = 'turn-' + idx;
  if (turn.type === 'user') {
    return '<div class="msg msg-user" id="' + tid + '">' +
      '<div class="msg-bubble">' + escapeHtml(turn.item.content || '') + '</div>' +
    '</div>';
  }
  if (turn.type === 'ai') {
    var body = '';
    for (var i = 0; i < turn.items.length; i++) {
      var item = turn.items[i];
      if (item.type === 'reply') {
        var html = item.content || '';
        if (item.status === 'running') {
          try { html = marked.parse(item.content || ''); } catch(e) {}
        }
        body += '<div class="md-body">' + html + '</div>';
      } else if (item.type === 'tool') {
        body += renderToolCard(item);
      }
    }
    return '<div class="msg msg-ai" id="' + tid + '">' +
      '<div class="msg-bubble">' + body + '</div>' +
    '</div>';
  }
  if (turn.type === 'notice') {
    return '<div class="msg msg-notice" id="' + tid + '">' +
      '<div class="msg-bubble">' + escapeHtml(turn.item.content || '') + '</div>' +
    '</div>';
  }
  return '';
}

function renderToolCard(item) {
  var dotClass = item.status === 'running' ? 'running' : (item.error ? 'error' : 'done');
  var tcidAttr = item.tool_call_id ? ' data-tool-call-id="' + item.tool_call_id + '"' : '';

  // Lazy render: running items need body for streaming; done items render on expand
  var bodyHtml = '';
  if (item.status === 'running') {
    bodyHtml = renderToolBody(item);
  }
  var onclickAttr = item.tool_call_id
    ? 'onclick="toggleToolCard(\'' + escapeHtml(item.tool_call_id) + '\')"'
    : 'onclick="this.parentElement.classList.toggle(\'open\')"';
  return '<div class="tool-card"' + tcidAttr + '>' +
    '<div class="tool-card-header" ' + onclickAttr + '>' +
      '<span class="tool-dot ' + dotClass + '"></span>' +
      '<span class="tool-name">' + escapeHtml(item.label || '') + '</span>' +
      '<span class="tool-chevron">▾</span>' +
    '</div>' +
    '<div class="tool-card-body">' + bodyHtml + '</div>' +
  '</div>';
}

function renderToolBody(item) {
  var content = item.content || '';
  var lastNewline = content.lastIndexOf('\n');
  var renderedContent = lastNewline >= 0 ? content.substring(0, lastNewline + 1) : '';
  var renderedLen = renderedContent.length;

  var contentHtml = '';
  var lines = renderedContent.split('\n');
  if (lines.length > 500) { lines = lines.slice(0, 500); contentHtml += '[Output truncated — showing first 500 lines]\n'; }
  for (var li = 0; li < lines.length; li++) {
    var line = lines[li];
    if (line === '') { contentHtml += '\n'; continue; }
    if (/^\$ /.test(line)) contentHtml += '<span style="color:var(--green)">' + escapeHtml(line) + '</span>\n';
    else if (/^Error:/.test(line)) contentHtml += '<span style="color:#c48a82">' + escapeHtml(line) + '</span>\n';
    else contentHtml += escapeHtml(line) + '\n';
  }
  if (item.status === 'running') contentHtml += '<span class="stream-cursor"></span>';
  if (item.status !== 'running' && renderedLen < content.length) {
    var tail = content.substring(renderedLen);
    if (/^\$ /.test(tail)) contentHtml += '<span style="color:var(--green)">' + escapeHtml(tail) + '</span>';
    else if (/^Error:/.test(tail)) contentHtml += '<span style="color:#c48a82">' + escapeHtml(tail) + '</span>';
    else contentHtml += escapeHtml(tail);
    renderedLen = content.length;
  }

  return (item.detail ? '<div class="tool-params"><strong>Details</strong><code>' + escapeHtml(item.detail) + '</code></div>' : '') +
    '<div class="tool-result" data-rendered-len="' + renderedLen + '">' + contentHtml + '</div>';
}

function toggleToolCard(toolCallId) {
  var card = document.querySelector('.tool-card[data-tool-call-id="' + CSS.escape(toolCallId) + '"]');
  if (!card) return;
  var body = card.querySelector('.tool-card-body');
  if (card.classList.contains('open')) {
    body.innerHTML = '';
    card.classList.remove('open');
  } else {
    var item = findTimelineItemByToolCallId(toolCallId);
    if (item) body.innerHTML = renderToolBody(item);
    card.classList.add('open');
  }
}

function findTimelineItemByToolCallId(tcid) {
  for (var i = 0; i < timelineItems.length; i++) {
    if (timelineItems[i].tool_call_id === tcid) return timelineItems[i];
  }
  return null;
}

function updateExpandContent(item) {
  if (!updateStreamingContent(item)) {
    renderAll();
  }
}

// ── Status bar ──

function updateStatus(text, type) {
  var bar = document.getElementById('status-bar');
  var label = bar.querySelector('.status-text');
  if (!bar || !label) return;
  label.textContent = text || '';
  bar.className = 'status-bar';
  if (type === 'thinking' || type === 'active') bar.classList.add('active');
  else if (type === 'error') bar.classList.add('error');
}

// ── No model prompt ──

var _idleHTML = '';

function showNoModelPrompt() {
  var idle = document.getElementById('idle-state');
  if (!idle) return;
  if (!_idleHTML) _idleHTML = idle.innerHTML;
  idle.innerHTML =
    '<div class="idle-glow no-model-glow">⚡</div>' +
    '<div class="idle-text" style="color:#fda4af">No model configured</div>' +
    '<div style="font-size:11px;color:var(--text-dim);margin-top:-8px">Add a model in Settings to start chatting.</div>' +
    '<button class="hint-chip" onclick="toggleSettings()" style="border-color:rgba(167,139,250,0.35);color:#c4b5fd;padding:8px 16px">Open Settings</button>';
}

function resetIdlePrompt() {
  hasModels = true;
  var idle = document.getElementById('idle-state');
  if (idle && _idleHTML) { idle.innerHTML = _idleHTML; }
}

// ── Image preview ──

function updateImagePreview() {
  var bar = document.getElementById('img-preview');
  if (!pendingImages.length) { bar.classList.remove('show'); bar.innerHTML = ''; return; }
  bar.classList.add('show');
  var html = '';
  pendingImages.forEach(function(img, i) {
    html += '<div class="img-thumb-wrap"><img class="img-thumb" src="' + img + '"><button class="img-thumb-close" onclick="removeImage(' + i + ')">✕</button></div>';
  });
  bar.innerHTML = html;
}

function removeImage(idx) {
  pendingImages.splice(idx, 1);
  updateImagePreview();
}

// ── Context bar ──

function updateContextBar(usedPct) {
  var fill = document.getElementById('ctx-progress-fill');
  var label = document.getElementById('ctx-label');
  if (!fill || !label) return;
  var pct = Math.max(0, Math.min(100, usedPct));
  fill.style.width = pct + '%';
  fill.classList.remove('warn', 'danger');
  if (pct >= 85) fill.classList.add('danger');
  else if (pct >= 60) fill.classList.add('warn');
  label.textContent = Math.round(pct) + '%';
}

function updateContextModel(modelName) {
  var nameEl = document.getElementById('ctx-model-name');
  var dotEl = document.getElementById('ctx-model-dot');
  if (!nameEl || !dotEl) return;
  if (!modelName) {
    nameEl.textContent = '—';
    dotEl.classList.add('no-model');
  } else {
    nameEl.textContent = modelName;
    dotEl.classList.remove('no-model');
  }
}

function updateContextTokens(total) {
  var el = document.getElementById('ctx-tokens');
  if (!el) return;
  el.textContent = '⚡ ' + formatTokens(total);
  el.title = 'Total tokens: ' + total;
}

function formatTokens(n) {
  if (!n || n < 0) n = 0;
  if (n >= 1000000) return (n / 1000000).toFixed(1) + 'M';
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k';
  return String(n);
}

// ── Utils ──

function escapeHtml(str) {
  if (!str) return '';
  return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function fillInput(text) {
  var inp = document.getElementById('msg-input');
  if (inp) { inp.value = text; inp.focus(); }
}

// ── View switching ──

function switchThread(threadId) {
  activeThreadId = threadId;
  _noMoreMessages = false;
  updateThreadList();
  updateContextBar(_contextPcts[threadId] || 0);
  loadThreadMessages(threadId);
  syncContentCards();
}

// ── Tool meta ──

function formatArgsLabel(args, maxLen) {
  if (maxLen === undefined) maxLen = 60;
  var parts = [];
  for (var k in args) {
    if (!args.hasOwnProperty(k)) continue;
    var v = args[k];
    if (v === null || v === undefined || v === '' || v === false) continue;
    if (maxLen > 0 && typeof v === 'string' && v.length > maxLen) v = v.substring(0, maxLen) + '…';
    else if (typeof v !== 'string') v = JSON.stringify(v);
    parts.push(k + ': ' + v);
  }
  return parts.join(', ');
}

function getToolMeta(tn, args) {
  if (tn === 'write_file' || tn === 'edit_file') {
    var fname = ((args.path || '').split('/').pop().split('\\').pop() || args.path);
    return { icon: '✎', label: tn + ' — ' + fname, detail: tn + '\npath: ' + (args.path || '') };
  }
  if (tn === 'spawn_worker') {
    return { icon: '⚙', label: 'spawn_worker — ' + (args.name || ''), detail: 'spawn_worker\nname: ' + (args.name || '') + '\ntask: ' + ((args.task || '').substring(0, 200) || '') };
  }
  if (tn === 'run_http_request') {
    return { icon: '🌐', label: (args.method || 'GET') + ' ' + (args.url || ''), detail: (args.method || 'GET') + ' ' + (args.url || '') };
  }

  var icon = '⚙';
  if (tn === 'web_search' || tn === 'web_fetch') icon = '🌐';
  else if (tn === 'read_file' || tn === 'read_dir') icon = '✎';
  else if (tn === 'run_command') icon = '⬛';
  else if (tn === 'search_content' || tn === 'search_files') icon = '⬛';

  var argsLabel = formatArgsLabel(args);
  return { icon: icon, label: tn + (argsLabel ? ' — ' + argsLabel : ''), detail: tn + (argsLabel ? '\n' + argsLabel : '') };
}

// ── Content cards ──

var _contentCards = {};
var _cardDrag = null;

document.addEventListener('mousemove', function(e) {
  if (!_cardDrag) return;
  _cardDrag.card.style.right = (_cardDrag.startRight - (e.clientX - _cardDrag.startX)) + 'px';
  _cardDrag.card.style.bottom = (_cardDrag.startBottom - (e.clientY - _cardDrag.startY)) + 'px';
});
document.addEventListener('mouseup', function() {
  if (!_cardDrag) return;
  _cardDrag.card.style.transition = 'opacity 0.25s ease, box-shadow 0.25s ease, border-color 0.25s ease';
  _cardDrag = null;
});

var _cardActive = null;

function getCurrentThreadId() {
  return activeThreadId || '';
}

function syncContentCards() {
  var stage = document.getElementById('stage');
  var currentTid = getCurrentThreadId();
  var visible = false;
  var firstVisible = null;
  for (var id in _contentCards) {
    var card = _contentCards[id];
    var belongs = !card.sessionId || card.sessionId === currentTid;
    card.el.style.display = belongs ? 'flex' : 'none';
    if (belongs) {
      if (!firstVisible) firstVisible = id;
      visible = true;
    }
  }
  if (visible) {
    var active = _contentCards[_cardActive];
    if (!active || active.el.style.display === 'none') _cardActive = firstVisible;
    for (var id in _contentCards) {
      _contentCards[id].el.style.display = id === _cardActive ? 'flex' : 'none';
    }
    stage.classList.add('split');
  } else {
    _cardActive = null;
    stage.classList.remove('split');
  }
}

function createContentCard(data) {
  if (!data.id) return;
  var cardHtml = data.html || '<div style="padding:20px;color:var(--text-dim);text-align:center;">Empty content</div>';
  if (_contentCards[data.id]) removeContentCard(data.id);

  var container = document.getElementById('content-cards');
  var card = document.createElement('div');
  card.className = 'content-card';
  card.id = 'card-' + data.id;
  if (data.height) card.style.height = data.height + 'px';

  var htmlSource = data.html_source || '';
  card.innerHTML =
    '<div class="content-card-body">' +
      '<iframe sandbox="allow-scripts allow-same-origin allow-downloads" srcdoc="' + escapeAttr(cardHtml) + '"></iframe>' +
    '</div>';

  container.appendChild(card);

  _contentCards[data.id] = {
    el: card,
    title: data.title || 'Content',
    html: data.html,
    htmlSource: htmlSource,
    isHtmlFile: data.is_html_file || false,
    showingSource: false,
    filePath: data.file_path || '',
    sessionId: data.session_id || ''
  };

  _cardActive = data.id;
  syncContentCards();
}

function removeContentCard(id) {
  var entry = _contentCards[id];
  if (entry && entry.el) entry.el.remove();
  delete _contentCards[id];
  if (_cardActive === id) _cardActive = null;
  syncContentCards();
}

function escapeAttr(s) {
  return s.replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}

// ── Toggle sidebar ──

function toggleSidebar() {
  var closed = document.body.classList.toggle('sb-closed');
  if (window.innerWidth <= 860) {
    if (closed) {
      document.getElementById('backdrop').classList.remove('open');
    } else {
      document.getElementById('backdrop').classList.add('open');
    }
  }
}

if (window.innerWidth <= 860) {
  document.body.classList.add('sb-closed');
}

document.getElementById('hamburger-btn').addEventListener('click', toggleSidebar);
var closeBtn = document.querySelector('.sidebar-close-btn');
if (closeBtn) closeBtn.addEventListener('click', toggleSidebar);
