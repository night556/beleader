// sse.js — SSE event stream and state dispatcher

// Fetch existing projects on startup
fetch(SERVER_URL + '/api/projects')
  .then(function(r) { return r.json(); })
  .then(function(projects) {
    if (Array.isArray(projects)) {
      projects.forEach(function(p) {
        if (!sessions.find(function(s) { return s.id === p.id; })) {
          var pAgents = p.agents || [];
          var coordSid = '';
          for (var ai = 0; ai < pAgents.length; ai++) {
            if (pAgents[ai].role === 'coordinator') { coordSid = pAgents[ai].session_id || ''; break; }
          }
          sessions.push({id: p.id, ref_id: p.id, title: p.title || p.id, status: p.status, agents: pAgents, session_id: coordSid});
        }
      });
      updateProjectTabs();
    }
  })
  .catch(function(err) { console.error('fetch projects error:', err); });

// Check model configuration on startup
fetch(SERVER_URL + '/api/settings')
  .then(function(r) { return r.json(); })
  .then(function(cfg) {
    _workDir = cfg.work_dir || '';
    var models = (cfg.llm && cfg.llm.models) || [];
    if (models.length === 0) {
      hasModels = false;
      showNoModelPrompt();
    }
  })
  .catch(function(err) { console.error('fetch settings error:', err); });

// Load history messages for a session into the timeline
function loadSessionMessages(sessionId) {
  timelineItems = [];
  _itemSeq = 0;
  currentStage = null;
  _noMoreMessages = false;
  _loadingOlder = false;
  fetch(SERVER_URL + '/api/messages?session_id=' + encodeURIComponent(sessionId) + '&turns=10')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var msgs = (d && d.messages) ? d.messages : d;
      if (!Array.isArray(msgs) || msgs.length === 0) {
        if (sessionId === 'main') {
          showIdle();
        } else {
          hideIdle();
          renderAll(true);
        }
        return;
      }
      appendMessagesInternal(msgs, false);
      hideIdle();
      currentStage = null;
      renderAll(true);
    })
    .catch(function(err) { console.error('load history error:', err); });
}

// Convert DB messages → timelineItems and append (or prepend for pagination)
function appendMessagesInternal(msgs, prepend) {
  var toolNameMap = {};
  for (var i = 0; i < msgs.length; i++) {
    var m = msgs[i];
    if (m.hidden) continue;
    if (m.role === 'assistant' && m.tool_calls) {
      try {
        var tcs = JSON.parse(m.tool_calls);
        if (Array.isArray(tcs)) {
          for (var j = 0; j < tcs.length; j++) {
            if (tcs[j].id && tcs[j].function && tcs[j].function.name) {
              var args = {};
              try { args = JSON.parse(tcs[j].function.arguments || '{}'); } catch(ea) {}
              toolNameMap[tcs[j].id] = {name: tcs[j].function.name, args: args};
            }
          }
        }
      } catch(e) {}
    }
  }
  var newItems = [];
  for (var i = 0; i < msgs.length; i++) {
    var m = msgs[i];
    if (m.hidden) continue;
    var item = convertMessageToItem(m, toolNameMap);
    if (item) {
      item.session_id = m.session_id || '';
      newItems.push(item);
    }
  }
  if (prepend) {
    for (var k = newItems.length - 1; k >= 0; k--) {
      timelineItems.unshift(newItems[k]);
    }
  } else {
    for (var k = 0; k < newItems.length; k++) {
      timelineItems.push(newItems[k]);
    }
  }
}

function convertMessageToItem(m, toolNameMap) {
  if (m.role === 'user') {
    return { type: 'user', icon: '\u{1F464}', label: 'You', content: m.content, status: 'done', db_id: m.id, bookmarked: m.bookmarked };
  } else if (m.role === 'assistant') {
    var content = m.content || '';
    var hasToolCalls = false;
    try { var tc = JSON.parse(m.tool_calls || '[]'); hasToolCalls = Array.isArray(tc) && tc.length > 0; } catch(e) {}
    if (!content && hasToolCalls) return null;
    var html = content;
    try { html = marked.parse(content); } catch(e) {}
    return { type: 'reply', icon: '✦', label: m.role_label || 'AI', content: html, status: 'done', db_id: m.id, bookmarked: m.bookmarked };
  } else if (m.role === 'tool') {
    var toolContent = m.content || '';
    var toolLabel = t('status.tool');
    var toolIcon = '⚙';
    var toolDetail = '';
    var hasError = false;
    try {
      var td = JSON.parse(m.content);
      if (td.content) toolContent = td.content;
      if (td.error) { toolContent = td.error; hasError = true; }
    } catch(e) {}
    var entry = toolNameMap[m.tool_call_id];
    if (entry) {
      var meta = getToolMeta(entry.name, entry.args);
      toolIcon = meta.icon;
      toolLabel = meta.label + (hasError ? t('status.tool_error_short') : '');
      toolDetail = meta.detail;
    } else {
      toolLabel = hasError ? t('status.tool_error') : t('status.tool_result');
      toolDetail = '';
    }
    return { type: 'tool', icon: toolIcon, label: toolLabel, detail: toolDetail, content: toolContent, status: hasError ? 'fail' : 'done', error: hasError, tool_call_id: m.tool_call_id, db_id: m.id, bookmarked: m.bookmarked };
  } else if (m.role === 'system') {
    return null;
  } else if (m.role === 'error') {
    return { type: 'error', icon: '⚠', label: t('status.error'), content: m.content || '', status: 'fail', db_id: m.id, bookmarked: m.bookmarked };
  } else if (m.role === 'notice') {
    return { type: 'notice', icon: 'ℹ', label: '', content: m.content || '', status: 'done', db_id: m.id, bookmarked: m.bookmarked };
  }
  return null;
}

// Pagination: load older messages (scroll to top)
function loadOlderMessages() {
  if (_loadingOlder || _noMoreMessages) return;
  var oldestId = null;
  for (var i = 0; i < timelineItems.length; i++) {
    var tid = timelineItems[i].db_id;
    if (tid && (oldestId === null || tid < oldestId)) oldestId = tid;
  }
  if (oldestId === null) return;
  _loadingOlder = true;
  showTopLoader(true);
  var sid = currentView === 'home' ? 'main' : currentView;
  fetch(SERVER_URL + '/api/messages?session_id=' + encodeURIComponent(sid) + '&before_id=' + oldestId + '&turns=10')
    .then(function(r) { return r.json(); })
    .then(function(d) {
      var msgs = (d && d.messages) ? d.messages : d;
      if (!Array.isArray(msgs) || msgs.length === 0) {
        _noMoreMessages = true;
      } else {
        prependMessages(msgs);
      }
    })
    .catch(function(e) { console.error('load older error:', e); })
    .finally(function() {
      _loadingOlder = false;
      showTopLoader(false);
    });
}

// Load main session history on startup
loadSessionMessages('main');

// ── SSE Event Stream ──
// Named handlers so we can removeEventListener before close() — guarantees
// old handlers never fire on a new EventSource after reconnect.
var evtSource = null;
var _sseRetryCount = 0;
var _sseRetryTimer = null;
var _sseFirstOpen = true;

function _onSSEMessage(e) {
  try {
    var d = JSON.parse(e.data);
    window.updateState(d.type, d.payload);
  } catch (err) {
    console.error('[SSE] parse error:', err);
  }
}

function _onSSEOpen() {
  _sseRetryCount = 0;
  showConnBanner('connected');
  updateStatus(t('status.ready'), 'idle');
  setTimeout(function () {
    if (evtSource && evtSource.readyState === EventSource.OPEN) hideConnBanner();
  }, 1500);

  if (!_sseFirstOpen) {
    console.log('[SSE] reconnected, refreshing current view');
    var sid = currentView === 'home' ? 'main' : currentView;
    loadSessionMessages(sid);
    fetch(SERVER_URL + '/api/projects')
      .then(function (r) { return r.json(); })
      .then(function (projects) {
        if (Array.isArray(projects)) {
          sessions = [];
          projects.forEach(function (p) {
            var pAgents = p.agents || [];
            var coordSid = '';
            for (var ai = 0; ai < pAgents.length; ai++) {
              if (pAgents[ai].role === 'coordinator') { coordSid = pAgents[ai].session_id || ''; break; }
            }
            sessions.push({ id: p.id, ref_id: p.id, title: p.title || p.id, status: p.status, agents: pAgents, session_id: coordSid });
          });
          updateProjectTabs();
        }
      })
      .catch(function (err) { console.error('fetch projects error:', err); });
  }
  _sseFirstOpen = false;
}

function _onSSEError() {
  var state = evtSource ? evtSource.readyState : EventSource.CLOSED;
  if (state === EventSource.CLOSED) {
    showConnBanner('failed');
  } else {
    _sseRetryCount++;
    showConnBanner('retrying');
    if (_sseRetryTimer) clearTimeout(_sseRetryTimer);
    _sseRetryTimer = setTimeout(function () {
      if (evtSource && evtSource.readyState !== EventSource.OPEN) {
        console.log('[SSE] forcing reconnect after backoff timeout');
        initSSE();
      }
    }, 10000);
  }
  updateStatus(t('conn.lost'), 'error');
}

function initSSE() {
  // Tear down old connection cleanly: remove listeners BEFORE close so no
  // queued event can fire a stale handler on the new EventSource.
  if (evtSource) {
    evtSource.removeEventListener('message', _onSSEMessage);
    evtSource.removeEventListener('open', _onSSEOpen);
    evtSource.removeEventListener('error', _onSSEError);
    try { evtSource.close(); } catch (e) {}
    evtSource = null;
  }
  if (_sseRetryTimer) { clearTimeout(_sseRetryTimer); _sseRetryTimer = null; }

  evtSource = new EventSource(SERVER_URL + '/api/sse?t=' + Date.now());
  evtSource.addEventListener('message', _onSSEMessage);
  evtSource.addEventListener('open', _onSSEOpen);
  evtSource.addEventListener('error', _onSSEError);
}

function reconnectSSE() {
  _sseRetryCount = 0;
  showConnBanner('retrying');
  initSSE();
}

function showConnBanner(state) {
  var banner = document.getElementById('conn-banner');
  if (!banner) return;
  banner.style.display = 'flex';
  banner.classList.remove('connected', 'failed');
  var text = banner.querySelector('.conn-banner-text');
  var btn = banner.querySelector('.conn-banner-reconnect');
  if (state === 'connected') {
    banner.classList.add('connected');
    if (text) text.textContent = t('conn.connected');
    if (btn) btn.style.display = 'none';
  } else if (state === 'failed') {
    if (text) text.textContent = t('conn.lost');
    if (btn) btn.style.display = '';
  } else {
    if (text) text.textContent = t('conn.retrying', { $1: _sseRetryCount });
    if (btn) btn.style.display = '';
  }
}

function hideConnBanner() {
  var banner = document.getElementById('conn-banner');
  if (banner) banner.style.display = 'none';
}

initSSE();

// Check if a session_id belongs to the current view
function isCurrentViewSession(sid) {
  if (!sid) return true; // events without session_id pass through
  if (currentView === 'home') return sid === 'main';

  // If drilling into a specific agent, only match that agent
  if (_agentFilter) return sid === _agentFilter;

  // Project view: match any agent in the project (coordinator + workers)
  for (var i = 0; i < sessions.length; i++) {
    if (sessions[i].ref_id === currentView || sessions[i].id === currentView) {
      if (sid === sessions[i].session_id) return true;
      var agents = sessions[i].agents;
      if (agents) {
        for (var j = 0; j < agents.length; j++) {
          if (sid === agents[j].session_id) return true;
        }
      }
      return false;
    }
  }
  return false;
}

function maybeResetStatus() {
  if (currentView === 'home') {
    updateStatus(t('status.ready'), 'idle');
    return;
  }
  for (var ri = 0; ri < sessions.length; ri++) {
    if (sessions[ri].ref_id === currentView || sessions[ri].id === currentView) {
      var agents = sessions[ri].agents || [];
      var allIdle = true;
      for (var rj = 0; rj < agents.length; rj++) {
        if (agents[rj].status !== 'idle') { allIdle = false; break; }
      }
      if (allIdle) updateStatus(t('status.ready'), 'idle');
      return;
    }
  }
}

// Central dispatcher
window.updateState = function(name, data) {
  data = data || {};
  var sid = data.session_id || '';

  // Filter timeline events to current view's session only
  var timelineTypes = ['thinking', 'tool_calls', 'tool_call', 'tool_result', 'responding', 'assistant_message', 'assistant_message_chunk', 'idle', 'stopped', 'context_compressed', 'notice'];
  if (timelineTypes.indexOf(name) !== -1 && !isCurrentViewSession(sid)) return;

  switch (name) {

    case 'thinking':
      state.name = 'thinking';
      updateStatus(t('status.thinking'), 'thinking');
      hideIdle();
      // Thinking is a status-bar indicator only — not a timeline item
      break;

    case 'tool_call':
    case 'tool_calls':
      state.name = 'tool_calls';
      hideIdle();
      try {
        var tcs = JSON.parse(data.tool_calls || '[]');
        for (var i = 0; i < tcs.length; i++) {
          var tn = tcs[i].function && tcs[i].function.name;
          if (!tn) continue;
          var args = {};
          try { args = JSON.parse(tcs[i].function.arguments || '{}'); } catch(ea) {}

          var meta = getToolMeta(tn, args);
          var label = meta.label;
          if (sid) {
            var ag = getAgentBySession(sid);
            if (ag && ag.agent && ag.agent.role !== 'coordinator') {
              label = '[' + (ag.agent.name || 'worker') + '] ' + label;
            }
          }
          var toolItem = {
            type: 'tool',
            icon: meta.icon,
            label: label,
            detail: meta.detail,
            content: '',
            status: 'running',
            error: false,
            tool_call_id: tcs[i].id,
            session_id: sid
          };
          pushTimelineItem(toolItem);
          setLiveStage(toolItem);

          if (sid) {
            _agentActivities[sid] = { text: t('status.calling_tool') + tn, since: Date.now() };
            updateAgentBar();
          }
        }
      } catch(e) {}
      break;

    case 'tool_progress':
      try {
        var content = data.content || '';
        if (!content) break;
        var tcid = data.tool_call_id || '';
        var progItem = null;
        for (var pi = timelineItems.length - 1; pi >= 0; pi--) {
          if (timelineItems[pi].tool_call_id === tcid) { progItem = timelineItems[pi]; break; }
        }
        if (progItem) {
          progItem.content += content;
          if (currentStage && currentStage.item === progItem) {
            updateExpandContent(progItem);
          } else if (currentStage && currentStage.live) {
            setLiveStage(progItem);
          }
        }
      } catch(e) {}
      break;

    case 'tool_result':
      try {
        var tcid2 = data.tool_call_id || '';
        var resultText = '';
        var hasError = false;
        try {
          var parsed = typeof data.content === 'string' ? JSON.parse(data.content) : data.content;
          if (parsed && parsed.error) { resultText = parsed.error; hasError = true; }
          if (parsed && parsed.content) {
            // Show stdout/stderr even when there's an error
            resultText = hasError ? parsed.content + '\n\n' + resultText : parsed.content;
          }
        } catch(e2) {}

        var resultItem = null;
        for (var ri = timelineItems.length - 1; ri >= 0; ri--) {
          if (timelineItems[ri].tool_call_id === tcid2) { resultItem = timelineItems[ri]; break; }
        }
        if (resultItem) {
          resultItem.status = hasError ? 'fail' : 'done';
          resultItem.error = hasError;
          resultItem.content = resultText || (hasError ? t('status.unknown_error') : '');
          if (currentStage && currentStage.item === resultItem) {
            updateExpandContent(resultItem);
          }
        }

        if (hasError && sid) {
          _agentActivities[sid] = { text: t('status.tool_error_activity'), since: Date.now() };
          updateAgentBar();
        } else if (sid) {
          _agentActivities[sid] = { text: t('status.tool_done'), since: Date.now() };
          updateAgentBar();
        }
      } catch(e3) {}
      break;

    case 'assistant_message_chunk':
      hideIdle();
      var chunkContent = data.content || '';
      if (!chunkContent) break;

      var chunkItem = null;
      for (var ci2 = timelineItems.length - 1; ci2 >= 0; ci2--) {
        if (timelineItems[ci2].type === 'reply' &&
            timelineItems[ci2].status === 'running' &&
            (timelineItems[ci2].session_id || '') === (sid || '')) {
          chunkItem = timelineItems[ci2];
          break;
        }
      }

      if (!chunkItem) {
        updateStatus(t('status.replying'), 'thinking');
        chunkItem = {
          type: 'reply',
          icon: '✦',
          label: data.role_label || t('timeline.ai_reply'),
          content: '',
          status: 'running',
          session_id: sid || ''
        };
        pushTimelineItem(chunkItem);
      }

      chunkItem.content += chunkContent;
      setLiveStage(chunkItem);
      break;

    case 'speaking':
      if (typeof speak === 'function') speak(data.text);
      break;

    case 'responding':
    case 'assistant_message':
      state.name = 'responding';
      hideIdle();
      updateStatus(t('status.replying'), 'thinking');

      // Finalize any existing streaming reply item
      var streamItem = null;
      for (var si2 = timelineItems.length - 1; si2 >= 0; si2--) {
        if (timelineItems[si2].type === 'reply' &&
            timelineItems[si2].status === 'running' &&
            (timelineItems[si2].session_id || '') === (sid || '')) {
          streamItem = timelineItems[si2];
          break;
        }
      }

      if (streamItem) {
        streamItem.status = 'done';
        var finalContent = data.content || '';
        if (finalContent && finalContent !== streamItem.content) {
          streamItem.content = finalContent;
        }
        // Parse accumulated markdown to HTML for done rendering
        try { streamItem.content = marked.parse(streamItem.content); } catch(e) {}
        setLiveStage(streamItem);
      } else {
        // Fallback: no streaming reply, create one from the full content
        var cardHTML = data.html || '';
        if (!cardHTML && data.content) {
          try { cardHTML = marked.parse(data.content); } catch(me) { cardHTML = data.content; }
        }
        var isWorker = data.role_label && data.role_label !== 'coordinator';
        if (cardHTML && (!isWorker || _agentFilter === sid)) {
          var replyItem = {
            type: 'reply',
            icon: '✦',
            label: t('timeline.ai_reply'),
            content: cardHTML,
            status: 'done'
          };
          pushTimelineItem(replyItem);
          setLiveStage(replyItem);
        }
      }
      if (sid) {
        _agentActivities[sid] = { text: t('status.replying_activity'), since: Date.now() };
        updateAgentBar();
      }
      break;

    case 'notice':
      hideIdle();
      var noticeItem = {
        type: 'notice',
        icon: 'ℹ',
        label: '',
        content: data.content || '',
        status: 'done'
      };
      pushTimelineItem(noticeItem);
      setLiveStage(noticeItem);
      break;

    case 'error':
      state.name = 'error';
      var errMsg = data.message || t('status.unknown_error');
      updateStatus(errMsg, 'error');
      hideIdle();
      var errLabel = t('status.error');
      if (sid && !isCurrentViewSession(sid)) {
        var agInfo = getAgentBySession(sid);
        if (agInfo && agInfo.agent && agInfo.agent.name) {
          errLabel = '[' + agInfo.agent.name + '] ' + t('status.error');
        } else if (sid !== 'main') {
          errLabel = '[' + sid + '] ' + t('status.error');
        }
      }
      // Update agent status to idle in sessions array
      for (var esi = 0; esi < sessions.length; esi++) {
        if (sessions[esi].agents) {
          for (var eai = 0; eai < sessions[esi].agents.length; eai++) {
            if (sessions[esi].agents[eai].session_id === sid) {
              sessions[esi].agents[eai].status = 'idle';
              break;
            }
          }
        }
      }
      updateAgentBar();
      var errItem = {
        type: 'error',
        icon: '⚠',
        label: errLabel,
        content: errMsg,
        status: 'fail'
      };
      pushTimelineItem(errItem);
      setLiveStage(errItem);
      setTimeout(function() {
        if (state.name === 'error') { updateStatus(t('status.ready'), 'idle'); }
      }, 3000);
      break;

    case 'project_created':
      var pExisting = sessions.find(function(s) { return s.id === data.ref_id; });
      if (!pExisting) {
        sessions.push({
          id: data.ref_id, ref_id: data.ref_id, title: data.title || data.ref_id,
          status: 'running', session_id: data.session_id || '',
          agents: [{ name: 'coordinator', session_id: data.session_id || '', role: 'coordinator', status: 'running' }]
        });
        updateProjectTabs();
      }
      break;

    case 'project_completed':
      for (var pci = 0; pci < sessions.length; pci++) {
        if (sessions[pci].ref_id === data.ref_id) {
          sessions[pci].status = 'completed';
          if (sessions[pci].agents) {
            for (var ak2 = 0; ak2 < sessions[pci].agents.length; ak2++) {
              sessions[pci].agents[ak2].status = 'idle';
              delete _agentActivities[sessions[pci].agents[ak2].session_id];
            }
          }
          break;
        }
      }
      updateProjectTabs();
      updateAgentBar();
      maybeResetStatus();
      break;

    case 'worker_spawned':
      console.log('[worker_spawned]', 'ref_id=' + data.ref_id, 'name=' + data.name, 'sid=' + data.session_id, 'sessions.length=' + sessions.length);
      for (var wsi = 0; wsi < sessions.length; wsi++) {
        console.log('[worker_spawned] checking session', wsi, 'ref_id=' + sessions[wsi].ref_id, 'id=' + sessions[wsi].id);
        if (sessions[wsi].ref_id === data.ref_id) {
          if (!sessions[wsi].agents) sessions[wsi].agents = [];
          if (!sessions[wsi].agents.find(function(a) { return a.session_id === data.session_id; })) {
            sessions[wsi].agents.push({name: data.name, session_id: data.session_id, role: data.role, status:'running'});
            console.log('[worker_spawned] added agent', data.name, data.session_id, 'to project', data.ref_id);
          }
          break;
        }
      }
      updateProjectTabs();
      updateAgentBar();
      break;

    case 'worker_completed':
    case 'worker_terminated':
      for (var wci = 0; wci < sessions.length; wci++) {
        if (sessions[wci].ref_id === data.ref_id && sessions[wci].agents) {
          for (var ak = 0; ak < sessions[wci].agents.length; ak++) {
            if (sessions[wci].agents[ak].name === data.worker_name) {
              sessions[wci].agents[ak].status = 'idle';
              break;
            }
          }
          break;
        }
      }
      updateProjectTabs();
      updateAgentBar();
      break;

    case 'worker_deleted':
      for (var wdi = 0; wdi < sessions.length; wdi++) {
        if (sessions[wdi].ref_id === data.ref_id && sessions[wdi].agents) {
          for (var am = 0; am < sessions[wdi].agents.length; am++) {
            if (sessions[wdi].agents[am].name === data.worker_name) {
              sessions[wdi].agents.splice(am, 1);
              break;
            }
          }
          break;
        }
      }
      updateProjectTabs();
      updateAgentBar();
      maybeResetStatus();
      break;

    case 'worker_intervened':
      if (data.status === 'restarted') {
        for (var wii = 0; wii < sessions.length; wii++) {
          if (sessions[wii].ref_id === data.ref_id && sessions[wii].agents) {
            for (var wj = 0; wj < sessions[wii].agents.length; wj++) {
              if (sessions[wii].agents[wj].name === data.worker_name) {
                sessions[wii].agents[wj].status = 'running';
                break;
              }
            }
            break;
          }
        }
      }
      updateAgentBar();
      break;

    case 'session_stopped':
    case 'project_deleted':
      for (var di = 0; di < sessions.length; di++) {
        if (sessions[di].ref_id === data.ref_id || sessions[di].id === data.ref_id) {
          sessions.splice(di, 1);
          break;
        }
      }
      var stillActive = sessions.find(function(s) { return s.id === activeSessionId; });
      if (!stillActive) activeSessionId = 'main';
      updateProjectTabs();
      if (currentView !== 'home' && !sessions.find(function(s) { return s.ref_id === currentView || s.id === currentView; })) {
        switchView('home');
      }
      hideAgentBar();
      updateStatus(t('status.ready'), 'idle');
      break;

    case 'context_pct':
      _contextPcts[sid || 'main'] = data.pct || 0;
      if (isCurrentViewSession(sid)) updateContextBar(data.pct || 0);
      break;

    case 'token_total':
      _sessionTokens[sid || 'main'] = data.session_total || 0;
      if (data.project_id) {
        _projectTokens[data.project_id] = data.project_total || 0;
        if (currentView === data.project_id) {
          updateContextTokens(data.project_total || 0);
        }
      } else if (isCurrentViewSession(sid)) {
        updateContextTokens(data.session_total || 0);
      }
      break;

    case 'session_focused':
      var focusRef = data.session_id || data.ref_id || '';
      if (focusRef === 'main') focusRef = 'home';
      if (focusRef && currentView !== focusRef) {
        switchView(focusRef);
      }
      break;

    case 'content_created':
      var cardData = {
        id: data.id,
        title: data.title || '',
        html: data.html || '',
        html_source: data.html_source || '',
        is_html_file: data.is_html_file || false,
        file_path: data.file_path || '',
        session_id: data.session_id || '',
        width: data.width || 800,
        height: data.height || 500
      };
      createContentCard(cardData);
      break;

    case 'content_removed':
      removeContentCard(data.id || '');
      break;

    case 'idle':
    case 'stopped':
      // Update agent status in sessions array
      if (sid) {
        console.log('[idle/stopped] sid=' + sid, 'type=' + name);
        delete _agentActivities[sid];
        for (var si = 0; si < sessions.length; si++) {
          if (sessions[si].agents) {
            for (var aj = 0; aj < sessions[si].agents.length; aj++) {
              if (sessions[si].agents[aj].session_id === sid) {
                console.log('[idle/stopped] setting agent ' + sessions[si].agents[aj].name + ' to idle');
                sessions[si].agents[aj].status = 'idle';
                break;
              }
            }
          }
        }
      }
      state.name = name;
      maybeResetStatus();
      updateAgentBar();
      break;
  }
};

// Render sidebar project items
function updateProjectTabs() {
  var nav = document.getElementById('sidebar-nav');
  // Remove old project items (keep the home item)
  var children = nav.children;
  for (var i = children.length - 1; i >= 0; i--) {
    if (children[i].classList.contains('sidebar-project')) {
      children[i].remove();
    }
  }

  sessions.forEach(function(s) {
    if (s.id === 'main') return;
    var wrap = document.createElement('div');
    wrap.className = 'sidebar-item sidebar-project';
    if (s.ref_id === currentView || s.id === currentView) wrap.classList.add('active');
    wrap.dataset.view = s.ref_id || s.id;

    var nameSpan = document.createElement('span');
    nameSpan.className = 'sidebar-project-name';
    nameSpan.textContent = s.title || s.id;
    nameSpan.onclick = function() { switchView(s.ref_id || s.id); };
    wrap.appendChild(nameSpan);

    if (s.status === 'running') {
      var dot = document.createElement('span');
      dot.className = 'side-dot';
      nameSpan.appendChild(dot);
    }

    var delBtn = document.createElement('button');
    delBtn.className = 'sidebar-project-del';
    delBtn.textContent = '×';
    delBtn.title = t('project.del_tooltip');
    delBtn.onclick = function(e) {
      e.stopPropagation();
      deleteProject(s.ref_id || s.id, s.title || s.id);
    };
    wrap.appendChild(delBtn);

    nav.appendChild(wrap);
  });
}

function deleteProject(refId, title) {
  var projectDir = _workDir ? _workDir.replace(/\\/g, '/') + '/' + refId : refId;
  openModal({
    title: t('project.delete_title'),
    body: '<div class="modal-confirm-text">' +
      '<p style="font-size:13px;color:var(--text);margin-bottom:8px">' + t('project.delete_confirm', { $1: '<strong>' + escapeHtml(title) + '</strong>' }) + '</p>' +
      '<p style="font-size:12px;color:#c4554d;margin-bottom:12px;padding:8px 12px;background:rgba(196,85,77,0.06);border:1px solid rgba(196,85,77,0.15);border-radius:6px">' + t('project.delete_warning') + '</p>' +
      '<p style="font-size:11px;color:var(--text-dim);word-break:break-all">' + t('project.delete_dir_label') + ' <code style="background:rgba(0,0,0,0.04);padding:2px 6px;border-radius:4px;font-family:monospace;font-size:11px">' + escapeHtml(projectDir) + '</code></p>' +
      '</div>',
    confirmText: t('project.delete_confirm_btn'),
    cancelText: t('modal.cancel'),
    danger: true,
    onConfirm: function() {
      fetch(SERVER_URL + '/api/projects/' + encodeURIComponent(refId), {
        method: 'DELETE'
      }).then(function(r) { return r.json(); })
        .then(function(d) {
          if (d.error) { console.error('delete project error:', d.error); }
        })
        .catch(function(e) { console.error('delete project error:', e); });
      return true;
    }
  });
}

// Toggle sidebar (desktop + mobile)
function toggleSidebar() {
  var closed = document.body.classList.toggle('sb-closed');
  // On mobile, sidebar overlays — sync backdrop
  if (window.innerWidth <= 860) {
    if (closed) {
      document.getElementById('backdrop').classList.remove('open');
    } else {
      document.getElementById('backdrop').classList.add('open');
    }
  }
}

// On mobile, start with sidebar closed
if (window.innerWidth <= 860) {
  document.body.classList.add('sb-closed');
}

// Agent bar
function updateAgentBar() {
  renderAgentBar();
}

// Create project — opens modal instead of native prompt()
function createProject() {
  openModal({
    title: t('project.new_title'),
    body: '<div class="modal-field"><label>' + t('project.name_placeholder') + '</label>' +
          '<input type="text" id="project-name-input" class="modal-input" placeholder="' + t('project.name_placeholder') + '" autofocus></div>',
    confirmText: t('project.create'),
    onConfirm: function() {
      var input = document.getElementById('project-name-input');
      var title = input ? input.value.trim() : '';
      if (!title) return false;
      fetch(SERVER_URL + '/api/projects', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({title: title, prompt: ''})
      }).then(function(r) { return r.json(); })
        .then(function(p) {
          if (p.id) switchView(p.id);
        })
        .catch(function(e) { console.error('create project error:', e); });
      return true;
    }
  });
  setTimeout(function() {
    var input = document.getElementById('project-name-input');
    if (input) {
      input.focus();
      input.addEventListener('keydown', function(e) {
        if (e.key === 'Enter') {
          e.preventDefault();
          var confirmBtn = document.querySelector('#modal-foot .modal-btn.primary');
          if (confirmBtn) confirmBtn.click();
        }
      });
      input.addEventListener('input', function() {
        var confirmBtn = document.querySelector('#modal-foot .modal-btn.primary');
        if (confirmBtn) confirmBtn.disabled = input.value.trim() === '';
      });
      var confirmBtn = document.querySelector('#modal-foot .modal-btn.primary');
      if (confirmBtn) confirmBtn.disabled = true;
    }
  }, 50);
}
