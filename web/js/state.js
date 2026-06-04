// state.js — Global state for IAmHuman frontend
var SERVER_URL = window.location.origin;

// Core state
var state = {
  name: 'idle',       // idle | thinking | tool_calls | responding | speaking | error
};

// Timeline & Stage
var timelineItems = [];   // [{id, type, icon, label, content, status, html, session_id, tool_call_id, time}]
var currentStage = null;  // {live:true|false, item: timelineItem}
var lastUserItem = null;  // track last user message for auto-collapse

// Session management
var activeSessionId = 'main';
var currentView = 'home';    // 'home' | project ref_id
var sessions = [{ id:'main', ref_id:null, title:'main', status:'idle', agents:[], session_id:'main' }];
var pendingImages = [];

// UI state
var historyOpen = false;
var settingsOpen = false;
var speakEnabled = true;
var hasModels = true;

// Agent activity tracking
var _agentActivities = {};

// Per-session context usage tracking
var _contextPcts = {};

// Agent drill-down filter (worker session_id, or null)
var _agentFilter = null;

function getAgentBySession(sid) {
  for (var i = 0; i < sessions.length; i++) {
    if (sessions[i].session_id === sid) return { session: sessions[i], agent: null };
    var agents = sessions[i].agents;
    if (agents) {
      for (var j = 0; j < agents.length; j++) {
        if (agents[j].session_id === sid) return { session: sessions[i], agent: agents[j] };
      }
    }
  }
  return null;
}

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
