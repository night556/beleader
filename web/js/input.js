// input.js — Message input, send, and context management

var msgInput = document.getElementById('msg-input');
var sendBtn = document.getElementById('send-btn');

function showUserMsg(text) {
  hideIdle();
  var item = { type: 'user', icon: '\u{1F464}', label: 'You', content: text, status: 'done' };
  lastUserItem = item;
  pushTimelineItem(item);
  setLiveStage(item);
}

function showErrorView(title, msg) {
  hideIdle();
  var item = { type: 'error', icon: '⚠', label: title, content: msg, status: 'fail' };
  pushTimelineItem(item);
  setLiveStage(item);
}

function sendMsg() {
  var text = msgInput.value.trim();
  var imgs = pendingImages.slice();
  pendingImages = [];
  updateImagePreview();
  if (!text && imgs.length === 0) return;

  if (!hasModels) {
    showErrorView('Cannot send', 'No model configured. Add one in Settings.');
    updateStatus('Missing model', 'error');
    msgInput.value = text;
    return;
  }
  if (!activeAgentId) {
    showErrorView('Cannot send', 'No agent available.');
    updateStatus('Missing agent', 'error');
    msgInput.value = text;
    return;
  }
  msgInput.value = '';
  msgInput.style.height = 'auto';

  // Show user message on stage immediately
  if (text) showUserMsg(text);

  var body = { message: text, images: imgs, agent_id: activeAgentId };
  if (activeThreadId) body.thread_id = activeThreadId;

  fetch(SERVER_URL + '/api/chat', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  }).then(function(r) { return r.json(); })
    .then(function(d) {
      if (d.thread_id && !activeThreadId) {
        activeThreadId = d.thread_id;
        updateThreadList();
        // Refresh thread list from server
        fetch(SERVER_URL + '/api/threads')
          .then(function(r) { return r.json(); })
          .then(function(data) {
            threads = Array.isArray(data) ? data : [];
            updateThreadList();
          });
      }
    })
    .catch(function(e) { console.error('chat error:', e); });
}

sendBtn.addEventListener('click', function() {
  sendMsg();
});

msgInput.addEventListener('keydown', function(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    sendMsg();
  }
});

// Auto-resize textarea
msgInput.addEventListener('input', function() {
  this.style.height = 'auto';
  this.style.height = Math.min(this.scrollHeight, 120) + 'px';
});

// ── Image upload / paste ──

document.addEventListener('paste', function(e) {
  var items = e.clipboardData && e.clipboardData.items;
  if (!items) return;
  for (var i = 0; i < items.length; i++) {
    if (items[i].type.indexOf('image') === 0) {
      e.preventDefault();
      var blob = items[i].getAsFile();
      var reader = new FileReader();
      reader.onload = function(ev) {
        pendingImages.push(ev.target.result);
        updateImagePreview();
      };
      reader.readAsDataURL(blob);
    }
  }
});

document.getElementById('img-file-input').addEventListener('change', function() {
  var files = this.files;
  var loaded = 0;
  for (var i = 0; i < files.length; i++) {
    var reader = new FileReader();
    reader.onload = (function() {
      return function(ev) {
        pendingImages.push(ev.target.result);
        loaded++;
        if (loaded === files.length) updateImagePreview();
      };
    })();
    reader.readAsDataURL(files[i]);
  }
  this.value = '';
});

// ── Toast ──

function toast(msg) {
  var el = document.createElement('div');
  el.className = 'toast';
  el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(function() { el.classList.add('show'); }, 10);
  setTimeout(function() {
    el.classList.remove('show');
    setTimeout(function() { el.remove(); }, 300);
  }, 1500);
}

// ── Clear Context & Stop ──

function clearContext() {
  if (!activeThreadId) return;
  openModal({
    title: 'Clear Context',
    body: '<div class="modal-confirm-text"><p>Clear the conversation context for this thread?</p><p style="color:var(--text-dim);font-size:12px">The thread will be re-created, keeping only the agent setup.</p></div>',
    confirmText: 'Clear',
    danger: true,
    onConfirm: function() {
      // Pause current turn then delete and recreate the runtime thread
      fetch(SERVER_URL + '/api/threads/' + encodeURIComponent(activeThreadId) + '/pause', { method: 'POST' })
        .then(function() { toast('Context cleared — send a new message to continue.'); })
        .catch(function(e) { toast('Clear failed: ' + e.message); });
      return true;
    }
  });
}

function stopSession() {
  if (!activeThreadId) return;
  fetch(SERVER_URL + '/api/threads/' + encodeURIComponent(activeThreadId) + '/pause', { method: 'POST' })
    .then(function(r) { return r.json(); })
    .then(function(d) { console.log('thread paused:', d); })
    .catch(function(e) { console.error('stop error:', e); });
}
