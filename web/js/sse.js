// sse.js — SSE event stream and state dispatcher

// Fetch threads on startup
fetch(SERVER_URL + '/api/threads')
  .then(function(r) { return r.json(); })
  .then(function(data) {
    threads = Array.isArray(data) ? data : [];
    updateThreadList();
  })
  .catch(function(err) { console.error('fetch threads error:', err); });

// Fetch agents on startup (pick Default or first)
fetch(SERVER_URL + '/api/agents')
  .then(function(r) { return r.json(); })
  .then(function(data) {
    agents = data || [];
    _agentsCache = agents;
    for (var i = 0; i < agents.length; i++) {
      if (agents[i].name === 'Default') { activeAgentId = agents[i].id; break; }
    }
    if (!activeAgentId && agents.length > 0) activeAgentId = agents[0].id;
  })
  .catch(function(err) { console.error('fetch agents error:', err); });

// Check model configuration on startup
fetch(SERVER_URL + '/api/settings')
  .then(function(r) { return r.json(); })
  .then(function(cfg) {
    var models = (cfg.llm && cfg.llm.models) || [];
    if (models.length === 0) {
      hasModels = false;
      showNoModelPrompt();
    }
    var activeId = (cfg.llm && cfg.llm.active) || (models.length > 0 ? models[0].id : '');
    updateContextModel(activeId);
  })
  .catch(function(err) { console.error('fetch settings error:', err); });

// Load history messages for a thread into the timeline
function loadThreadMessages(threadId) {
  timelineItems = [];
  _itemSeq = 0;
  currentStage = null;
  _noMoreMessages = false;
  _loadingOlder = false;
  if (!threadId) {
    showIdle();
    return;
  }
  fetch(SERVER_URL + '/api/threads/' + encodeURIComponent(threadId) + '/messages')
    .then(function(r) { return r.json(); })
    .then(function(msgs) {
      if (!Array.isArray(msgs) || msgs.length === 0) {
        showIdle();
        return;
      }
      appendMessagesInternal(msgs, false);
      hideIdle();
      currentStage = null;
      renderAll(true);
    })
    .catch(function(err) { console.error('load history error:', err); });
}

// Convert DB messages → timelineItems
function appendMessagesInternal(msgs, prepend) {
  var toolNameMap = {};
  for (var i = 0; i < msgs.length; i++) {
    var m = msgs[i];
    if (m.kind === 'tool_call' && m.tool_calls) {
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
    var item = convertMessageToItem(m, toolNameMap);
    if (item) {
      item.thread_id = m.thread_id || '';
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
  if (m.kind === 'user_message') {
    return { type: 'user', icon: '\u{1F464}', label: 'You', content: m.content, status: 'done', db_id: m.id };
  } else if (m.kind === 'agent_message') {
    var content = m.content || '';
    var html = content;
    try { html = marked.parse(content); } catch(e) {}
    return { type: 'reply', icon: '✦', label: 'AI', content: html, status: 'done', db_id: m.id };
  } else if (m.kind === 'tool_call') {
    var tcs = [];
    try { tcs = JSON.parse(m.tool_calls || '[]'); } catch(e) {}
    if (tcs.length === 0) return null;
    var tc = tcs[0];
    var tn = tc.function && tc.function.name;
    if (!tn) return null;
    var args = {};
    try { args = JSON.parse(tc.function.arguments || '{}'); } catch(e) {}
    var meta = getToolMeta(tn, args);
    return {
      type: 'tool',
      icon: meta.icon,
      label: meta.label,
      detail: meta.detail,
      content: '',
      status: 'done',
      tool_call_id: tc.id,
      db_id: m.id
    };
  } else if (m.kind === 'tool_result') {
    var hasError = false;
    var resultText = m.content || '';
    try {
      var td = JSON.parse(m.content);
      if (td.content) resultText = td.content;
      if (td.error) { resultText = td.error; hasError = true; }
    } catch(e) {}
    return {
      type: 'tool',
      icon: '⚙',
      label: hasError ? 'Tool Error' : 'Tool Result',
      content: resultText,
      status: hasError ? 'fail' : 'done',
      error: hasError,
      tool_call_id: m.tool_call_id,
      db_id: m.id
    };
  } else if (m.kind === 'error') {
    return { type: 'error', icon: '⚠', label: 'Error', content: m.content || '', status: 'fail', db_id: m.id };
  }
  return null;
}

// Pagination: load older messages
function loadOlderMessages() {
  if (_loadingOlder || _noMoreMessages || !activeThreadId) return;
  var oldestId = null;
  for (var i = 0; i < timelineItems.length; i++) {
    var tid = timelineItems[i].db_id;
    if (tid && (oldestId === null || tid < oldestId)) oldestId = tid;
  }
  if (oldestId === null) return;
  _loadingOlder = true;
  showTopLoader(true);
  fetch(SERVER_URL + '/api/threads/' + encodeURIComponent(activeThreadId) + '/messages?after_id=' + oldestId)
    .then(function(r) { return r.json(); })
    .then(function(msgs) {
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

// ── SSE Event Stream ──

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
  updateStatus('Ready', 'idle');
  setTimeout(function () {
    if (evtSource && evtSource.readyState === EventSource.OPEN) hideConnBanner();
  }, 1500);

  if (!_sseFirstOpen) {
    console.log('[SSE] reconnected, refreshing');
    if (activeThreadId) loadThreadMessages(activeThreadId);
    fetch(SERVER_URL + '/api/threads')
      .then(function (r) { return r.json(); })
      .then(function (data) {
        threads = Array.isArray(data) ? data : [];
        updateThreadList();
      })
      .catch(function (err) { console.error('fetch threads error:', err); });
  }
  _sseFirstOpen = false;
}

function _onSSEError() {
  var st = evtSource ? evtSource.readyState : EventSource.CLOSED;
  if (st === EventSource.CLOSED) {
    showConnBanner('failed');
  } else {
    _sseRetryCount++;
    showConnBanner('retrying');
    if (_sseRetryTimer) clearTimeout(_sseRetryTimer);
    _sseRetryTimer = setTimeout(function () {
      if (evtSource && evtSource.readyState !== EventSource.OPEN) {
        console.log('[SSE] forcing reconnect');
        initSSE();
      }
    }, 10000);
  }
  updateStatus('Connection lost', 'error');
}

function initSSE() {
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

function showConnBanner(st) {
  var banner = document.getElementById('conn-banner');
  if (!banner) return;
  banner.style.display = 'flex';
  banner.classList.remove('connected', 'failed');
  var text = banner.querySelector('.conn-banner-text');
  var btn = banner.querySelector('.conn-banner-reconnect');
  if (st === 'connected') {
    banner.classList.add('connected');
    if (text) text.textContent = 'Connected';
    if (btn) btn.style.display = 'none';
  } else if (st === 'failed') {
    if (text) text.textContent = 'Connection lost. Reconnecting…';
    if (btn) btn.style.display = '';
  } else {
    if (text) text.textContent = 'Retrying… (' + _sseRetryCount + ')';
    if (btn) btn.style.display = '';
  }
}

function hideConnBanner() {
  var banner = document.getElementById('conn-banner');
  if (banner) banner.style.display = 'none';
}

initSSE();

// ── Central dispatcher ──

window.updateState = function(name, data) {
  data = data || {};
  var tid = data.session_id || '';

  // Filter timeline events to current thread only
  var timelineTypes = ['turn_started', 'response_start', 'response_delta', 'response_end',
    'tool_call_start', 'tool_call_result',
    'exec_command_begin', 'exec_command_output_delta', 'exec_command_end',
    'turn_complete', 'turn_aborted', 'error'];
  if (timelineTypes.indexOf(name) !== -1 && tid && tid !== activeThreadId) return;

  switch (name) {

    case 'turn_started':
      state.name = 'thinking';
      updateStatus('Thinking…', 'thinking');
      hideIdle();
      break;

    case 'response_start':
      // Prepare for reply — nothing visible yet
      break;

    case 'response_delta':
      state.name = 'responding';
      hideIdle();
      var deltaContent = data.delta || '';
      var channel = data.channel || 'text';
      if (!deltaContent) break;
      if (channel === 'reasoning') break;

      // Find or create streaming reply item for this turn
      var replyItem = null;
      for (var ri = timelineItems.length - 1; ri >= 0; ri--) {
        if (timelineItems[ri].type === 'reply' &&
            timelineItems[ri].status === 'running') {
          replyItem = timelineItems[ri];
          break;
        }
      }

      if (!replyItem) {
        updateStatus('Replying…', 'thinking');
        replyItem = {
          type: 'reply',
          icon: '✦',
          label: 'AI',
          content: '',
          status: 'running',
          thread_id: tid || ''
        };
        pushTimelineItem(replyItem);
      }

      replyItem.content += deltaContent;
      setLiveStage(replyItem);
      break;

    case 'response_end':
      // Finalize the streaming reply
      var finalReply = null;
      for (var fri = timelineItems.length - 1; fri >= 0; fri--) {
        if (timelineItems[fri].type === 'reply' &&
            timelineItems[fri].status === 'running') {
          finalReply = timelineItems[fri];
          break;
        }
      }

      if (finalReply) {
        finalReply.status = 'done';
        var finalContent = data.content || '';
        if (finalContent && finalContent !== finalReply.content) {
          finalReply.content = finalContent;
        }
        try { finalReply.content = marked.parse(finalReply.content); } catch(e) {}
        setLiveStage(finalReply);
      } else {
        // Fallback: no streaming reply, create from full content
        var cardHTML = data.content || '';
        if (cardHTML) {
          try { cardHTML = marked.parse(cardHTML); } catch(me) {}
          var fallbackItem = {
            type: 'reply',
            icon: '✦',
            label: 'AI',
            content: cardHTML,
            status: 'done'
          };
          pushTimelineItem(fallbackItem);
          setLiveStage(fallbackItem);
        }
      }
      break;

    case 'tool_call_start':
      state.name = 'tool_calls';
      hideIdle();
      var tn = data.tool_name || '';
      var args = {};
      try { args = JSON.parse(data.arguments || '{}'); } catch(ea) {}
      var meta = getToolMeta(tn, args);
      var toolItem = {
        type: 'tool',
        icon: meta.icon,
        label: meta.label,
        detail: meta.detail,
        content: '',
        status: 'running',
        error: false,
        tool_call_id: data.response_id,
        thread_id: tid
      };
      pushTimelineItem(toolItem);
      setLiveStage(toolItem);
      break;

    case 'tool_call_result':
      var tcid = data.response_id || '';
      var output = data.output;
      var resultText = '';
      var hasError = false;
      if (output && typeof output === 'object') {
        if (output.error) { resultText = output.error; hasError = true; }
        if (output.content) {
          resultText = hasError ? output.content + '\n\n' + resultText : output.content;
        }
      } else if (typeof output === 'string') {
        resultText = output;
      }

      var resultItem = null;
      for (var rj = timelineItems.length - 1; rj >= 0; rj--) {
        if (timelineItems[rj].tool_call_id === tcid) { resultItem = timelineItems[rj]; break; }
      }
      if (resultItem) {
        resultItem.status = hasError ? 'fail' : 'done';
        resultItem.error = hasError;
        resultItem.content = resultText || (hasError ? 'Unknown error' : '');
        if (currentStage && currentStage.item === resultItem) {
          updateExpandContent(resultItem);
        }
      }
      break;

    case 'exec_command_begin':
      hideIdle();
      var cmdItem = {
        type: 'tool',
        icon: '⬛',
        label: data.command || 'Command',
        detail: (data.cwd || '') + ' > ' + (data.command || ''),
        content: '',
        status: 'running',
        error: false,
        thread_id: tid
      };
      pushTimelineItem(cmdItem);
      setLiveStage(cmdItem);
      break;

    case 'exec_command_output_delta':
      var cmdDelta = data.delta || '';
      if (!cmdDelta) break;
      var cmdRunningItem = null;
      for (var ci = timelineItems.length - 1; ci >= 0; ci--) {
        if (timelineItems[ci].type === 'tool' &&
            timelineItems[ci].status === 'running' &&
            timelineItems[ci].label === (data.command || 'Command')) {
          cmdRunningItem = timelineItems[ci];
          break;
        }
      }
      if (cmdRunningItem) {
        cmdRunningItem.content += cmdDelta;
        if (currentStage && currentStage.item === cmdRunningItem) {
          updateExpandContent(cmdRunningItem);
        } else if (currentStage && currentStage.live) {
          setLiveStage(cmdRunningItem);
        }
      }
      break;

    case 'exec_command_end':
      var exitCode = data.exit_code;
      var cmdDoneItem = null;
      for (var cdj = timelineItems.length - 1; cdj >= 0; cdj--) {
        if (timelineItems[cdj].type === 'tool' &&
            timelineItems[cdj].status === 'running' &&
            timelineItems[cdj].label === (data.command || 'Command')) {
          cmdDoneItem = timelineItems[cdj];
          break;
        }
      }
      if (cmdDoneItem) {
        cmdDoneItem.status = exitCode === 0 ? 'done' : 'fail';
        cmdDoneItem.error = exitCode !== 0;
        if (exitCode !== 0) {
          cmdDoneItem.content += '\nExit code: ' + exitCode;
        }
        if (currentStage && currentStage.item === cmdDoneItem) {
          updateExpandContent(cmdDoneItem);
        }
      }
      break;

    case 'turn_complete':
      state.name = 'idle';
      updateStatus('Ready', 'idle');
      break;

    case 'turn_aborted':
      state.name = 'idle';
      updateStatus(data.reason === 'paused' ? 'Paused' : 'Stopped', 'idle');
      break;

    case 'error':
      state.name = 'error';
      var errMsg = data.message || 'Unknown error';
      updateStatus(errMsg, 'error');
      hideIdle();
      var errItem = {
        type: 'error',
        icon: '⚠',
        label: 'Error',
        content: errMsg,
        status: 'fail'
      };
      pushTimelineItem(errItem);
      setLiveStage(errItem);
      setTimeout(function() {
        if (state.name === 'error') { updateStatus('Ready', 'idle'); }
      }, 3000);
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

    case 'context_pct':
      _contextPcts[tid || activeThreadId || ''] = data.pct || 0;
      if (isCurrentThread(tid)) updateContextBar(data.pct || 0);
      break;
  }
};

// ── Sidebar thread list ──

function updateThreadList() {
  var nav = document.getElementById('sidebar-nav');
  // Remove old thread items (keep home)
  var children = nav.children;
  for (var i = children.length - 1; i >= 0; i--) {
    if (children[i].classList.contains('sidebar-thread')) {
      children[i].remove();
    }
  }

  threads.forEach(function(t) {
    var wrap = document.createElement('div');
    wrap.className = 'sidebar-item sidebar-thread';
    if (t.id === activeThreadId) wrap.classList.add('active');
    wrap.dataset.threadId = t.id;

    var nameSpan = document.createElement('span');
    nameSpan.className = 'sidebar-thread-name';
    nameSpan.textContent = t.title || t.id;
    wrap.onclick = function() { switchThread(t.id); };
    wrap.appendChild(nameSpan);

    var delBtn = document.createElement('button');
    delBtn.className = 'sidebar-project-del';
    delBtn.textContent = '×';
    delBtn.title = 'Delete thread';
    delBtn.onclick = function(e) {
      e.stopPropagation();
      deleteThread(t.id, t.title || t.id);
    };
    wrap.appendChild(delBtn);

    nav.appendChild(wrap);
  });
}

function deleteThread(id, title) {
  openModal({
    title: 'Delete Thread',
    body: '<div class="modal-confirm-text"><p>Delete thread <strong>' + escapeHtml(title) + '</strong>?</p><p style="font-size:11px;color:var(--text-dim)">This cannot be undone.</p></div>',
    confirmText: 'Delete',
    danger: true,
    onConfirm: function() {
      fetch(SERVER_URL + '/api/threads/' + encodeURIComponent(id), { method: 'DELETE' })
        .then(function(r) { return r.json(); })
        .then(function() {
          threads = threads.filter(function(t) { return t.id !== id; });
          updateThreadList();
          if (activeThreadId === id) {
            activeThreadId = null;
            timelineItems = [];
            _itemSeq = 0;
            currentStage = null;
            showIdle();
          }
        })
        .catch(function(e) { console.error('delete thread error:', e); });
      return true;
    }
  });
}

function newThread() {
  activeThreadId = null;
  timelineItems = [];
  _itemSeq = 0;
  currentStage = null;
  _noMoreMessages = false;
  updateThreadList();
  showIdle();
  updateStatus('Ready', 'idle');
  updateContextBar(0);
}

// Scroll listener for pagination
document.getElementById('timeline').addEventListener('scroll', function() {
  if (this.scrollTop < 50 && !_loadingOlder && !_noMoreMessages) {
    loadOlderMessages();
  }
});
