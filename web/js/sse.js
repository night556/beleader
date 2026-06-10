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
  fetch(SERVER_URL + '/api/messages?session_id=' + encodeURIComponent(sessionId) + '&limit=50')
    .then(function(r) { return r.json(); })
    .then(function(msgs) {
      if (!Array.isArray(msgs) || msgs.length === 0) {
        if (sessionId === 'main') {
          showIdle();
        } else {
          hideIdle();
          renderAll();
        }
        return;
      }
      var toolNameMap = {};
      for (var i = 0; i < msgs.length; i++) {
        var m = msgs[i];
        if (m.hidden) continue;
        // Build tool_call_id → function name map from assistant messages
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
      for (var i = 0; i < msgs.length; i++) {
        var m = msgs[i];
        if (m.hidden) continue;
        var item = null;
        if (m.role === 'user') {
          item = { type: 'user', icon: '\u{1F464}', label: 'You', content: m.content, status: 'done' };
        } else if (m.role === 'assistant') {
          var content = m.content || '';
          var hasToolCalls = false;
          try { var tc = JSON.parse(m.tool_calls || '[]'); hasToolCalls = Array.isArray(tc) && tc.length > 0; } catch(e) {}
          if (!content && hasToolCalls) continue;
          var html = content;
          try { html = marked.parse(content); } catch(e) {}
          item = { type: 'reply', icon: '✦', label: m.role_label || 'AI', content: html, status: 'done' };
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
          item = { type: 'tool', icon: toolIcon, label: toolLabel, detail: toolDetail, content: toolContent, status: hasError ? 'fail' : 'done', error: hasError };
        } else if (m.role === 'system') {
          continue;
        } else if (m.role === 'error') {
          item = { type: 'error', icon: '⚠', label: t('status.error'), content: m.content || '', status: 'fail' };
        } else if (m.role === 'notice') {
          item = { type: 'notice', icon: 'ℹ', label: '', content: m.content || '', status: 'done' };
        }
        if (item) pushTimelineItem(item);
      }
      hideIdle();
      currentStage = null;
      renderAll();
    })
    .catch(function(err) { console.error('load history error:', err); });
}

// Load main session history on startup
loadSessionMessages('main');

// SSE Event Stream
var evtSource = new EventSource(SERVER_URL + '/api/sse');
evtSource.onmessage = function(e) {
  try {
    var d = JSON.parse(e.data);
    window.updateState(d.type, d.payload);
  } catch(err) {
    console.error('[SSE] parse error:', err);
  }
};
evtSource.onerror = function() { console.error('[SSE] connection error'); };

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

// Central dispatcher
window.updateState = function(name, data) {
  data = data || {};
  var sid = data.session_id || '';

  // Filter timeline events to current view's session only
  var timelineTypes = ['thinking', 'tool_calls', 'tool_call', 'tool_result', 'responding', 'assistant_message', 'idle', 'stopped', 'context_compressed', 'notice'];
  if (timelineTypes.indexOf(name) !== -1 && !isCurrentViewSession(sid)) return;

  switch (name) {

    case 'thinking':
      state.name = 'thinking';
      updateStatus(t('status.thinking'), 'thinking');
      hideIdle();
      var thinkItem = { type: 'thinking', icon: '✦', label: '', content: '', status: 'running' };
      setLiveStage(thinkItem);
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
          if (parsed && parsed.content && !hasError) resultText = parsed.content;
        } catch(e2) {}

        var resultItem = null;
        for (var ri = timelineItems.length - 1; ri >= 0; ri--) {
          if (timelineItems[ri].tool_call_id === tcid2) { resultItem = timelineItems[ri]; break; }
        }
        if (resultItem) {
          resultItem.status = hasError ? 'fail' : 'done';
          resultItem.error = hasError;
          if (resultText && !hasError) resultItem.content = resultText;
          else if (hasError) resultItem.content = resultText || t('status.unknown_error');
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

    case 'speaking':
      if (typeof speak === 'function') speak(data.text);
      break;

    case 'responding':
    case 'assistant_message':
      state.name = 'responding';
      hideIdle();
      updateStatus(t('status.replying'), 'thinking');
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
        // keep current status — agent may still be in a loop
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
      updateStatus(t('status.ready'), 'idle');
      updateAgentBar();
      break;
  }
};

// Update project tabs in the top bar
function updateProjectTabs() {
  var tabRow = document.getElementById('tab-row');
  var children = tabRow.children;
  for (var i = children.length - 1; i >= 0; i--) {
    if (children[i].classList.contains('project-tab') || children[i].classList.contains('tab-item') && !children[i].classList.contains('home-tab')) {
      children[i].remove();
    }
  }
  var addBtn = document.getElementById('tab-add');

  sessions.forEach(function(s) {
    if (s.id === 'main') return;
    var btn = document.createElement('button');
    btn.className = 'tab-item project-tab';
    if (s.ref_id === currentView || s.id === currentView) btn.classList.add('active');
    btn.dataset.view = s.ref_id || s.id;
    btn.textContent = s.title || s.id;
    if (s.status === 'running') {
      var badge = document.createElement('span');
      badge.className = 'badge';
      btn.appendChild(badge);
    }
    btn.onclick = function() { switchView(s.ref_id || s.id); };
    tabRow.insertBefore(btn, addBtn);
  });
}

// Agent bar
function updateAgentBar() {
  renderAgentBar();
}

// Create project
function createProject() {
  var title = prompt(t('project.name_prompt'));
  if (!title) return;
  fetch(SERVER_URL + '/api/projects', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({title: title, prompt: ''})
  }).then(function(r) { return r.json(); })
    .then(function(p) {
      if (p.id) switchView(p.id);
    })
    .catch(function(e) { console.error('create project error:', e); });
}
