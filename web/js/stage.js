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
  hideAgentBar();
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
  // Only auto-scroll if user was already at the bottom
  if ((!currentStage || currentStage.live) && wasAtBottom) {
    tl.scrollTop = tl.scrollHeight;
  }
}

// ── Timeline Rendering ──

function renderTimeline(forceFull) {
  var tl = document.getElementById('timeline');
  var turns = buildTurns();

  // Full render: first time, force flag, or turn count decreased (reset)
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

  // Last turn changed (items added/modified)? Re-render it in-place
  if (turns.length === domCount && turns.length > 0) {
    var lastEl = inner.children[domCount - 1];
    if (lastEl) {
      // Save user-expanded tool-cards so we can restore them after re-render
      var openIds = [];
      var openCards = lastEl.querySelectorAll('.tool-card.open');
      for (var oi = 0; oi < openCards.length; oi++) {
        var tcid = openCards[oi].getAttribute('data-tool-call-id');
        if (tcid) openIds.push(tcid);
      }
      lastEl.outerHTML = renderTurn(turns[turns.length - 1], turns.length - 1);
      // Restore open state on matching tool-cards
      var newLastEl = inner.children[domCount - 1];
      for (var oj = 0; oj < openIds.length; oj++) {
        var card = newLastEl.querySelector('.tool-card[data-tool-call-id="' + openIds[oj] + '"]');
        if (card) card.classList.add('open');
      }
    }
    _renderedTurnCount = turns.length;
    return;
  }

  // New turns: append only (use DOM count as ground truth)
  for (var i = domCount; i < turns.length; i++) {
    inner.insertAdjacentHTML('beforeend', renderTurn(turns[i], i));
  }
  _renderedTurnCount = turns.length;
}

// ── Targeted DOM update for streaming items (avoids full re-render flicker) ──

function updateStreamingContent(item) {
  var tl = document.getElementById('timeline');
  var wasAtBottom = tl.scrollTop + tl.clientHeight >= tl.scrollHeight - 50;

  if (item.type === 'reply' && item.status === 'running') {
    // Update the last .md-body in the last AI bubble in-place
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
    // Update tool card result in-place — only append new content, never rebuild
    var cards = tl.querySelectorAll('.tool-card');
    for (var ci = 0; ci < cards.length; ci++) {
      if (cards[ci].getAttribute('data-tool-call-id') === item.tool_call_id) {
        var resultEl = cards[ci].querySelector('.tool-result');
        if (resultEl) {
          var fullContent = item.content || '';
          var renderedLen = parseInt(resultEl.getAttribute('data-rendered-len') || '0');
          if (renderedLen >= fullContent.length) {
            if (wasAtBottom) tl.scrollTop = tl.scrollHeight;
            return true; // nothing new
          }

          // Find last complete lines in the new content
          var newPart = fullContent.substring(renderedLen);
          var lastNewline = newPart.lastIndexOf('\n');
          var toRender, newRendered;
          if (lastNewline >= 0) {
            toRender = newPart.substring(0, lastNewline + 1);
            newRendered = renderedLen + toRender.length;
          } else {
            // No complete line yet — wait for next chunk
            if (wasAtBottom) tl.scrollTop = tl.scrollHeight;
            return true;
          }

          // Format and append only the new lines
          var lines = toRender.split('\n');
          var html = '';
          for (var li = 0; li < lines.length; li++) {
            var line = lines[li];
            if (line === '') { html += '\n'; continue; }
            if (/^\$ /.test(line)) html += '<span style="color:var(--green)">' + escapeHtml(line) + '</span>\n';
            else if (/^Error:/.test(line)) html += '<span style="color:#c48a82">' + escapeHtml(line) + '</span>\n';
            else html += escapeHtml(line) + '\n';
          }

          // Remove old cursor, append new lines, re-add cursor
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

  // Tool finished — flush final partial line, remove cursor, update status
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
          // Remove cursor
          var cur = resEl.querySelector('.stream-cursor');
          if (cur) cur.remove();
        }
        // Update card status styling (keep user's open/close preference)
        var dot = cards2[cj].querySelector('.tool-dot');
        if (dot) { dot.className = 'tool-dot ' + (item.error ? 'error' : 'done'); }
        // Remove the data-rendered-len tracking (no longer needed)
        // Keep scroll position
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
    } else if (item.type === 'thinking') {
      flushAI();
      turns.push({ type: 'thinking', item: item });
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

function starBtn(dbId, bookmarked) {
  if (!dbId) return '';
  var cls = bookmarked ? ' msg-star bookmarked' : ' msg-star';
  var sym = bookmarked ? '★' : '☆';
  return '<button class="' + cls + '" data-msg-id="' + dbId + '" onclick="event.stopPropagation();var b=!this.classList.contains(\'bookmarked\');toggleMessageBookmark(' + dbId + ',b)" title="' + (bookmarked ? '取消收藏' : '收藏') + '">' + sym + '</button>';
}

function renderTurn(turn, idx) {
  var tid = 'turn-' + idx;
  if (turn.type === 'user') {
    return '<div class="msg msg-user" id="' + tid + '">' +
      '<div class="msg-bubble">' + starBtn(turn.item.db_id, turn.item.bookmarked) + escapeHtml(turn.item.content || '') + '</div>' +
    '</div>';
  }
  if (turn.type === 'ai') {
    var dbId = null, bm = false;
    for (var i = 0; i < turn.items.length; i++) {
      if (turn.items[i].type === 'reply' && turn.items[i].db_id) { dbId = turn.items[i].db_id; bm = turn.items[i].bookmarked; break; }
    }
    var body = '';
    for (var i = 0; i < turn.items.length; i++) {
      var item = turn.items[i];
      if (item.type === 'reply') {
        // For streaming status, use raw markdown rendered; for done, use content as-is (already HTML from marked)
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
      '<div class="msg-bubble">' + starBtn(dbId, bm) + body + '</div>' +
    '</div>';
  }
  if (turn.type === 'notice') {
    return '<div class="msg msg-notice" id="' + tid + '">' +
      '<div class="msg-bubble">' + starBtn(turn.item.db_id, turn.item.bookmarked) + escapeHtml(turn.item.content || '') + '</div>' +
    '</div>';
  }
  if (turn.type === 'thinking') {
    // Active thinking shows the pulse bubble; old ones are invisible separators
    if (turn.item.status !== 'running') return '';
    return '<div class="msg msg-ai msg-thinking" id="' + tid + '">' +
      '<div class="msg-bubble">' + t('status.thinking') + '</div>' +
    '</div>';
  }
  return '';
}

function renderToolCard(item) {
  var isOpen = '';
  var dotClass = item.status === 'running' ? 'running' : (item.error ? 'error' : 'done');
  var content = item.content || '';

  // Render complete lines only; keep partial line buffered for streaming
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
  // For done/fail, flush any remaining partial line
  if (item.status !== 'running' && renderedLen < content.length) {
    var tail = content.substring(renderedLen);
    if (/^\$ /.test(tail)) contentHtml += '<span style="color:var(--green)">' + escapeHtml(tail) + '</span>';
    else if (/^Error:/.test(tail)) contentHtml += '<span style="color:#c48a82">' + escapeHtml(tail) + '</span>';
    else contentHtml += escapeHtml(tail);
    renderedLen = content.length;
  }

  var tcidAttr = item.tool_call_id ? ' data-tool-call-id="' + item.tool_call_id + '"' : '';
  return '<div class="tool-card' + isOpen + '"' + tcidAttr + '>' +
    '<div class="tool-card-header" onclick="this.parentElement.classList.toggle(\'open\')">' +
      '<span class="tool-dot ' + dotClass + '"></span>' +
      '<span class="tool-name">' + escapeHtml(item.label || '') + '</span>' +
      '<span class="tool-chevron">▾</span>' +
    '</div>' +
    '<div class="tool-card-body">' +
      (item.detail ? '<div class="tool-params"><strong>' + t('model.params_label') + '</strong><code>' + escapeHtml(item.detail) + '</code></div>' : '') +
      '<div class="tool-result" data-rendered-len="' + renderedLen + '">' + contentHtml + '</div>' +
    '</div>' +
  '</div>';
}

// ── Legacy (no-op in chat layout) ──

function expandTimelineItem(id) {}

function updateExpandContent(item) {
  // Try targeted DOM update first (no flicker); fall back to full render
  if (!updateStreamingContent(item)) {
    renderAll();
  }
}

// ── Expand Body Formatting ──

function formatExpandBody(item) {
  if (item.type === 'reply') {
    return item.content || '';
  }
  if (item.type === 'user') {
    return '<div class="exp-user-content">' + escapeHtml(item.content || '') + '</div>';
  }
  if (item.type === 'error') {
    return escapeHtml(item.content || '');
  }
  if (item.type === 'notice') {
    return '<div style="color:var(--text-dim);font-style:italic">' + escapeHtml(item.content || '') + '</div>';
  }
  if (item.type === 'tool') {
    var detailHtml = '';
    if (item.detail) {
      detailHtml = '<div class="exp-tool-args">' + escapeHtml(item.detail) + '</div>';
    }
    return detailHtml + formatToolContent(item.content || '', item.status === 'running');
  }
  return '';
}

function formatToolContent(text, isRunning) {
  var lines = text.split('\n');
  var truncated = false;
  if (lines.length > 500) {
    lines = lines.slice(0, 500);
    truncated = true;
  }
  var out = lines.map(function(line) {
    if (/^\$ /.test(line)) return '<span class="t-prompt">' + escapeHtml(line) + '</span>';
    if (/^Error:/.test(line) || /^.*error/i.test(line)) return '<span class="t-err">' + escapeHtml(line) + '</span>';
    return escapeHtml(line);
  }).join('\n');
  if (truncated) out += '\n\n[Output truncated — showing first 500 lines]';
  if (isRunning && out.length > 0) out += '<span class="t-cursor">▊</span>';
  return out;
}

// ── Agent Bar ──

function renderAgentBar() {
  if (currentView === 'home') { hideAgentBar(); return; }
  var session = null;
  for (var i = 0; i < sessions.length; i++) {
    if (sessions[i].ref_id === currentView || sessions[i].id === currentView) { session = sessions[i]; break; }
  }
  var bar = document.getElementById('agent-bar');
  if (!session || !session.agents || session.agents.length === 0) {
    bar.classList.remove('show');
    bar.innerHTML = '';
    return;
  }
  bar.classList.add('show');
  var html = '';

  // Back button when drilling into a worker
  if (_agentFilter) {
    var filterAgent = null;
    for (var fi = 0; fi < session.agents.length; fi++) {
      if (session.agents[fi].session_id === _agentFilter) { filterAgent = session.agents[fi]; break; }
    }
    html += '<div class="agent-item agent-filter" onclick="clearAgentFilter()">';
    html += '<span class="agent-dot filter"></span>';
    html += '<span class="agent-name">' + t('timeline.viewing') + escapeHtml(filterAgent ? filterAgent.name : 'Worker') + '</span>';
    html += '<span class="agent-activity">' + t('timeline.back_to_project') + '</span>';
    html += '</div>';
  }

  session.agents.forEach(function(a) {
    var running = a.status === 'running';
    var dotCls = running ? 'running' : 'idle';
    var selected = _agentFilter === a.session_id;
    var activity = '';
    if (running && _agentActivities[a.session_id]) {
      activity = _agentActivities[a.session_id].text;
    } else if (!running) {
      activity = t('status.idle_activity');
    }
    html += '<div class="agent-item' + (selected ? ' selected' : '') + '" onclick="setAgentFilter(\'' + a.session_id + '\')">';
    html += '<span class="agent-dot ' + dotCls + '"></span>';
    html += '<span class="agent-name">' + escapeHtml(a.name || 'Agent') + '</span>';
    if (activity) html += '<span class="agent-activity">' + escapeHtml(activity) + '</span>';
    html += '</div>';
  });
  bar.innerHTML = html;
}

function setAgentFilter(sid) {
  if (_agentFilter === sid) {
    clearAgentFilter();
    return;
  }
  _agentFilter = sid;
  timelineItems = [];
  _itemSeq = 0;
  currentStage = null;
  loadSessionMessages(sid);
  updateContextBar(_contextPcts[sid] || 0);
  renderAgentBar();
}

function clearAgentFilter() {
  _agentFilter = null;
  timelineItems = [];
  _itemSeq = 0;
  currentStage = null;
  loadSessionMessages(currentView);
  updateViewContextBar();
  renderAgentBar();
}

function updateViewContextBar() {
  var viewSid = 'main';
  if (currentView !== 'home') {
    for (var i = 0; i < sessions.length; i++) {
      if (sessions[i].ref_id === currentView || sessions[i].id === currentView) {
        viewSid = sessions[i].session_id || currentView;
        break;
      }
    }
  }
  updateContextBar(_contextPcts[viewSid] || 0);
}

function hideAgentBar() {
  var bar = document.getElementById('agent-bar');
  bar.classList.remove('show');
  bar.innerHTML = '';
}

// ── Status bar ──

function updateStatus(text, type, target) {
  var bar = document.getElementById('status-bar');
  var label = bar.querySelector('.status-text');
  if (!bar || !label) return;
  var display = text || '';
  var tar = target || _statusTarget || '';
  if (tar && text !== t('status.thinking')) display += ' → ' + tar;
  label.textContent = display;
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
    '<div class="idle-text" style="color:#fda4af">' + t('timeline.no_model_title') + '</div>' +
    '<div style="font-size:11px;color:var(--text-dim);margin-top:-8px">' + t('timeline.no_model_desc') + '</div>' +
    '<button class="hint-chip" onclick="toggleSettings()" style="border-color:rgba(167,139,250,0.35);color:#c4b5fd;padding:8px 16px">' + t('timeline.no_model_btn') + '</button>';
}

function resetIdlePrompt() {
  hasModels = true;
  var idle = document.getElementById('idle-state');
  if (idle && _idleHTML) { idle.innerHTML = _idleHTML; }
}

// ── TTS progress ──

function showTTSBar() { document.getElementById('tts-bar').style.display = 'flex'; }
function updateTTSBar(pct) { document.getElementById('tts-bar-fill').style.width = pct + '%'; }
function hideTTSBar() { document.getElementById('tts-bar').style.display = 'none'; }

function stopTTS() {
  if (typeof speechSynthesis !== 'undefined') speechSynthesis.cancel();
  hideTTSBar();
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
  var track = document.getElementById('ctx-track');
  var total = 10;
  var used = Math.round(usedPct / 100 * total);
  var html = '';
  for (var i = 0; i < total; i++) {
    html += '<div class="ctx-dot' + (i < used ? ' used' : '') + '"></div>';
  }
  track.innerHTML = html;
  document.getElementById('ctx-label').textContent = Math.round(usedPct) + '%';
}

// ── Utils ──

function escapeHtml(str) {
  if (!str) return '';
  return String(str).replace(/&/g,'&amp;').replace(/</g,'&lt;').replace(/>/g,'&gt;').replace(/"/g,'&quot;');
}

function stripHtml(html) {
  var d = document.createElement('div');
  d.innerHTML = html;
  return d.textContent || d.innerText || '';
}

function fillInput(text) {
  var inp = document.getElementById('msg-input');
  if (inp) { inp.value = text; inp.focus(); }
}

// ── View switching ──

function switchView(view) {
  if (view === 'main') view = 'home';
  if (voiceMode) stopVoiceAndDeactivate();
  currentView = view;
  _agentFilter = null;
  document.querySelectorAll('.tab-item').forEach(function(t) {
    t.classList.toggle('active', t.dataset.view === view);
  });

  updateViewContextBar();

  if (view === 'home') {
    _statusTarget = '';
    loadSessionMessages('main');
    updateStatus(t('status.ready'), 'idle');
    hideAgentBar();
  } else {
    showProjectDash(view);
    loadSessionMessages(view);
  }
  syncContentCards();
}

function showProjectDash(refId) {
  hideIdle();
  var contentArea = document.getElementById('content-area');
  contentArea.classList.remove('hidden');

  var session = null;
  for (var i = 0; i < sessions.length; i++) {
    if (sessions[i].ref_id === refId || sessions[i].id === refId) { session = sessions[i]; break; }
  }

  _statusTarget = session ? (session.title || refId) : refId;
  updateStatus(t('status.ready'), 'idle');
  renderAgentBar();

  if (currentStage) {
    renderAll();
  } else if (timelineItems.length === 0) {
    document.getElementById('timeline').innerHTML = '';
  } else {
    renderTimeline();
  }
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
  // Large-text tools — show only identifying params
  if (tn === 'write_file' || tn === 'edit_file') {
    var fname = ((args.path || '').split('/').pop().split('\\').pop() || args.path);
    return { icon: '✎', label: tn + ' — ' + fname, detail: tn + '\npath: ' + (args.path || '') };
  }
  if (tn === 'create_agent' || tn === 'edit_agent' || tn === 'delete_agent') {
    return { icon: '⚙', label: tn + ' — ' + (args.name || ''), detail: tn + '\nname: ' + (args.name || '') };
  }
  if (tn === 'show_html') {
    return { icon: '🌐', label: 'show_html — ' + (args.title || ''), detail: 'show_html\ntitle: ' + (args.title || '') + '\nwidth: ' + (args.width || 800) + ', height: ' + (args.height || 600) };
  }
  if (tn === 'create_project') {
    return { icon: '⚙', label: 'create_project — ' + (args.title || ''), detail: 'create_project\ntitle: ' + (args.title || '') + '\nprompt: ' + ((args.prompt || '').substring(0, 200) || '(empty)') };
  }
  if (tn === 'spawn_worker') {
    return { icon: '⚙', label: 'spawn_worker — ' + (args.name || ''), detail: 'spawn_worker\nname: ' + (args.name || '') + '\ntask: ' + ((args.task || '').substring(0, 200) || '') };
  }
  if (tn === 'run_http_request') {
    return { icon: '🌐', label: (args.method || 'GET') + ' ' + (args.url || ''), detail: (args.method || 'GET') + ' ' + (args.url || '') + '\nheaders: ' + JSON.stringify(args.headers || {}) + '\nbody: ' + ((args.body || '').substring(0, 200) || '(empty)') };
  }

  // Icons
  var icon = '⚙';
  if (tn === 'web_search' || tn === 'web_fetch' || tn === 'browser_content' || tn.startsWith('browser_')) icon = '🌐';
  else if (tn === 'show_file' && typeof args.path === 'string' && args.path.toLowerCase().endsWith('.scad')) icon = '📐';
  else if (tn === 'read_file' || tn === 'show_file' || tn === 'read_dir') icon = '✎';
  else if (tn === 'run_command' || tn === 'execute_command') icon = '⬛';
  else if (tn === 'search_content' || tn === 'search_files') icon = '⬛';
  else if (tn.startsWith('desktop_')) icon = '🖥';

  var argsLabel = formatArgsLabel(args);
  var argsDetail = formatArgsLabel(args, 0);
  return { icon: icon, label: tn + (argsLabel ? ' — ' + argsLabel : ''), detail: tn + (argsDetail ? '\n' + argsDetail : '') };
}

// ── Content cards (show_html, show_file) ──

var _contentCards = {};
var _cardDrag = null;

// Global drag listeners — set up once
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

var _cardActive = null; // id of currently visible card

function renderCardTabs() {
  var container = document.getElementById('content-cards');
  var bar = document.getElementById('card-tabs');
  if (!bar) {
    bar = document.createElement('div');
    bar.className = 'card-tabs';
    bar.id = 'card-tabs';
    container.insertBefore(bar, container.firstChild);
  }
  var ids = Object.keys(_contentCards);
  // Only show tabs for cards belonging to the current project
  var currentSid = getCurrentSessionId();
  ids = ids.filter(function(id) {
    var sid = _contentCards[id].sessionId;
    return !sid || sid === currentSid;
  });
  if (ids.length === 0) {
    bar.remove();
    return;
  }

  // Left: tab buttons
  var left = bar.querySelector('.card-tabs-left') || bar.appendChild(document.createElement('div'));
  left.className = 'card-tabs-left';
  left.innerHTML = ids.map(function(id) {
    var t = _contentCards[id].title || 'Content';
    return '<button class="card-tab' + (id === _cardActive ? ' active' : '') +
      '" onclick="switchCardTab(\'' + id + '\')">' +
      escapeHtml(t.length > 24 ? t.substring(0, 24) + '...' : t) +
      '</button>';
  }).join('');

  // Right: actions for the active card
  var right = bar.querySelector('.card-tabs-right') || bar.appendChild(document.createElement('div'));
  right.className = 'card-tabs-right';
  var entry = _contentCards[_cardActive];
  var localBtn = entry && entry.filePath
    ? '<span class="card-action-btn" onclick="openFileLocal(\'' + _cardActive + '\')">本地</span>'
    : '';
  if (entry && entry.htmlSource) {
    right.innerHTML = '<span class="card-action-btn' + (entry.showingSource ? ' on' : '') +
      '" onclick="toggleCardSource(\'' + _cardActive + '\')">' +
      (entry.showingSource ? '渲染' : '源码') + '</span>' + localBtn +
      '<span class="card-action-btn" onclick="refreshCard(\'' + _cardActive + '\')">刷新</span>' +
      '<span class="card-action-btn" onclick="screenshotCard(\'' + _cardActive + '\')">截图</span>' +
      '<span class="card-action-btn card-close-btn" onclick="removeContentCard(\'' + _cardActive + '\')">✕</span>';
  } else {
    right.innerHTML = localBtn +
      '<span class="card-action-btn" onclick="refreshCard(\'' + _cardActive + '\')">刷新</span>' +
      '<span class="card-action-btn" onclick="screenshotCard(\'' + _cardActive + '\')">截图</span>' +
      '<span class="card-action-btn card-close-btn" onclick="removeContentCard(\'' + _cardActive + '\')">✕</span>';
  }
}

function switchCardTab(id) {
  // Guard: don't switch to a card hidden for the current project
  var currentSid = getCurrentSessionId();
  var entry = _contentCards[id];
  if (entry && entry.sessionId && entry.sessionId !== currentSid) return;
  _cardActive = id;
  Object.keys(_contentCards).forEach(function(cid) {
    _contentCards[cid].el.style.display = cid === id ? 'flex' : 'none';
  });
  renderCardTabs();
}


function getCurrentSessionId() {
  if (currentView === 'home') return 'main';
  for (var i = 0; i < sessions.length; i++) {
    if (sessions[i].ref_id === currentView || sessions[i].id === currentView) {
      return sessions[i].session_id || sessions[i].id;
    }
  }
  return '';
}

function syncContentCards() {
  var stage = document.getElementById('stage');
  var currentSid = getCurrentSessionId();
  var visible = false;
  var firstVisible = null;
  for (var id in _contentCards) {
    var card = _contentCards[id];
    var belongs = !card.sessionId || card.sessionId === currentSid;
    card.el.style.display = belongs ? 'flex' : 'none';
    if (belongs) {
      if (!firstVisible) firstVisible = id;
      visible = true;
    }
  }
  if (visible) {
    var active = _contentCards[_cardActive];
    if (!active || active.el.style.display === 'none') _cardActive = firstVisible;
    stage.classList.add('split');
  } else {
    _cardActive = null;
    stage.classList.remove('split');
  }
  renderCardTabs();
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
  var isHtmlFile = data.is_html_file || false;

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
    isHtmlFile: isHtmlFile,
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

function openFileLocal(id) {
  var entry = _contentCards[id];
  if (!entry || !entry.filePath) return;
  fetch('/api/files/open', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ path: entry.filePath })
  });
}

function refreshCard(id) {
  var entry = _contentCards[id];
  if (!entry) return;
  var iframe = entry.el.querySelector('iframe');
  if (!iframe) return;
  iframe.srcdoc = entry.html || '';
}

function highlightSourceHTML(code) {
  return '<!DOCTYPE html><html><head><meta charset="UTF-8">' +
    '<link rel="stylesheet" href="https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.9.0/build/styles/atom-one-dark.min.css">' +
    '<style>body{font-family:monospace;font-size:13px;color:#e0d9f5;background:#141028;margin:0;padding:16px;white-space:pre-wrap;line-height:1.6}' +
    '.hljs{background:transparent!important}</style></head><body>' +
    '<pre><code>' + escapeHtml(code) + '</code></pre>' +
    '<script src="https://cdn.jsdelivr.net/gh/highlightjs/cdn-release@11.9.0/build/highlight.min.js"></script>' +
    '<script>hljs.highlightAll();</script>' +
    '</body></html>';
}

function toggleCardSource(id) {
  var entry = _contentCards[id];
  if (!entry || !entry.htmlSource) return;
  entry.showingSource = !entry.showingSource;
  var iframe = entry.el.querySelector('iframe');
  if (iframe) {
    iframe.srcdoc = entry.showingSource ? highlightSourceHTML(entry.htmlSource) : entry.html;
  }
  renderCardTabs();
}

function screenshotCard(id) {
  var entry = _contentCards[id];
  if (!entry || !entry.html) return;
  var html = entry.showingSource ? highlightSourceHTML(entry.htmlSource) : entry.html;

  fetch('/api/render-html', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ html: html, width: 800 })
  })
  .then(function(res) {
    if (!res.ok) throw new Error('render failed');
    return res.blob();
  })
  .then(function(blob) {
    var url = URL.createObjectURL(blob);
    var a = document.createElement('a');
    a.href = url;
    a.download = (entry.title || 'screenshot') + '.png';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  })
  .catch(function(err) {
    console.error('Screenshot failed:', err);
  });
}

function escapeAttr(s) {
  return s.replace(/&/g,'&amp;').replace(/"/g,'&quot;').replace(/</g,'&lt;').replace(/>/g,'&gt;');
}
