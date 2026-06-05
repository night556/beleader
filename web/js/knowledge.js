// knowledge.js — Knowledge base panel

var knowledgePage = 0;
var knowledgePageSize = 20;

function toggleKnowledge() {
  var panel = document.getElementById('knowledge-panel');
  var isOpen = panel.classList.contains('open');
  if (isOpen) {
    panel.classList.remove('open');
  } else {
    // Close settings if open
    var sp = document.getElementById('settings-panel');
    if (sp) sp.classList.remove('open');
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
    var tags = k.tags ? k.tags.split(',').map(function(t) { return '<span class="k-tag">' + escapeHtml(t.trim()) + '</span>'; }).join('') : '';
    var source = k.source ? '<span class="k-source">' + escapeHtml(k.source) + '</span>' : '';
    var date = k.created_at ? new Date(k.created_at).toLocaleDateString() : '';
    html += '<div class="k-item">' +
      '<div class="k-content">' + escapeHtml(k.content) + '</div>' +
      '<div class="k-meta">' + tags + ' ' + source + ' <span class="k-date">' + date + '</span></div>' +
      '<button class="k-delete" onclick="deleteKnowledge(' + k.id + ')" title="Delete">✕</button>' +
      '</div>';
  }
  list.innerHTML = html;

  // Pagination
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
