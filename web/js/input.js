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
    showErrorView(t('error.cannot_send'), t('error.no_model_msg'));
    updateStatus(t('error.missing_model'), 'error');
    msgInput.value = text;
    return;
  }
  msgInput.value = '';
  msgInput.style.height = 'auto';

  // Show user message on stage immediately
  if (text) showUserMsg(text);

  var view = currentView;
  if (view === 'home') {
    fetch(SERVER_URL + '/api/chat', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message: text, images: imgs })
    });
  } else {
    var s = null;
    for (var i = 0; i < sessions.length; i++) {
      if (sessions[i].ref_id === view || sessions[i].id === view) { s = sessions[i]; break; }
    }
    if (s && s.ref_id) {
      fetch(SERVER_URL + '/api/projects/' + s.ref_id + '/intervene', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: text, images: imgs })
      });
    } else {
      console.error('[sendMsg] session not found for view:', view);
    }
  }
}

sendBtn.addEventListener('click', function() {
  if (voiceMode) stopVoiceAndDeactivate();
  sendMsg();
});

msgInput.addEventListener('keydown', function(e) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault();
    if (voiceMode) stopVoiceAndDeactivate();
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

// ── Clear Context & Stop ──

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

function clearContext() {
  var sess = null;
  for (var i = 0; i < sessions.length; i++) {
    if (sessions[i].id === currentView || sessions[i].ref_id === currentView) { sess = sessions[i]; break; }
  }
  var sid = sess ? (sess.session_id || sess.id) : 'main';
  fetch(SERVER_URL + '/api/sessions/' + sid + '/clear', { method: 'POST' })
    .then(function(r) { return r.json(); })
    .then(function() {
      toast(t('toast.context_cleared'));
      timelineItems = [];
      renderAll();
    })
    .catch(function(e) { toast(t('toast.clear_failed') + e.message); });
}

function stopSession() {
  var sess = null;
  for (var i = 0; i < sessions.length; i++) {
    if (sessions[i].id === currentView || sessions[i].ref_id === currentView) { sess = sessions[i]; break; }
  }
  var sid = sess ? (sess.session_id || sess.id) : 'main';
  fetch(SERVER_URL + '/api/sessions/' + sid + '/stop', { method: 'POST' })
    .then(function(r) { return r.json(); })
    .then(function(d) { console.log('session stopped:', d); })
    .catch(function(e) { console.error('stop error:', e); });
}
