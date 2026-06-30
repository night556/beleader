// knowledge.js — Knowledge base panel

var knowledgePage = 0;
var knowledgePageSize = 20;

function toggleKnowledge() {
  var panel = document.getElementById('knowledge-panel');
  var isOpen = panel.classList.contains('open');
  if (isOpen) {
    panel.classList.remove('open');
  } else {
    var sp = document.getElementById('settings-panel');
    var bp = document.getElementById('bookmarks-panel');
    var ap = document.getElementById('agents-panel');
    var tp = document.getElementById('tools-panel');
    var mp = document.getElementById('mcp-panel');
    if (sp) sp.classList.remove('open');
    if (bp) bp.classList.remove('open');
    if (ap) ap.classList.remove('open');
    if (tp) tp.classList.remove('open');
    if (mp) mp.classList.remove('open');
    panel.classList.add('open');
    knowledgePage = 0;
    loadKnowledge();
  }
}

function loadKnowledge() {
  var offset = knowledgePage * knowledgePageSize;
  fetch(SERVER_URL + '/api/knowledge?limit=' + knowledgePageSize + '&offset=' + offset)
    .then(function(r) { return r.json(); })
    .then(function(data) {
      renderKnowledgeList(data.knowledge || [], data.count || 0);
    })
    .catch(function(e) { console.error('load knowledge error:', e); });
}

function searchKnowledgeUI() {
  var q = document.getElementById('knowledge-search').value.trim();
  if (!q) {
    knowledgePage = 0;
    loadKnowledge();
    return;
  }
  fetch(SERVER_URL + '/api/knowledge/search?q=' + encodeURIComponent(q) + '&limit=50')
    .then(function(r) { return r.json(); })
    .then(function(data) {
      renderKnowledgeList(data.knowledge || [], data.count || 0);
    })
    .catch(function(e) { console.error('search knowledge error:', e); });
}

function renderKnowledgeList(items, total) {
  var list = document.getElementById('knowledge-list');
  var pag = document.getElementById('knowledge-pagination');

  if (!items.length) {
    list.innerHTML = '<div class="knowledge-empty">No knowledge entries yet.</div>';
    pag.innerHTML = '';
    return;
  }

  var html = '';
  for (var i = 0; i < items.length; i++) {
    var k = items[i];
    var title = k.title ? '<div class="k-title">' + escapeHtml(k.title) + '</div>' : '';
    var source = k.source ? '<span class="k-source">' + escapeHtml(k.source) + '</span>' : '';
    var date = k.created_at ? new Date(k.created_at).toLocaleDateString() : '';
    html += '<div class="k-item" id="k-item-' + k.id + '">' +
      title +
      '<div class="k-content">' + escapeHtml(k.content) + '</div>' +
      '<div class="k-meta">' + source + ' <span class="k-date">' + date + '</span></div>' +
      '<div class="k-actions">' +
        '<button class="k-edit" onclick="startEditKnowledge(' + k.id + ')" title="' + t('knowledge.edit') + '">' + t('knowledge.edit') + '</button>' +
        '<button class="k-delete" onclick="deleteKnowledge(' + k.id + ')" title="' + t('knowledge.delete') + '">' + t('knowledge.delete') + '</button>' +
      '</div>' +
      '</div>';
  }
  list.innerHTML = html;

  var totalPages = Math.ceil(total / knowledgePageSize);
  var pagHTML = '';
  if (totalPages > 1) {
    pagHTML += '<span class="k-page-info">' + (knowledgePage + 1) + ' / ' + totalPages + '</span> ';
    if (knowledgePage > 0) {
      pagHTML += '<button onclick="knowledgePage--; loadKnowledge();">Prev</button> ';
    }
    if (knowledgePage < totalPages - 1) {
      pagHTML += '<button onclick="knowledgePage++; loadKnowledge();">Next</button>';
    }
  }
  pag.innerHTML = pagHTML;
}

function startEditKnowledge(id) {
  var item = document.getElementById('k-item-' + id);
  if (!item) return;

  // If already editing, cancel first
  cancelEditKnowledge();

  var titleEl = item.querySelector('.k-title');
  var contentEl = item.querySelector('.k-content');

  // Hide original elements
  if (titleEl) titleEl.style.display = 'none';
  if (contentEl) contentEl.style.display = 'none';

  var currentTitle = titleEl ? titleEl.textContent : '';
  var currentContent = contentEl ? contentEl.textContent : '';

  var form = document.createElement('div');
  form.id = 'k-edit-form';
  form.className = 'k-edit-form';

  // Insert title input at title position, or before content if no title
  var titleInput = document.createElement('input');
  titleInput.type = 'text';
  titleInput.className = 'k-edit-title';
  titleInput.placeholder = 'Title';
  titleInput.value = currentTitle;

  var contentInput = document.createElement('textarea');
  contentInput.className = 'k-edit-content';
  contentInput.rows = 4;
  contentInput.placeholder = 'Content (max 500 chars)';
  contentInput.textContent = currentContent;

  var btns = document.createElement('div');
  btns.className = 'k-edit-btns';
  btns.innerHTML =
    '<button onclick="saveEditKnowledge(' + id + ')">Save</button>' +
    '<button onclick="cancelEditKnowledge()">Cancel</button>';

  form.appendChild(titleInput);
  form.appendChild(contentInput);
  form.appendChild(btns);

  // Insert form where title was, or at top if no title
  if (titleEl) {
    titleEl.parentNode.insertBefore(form, titleEl.nextSibling);
  } else if (contentEl) {
    contentEl.parentNode.insertBefore(form, contentEl);
  } else {
    item.insertBefore(form, item.firstChild);
  }
}

function cancelEditKnowledge() {
  var form = document.getElementById('k-edit-form');
  if (!form) return;

  var item = form.parentNode;
  // Show original elements
  var titleEl = item.querySelector('.k-title');
  var contentEl = item.querySelector('.k-content');
  if (titleEl) titleEl.style.display = '';
  if (contentEl) contentEl.style.display = '';

  form.remove();
}

function saveEditKnowledge(id) {
  var form = document.getElementById('k-edit-form');
  if (!form) return;
  var title = form.querySelector('.k-edit-title').value.trim();
  var content = form.querySelector('.k-edit-content').value.trim();

  if (!title && !content) {
    alert('Title or content is required.');
    return;
  }

  fetch(SERVER_URL + '/api/knowledge/' + id, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ title: title, content: content })
  })
    .then(function(r) { return r.json(); })
    .then(function() {
      var q = document.getElementById('knowledge-search').value.trim();
      if (q) searchKnowledgeUI();
      else loadKnowledge();
    })
    .catch(function(e) { console.error('update knowledge error:', e); });
}

function deleteKnowledge(id) {
  if (!confirm('Delete this knowledge entry?')) return;
  fetch(SERVER_URL + '/api/knowledge/' + id, { method: 'DELETE' })
    .then(function(r) { return r.json(); })
    .then(function() {
      var q = document.getElementById('knowledge-search').value.trim();
      if (q) searchKnowledgeUI();
      else loadKnowledge();
    })
    .catch(function(e) { console.error('delete knowledge error:', e); });
}
