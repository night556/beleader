// state.js — Global state for BeLeader frontend
var SERVER_URL = window.location.origin;

// Core state
var state = {
  name: 'idle',       // idle | thinking | tool_calls | responding | error
};

// Timeline & Stage
var timelineItems = [];   // [{id, type, icon, label, content, status, html, thread_id, tool_call_id, time}]
var currentStage = null;  // {live:true|false, item: timelineItem}
var lastUserItem = null;

// Thread management
var activeThreadId = null;  // null = new chat
var threads = [];           // [{id, title, agent_id, model_id, created_at, updated_at}]
var agents = [];            // [{id, name, desc, system_prompt, tools}]
var activeAgentId = null;
var pendingImages = [];

// UI state
var historyOpen = false;
var settingsOpen = false;
var hasModels = true;
var _contextPcts = {};

// Pagination state
var _loadingOlder = false;
var _noMoreMessages = false;

// Cached agent list for agents panel
var _agentsCache = [];

// Timeline helpers
var _itemSeq = 0;
function newItemId() { return 'ti' + (++_itemSeq) + '_' + Date.now(); }

function pushTimelineItem(item) {
  if (!item.id) item.id = newItemId();
  item.time = item.time || Date.now();
  timelineItems.push(item);
  if (timelineItems.length > 200) timelineItems.shift();
}

function findTimelineItem(id) {
  for (var i = timelineItems.length - 1; i >= 0; i--) {
    if (timelineItems[i].id === id) return timelineItems[i];
  }
  return null;
}

// Check if a thread_id belongs to the current view
function isCurrentThread(tid) {
  if (!tid) return true;
  return tid === activeThreadId;
}
