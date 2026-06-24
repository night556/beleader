// i18n.js — Internationalization
// Detects browser language; defaults to English for open-source friendliness.
// HTML: <span data-i18n="key">fallback</span> — textContent is replaced on DOM ready.
// HTML: <button data-i18n-title="key" title="fallback"> — title attr is replaced.
// JS:   t('key') returns the translated string.

var _locale = {};
var _lang = 'en';

(function() {
  var dict = {
    // App
    'app.title':                  { en: 'BeLeader', zh: 'BeLeader' },

    // Top bar
    'tab.new_project':            { en: 'New Project', zh: '新建项目' },

    // Context bar
    'ctx.clear':                  { en: 'Clear', zh: '清空' },

    // Idle state
    'idle.text':                  { en: 'Tell me what you want to do', zh: '告诉我你想做什么' },
    'idle.hint1':                 { en: 'Organize desktop files', zh: '整理桌面文件' },
    'idle.hint2':                 { en: 'Write a Python crawler', zh: '写一个 Python 爬虫' },
    'idle.hint3':                 { en: 'Check today\'s weather', zh: '查今天的天气' },

    // Status bar
    'status.ready':               { en: 'Ready', zh: '就绪' },
    'status.thinking':            { en: 'Thinking…', zh: '思考中…' },
    'status.replying':            { en: 'Replying…', zh: '回复中…' },
    'status.executing':           { en: 'Executing', zh: '执行中' },
    'status.failed':              { en: 'Failed', zh: '失败' },
    'status.tool':                { en: 'Tool', zh: '工具' },
    'status.error':               { en: 'Error', zh: '错误' },
    'status.tool_error':          { en: 'Tool Error', zh: '工具错误' },
    'status.tool_result':         { en: 'Tool Result', zh: '工具结果' },
    'status.tool_error_short':    { en: ' [Error]', zh: ' [错误]' },
    'status.unknown_error':       { en: 'Unknown Error', zh: '未知错误' },
    'status.listening':           { en: 'Listening...', zh: '正在聆听...' },
    'status.ai_thinking':         { en: 'AI thinking...', zh: 'AI 思考中...' },
    'status.idle_activity':       { en: 'Idle', zh: '空闲' },
    'status.replying_activity':   { en: 'Replying', zh: '回复中' },
    'status.calling_tool':        { en: 'Calling tool: ', zh: '调用工具: ' },
    'status.tool_done':           { en: 'Tool done', zh: '工具完成' },
    'status.tool_error_activity': { en: 'Tool error', zh: '工具返回错误' },

    // Input area
    'input.placeholder':          { en: 'Give instructions…', zh: '下指令…' },
    'input.stop_title':           { en: 'Stop', zh: '停止' },
    'input.upload_title':         { en: 'Upload image', zh: '上传图片' },
    'input.voice_title':          { en: 'Voice input', zh: '语音输入' },
    'input.voice_output_title':   { en: 'Voice output', zh: '语音输出' },
    'input.send_title':           { en: 'Send', zh: '发送' },

    // Settings panel
    'settings.button_title':      { en: 'Settings', zh: '设置' },
    'knowledge.button_title':     { en: 'Knowledge', zh: '知识库' },
    'agents.button_title':        { en: 'Agents', zh: 'Agents' },
    'knowledge.title':            { en: 'Knowledge', zh: '知识库' },
    'knowledge.search_placeholder': { en: 'Search knowledge...', zh: '搜索知识...' },

    // Top bar button labels
    'topbar.bookmarks':           { en: 'Bookmarks', zh: '收藏' },
    'topbar.agents':              { en: 'Agents', zh: 'Agents' },
    'topbar.knowledge':           { en: 'Knowledge', zh: '知识库' },
    'topbar.settings':            { en: 'Settings', zh: '设置' },

    // Agents panel
    'agents.title':               { en: 'Agents', zh: 'Agents' },
    'agents.new':                 { en: '+ New', zh: '+ 新建' },
    'agents.search_placeholder':  { en: 'Search agents...', zh: '搜索 Agent...' },
    'agents.name':                { en: 'Name', zh: '名称' },
    'agents.desc':                { en: 'Description', zh: '描述' },
    'agents.content':             { en: 'System Prompt', zh: '系统提示词' },
    'agents.edit':                { en: 'Edit', zh: '编辑' },
    'agents.delete':              { en: 'Delete', zh: '删除' },
    'agents.delete_confirm':      { en: 'Delete agent "$1"? This cannot be undone.', zh: '删除 Agent「$1」？此操作不可撤销。' },
    'agents.empty':               { en: 'No agents yet. Click + New to create one.', zh: '暂无 Agent，点击 + 新建 创建。' },
    'agents.new_title':           { en: 'New Agent', zh: '新建 Agent' },
    'agents.edit_title':          { en: 'Edit Agent', zh: '编辑 Agent' },
    'agents.save':                { en: 'Save', zh: '保存' },
    'agents.cancel':              { en: 'Cancel', zh: '取消' },
    'agents.delete_btn':          { en: 'Delete', zh: '删除' },

    // Modal / clear context
    'modal.cancel':               { en: 'Cancel', zh: '取消' },
    'ctx.clear_title':            { en: 'Clear Context?', zh: '清空当前会话上下文？' },
    'ctx.clear_body':             { en: 'This will erase the conversation history of the current session. This cannot be undone.', zh: '此操作会清除当前会话的对话历史，不可撤销。' },
    'ctx.clear_body_hint':        { en: 'Use this when the LLM returns malformed messages or the context feels polluted. After clearing, the Coordinator can still recover project state from STATUS.md; worker history is not recoverable.', zh: '适用于 LLM 返回错误消息、上下文被污染等异常情况。清空后 Coordinator 仍可从 STATUS.md 恢复项目状态，Worker 历史不可恢复。' },
    'ctx.clear_confirm':          { en: 'Clear', zh: '清空' },

    // createProject modal
    'project.new_title':          { en: 'New Project', zh: '新建项目' },
    'project.name_placeholder':   { en: 'Project name', zh: '项目名称' },
    'project.create':             { en: 'Create', zh: '创建' },
    'settings.title':             { en: 'Settings', zh: '设置' },
    'settings.general':           { en: 'General', zh: '通用' },
    'settings.max_hc':            { en: 'Max Concurrency (HC)', zh: '最大并发 (HC)' },
    'settings.context_threshold': { en: 'Context Threshold (%)', zh: '上下文阈值 (%)' },
    'settings.headless':          { en: 'Headless Mode', zh: '无头模式' },
    'settings.voice':             { en: 'Voice', zh: '语音' },
    'settings.stt_lang':          { en: 'STT Language', zh: 'STT 语言' },
    'settings.tts_rate':          { en: 'TTS Speed', zh: 'TTS 语速' },
    'settings.tts_pitch':         { en: 'TTS Pitch', zh: 'TTS 音调' },
    'settings.tts_voice':         { en: 'TTS Voice', zh: 'TTS 语音' },
    'settings.speak_enabled':     { en: 'Speech Output', zh: '语音输出' },
    'settings.models':            { en: 'Models', zh: '模型' },
    'settings.add_model':         { en: '+ Add Model', zh: '+ 添加模型' },
    'settings.active_model':      { en: 'Active Model', zh: '活跃模型' },
    'settings.save':              { en: 'Save Settings', zh: '保存设置' },

    // Timeline
    'timeline.ai_reply':          { en: 'AI Reply', zh: 'AI 回复' },
    'timeline.back_to_project':   { en: '✕ Back to project', zh: '✕ 返回项目' },
    'timeline.viewing':           { en: 'Viewing: ', zh: '查看: ' },
    'timeline.no_model_title':    { en: 'No AI Model Configured', zh: '尚未配置 AI 模型' },
    'timeline.no_model_desc':     { en: 'Configure an LLM model to start chatting', zh: '需要配置一个 LLM 模型才能开始对话' },
    'timeline.no_model_btn':      { en: '⚙ Configure Model in Settings', zh: '⚙ 前往设置配置模型' },
    'timeline.no_models_setup_title': { en: 'No Models Configured', zh: '尚未配置模型' },
    'timeline.no_models_setup_hint':  { en: 'Add at least one LLM model to get started', zh: '请添加至少一个 LLM 模型以开始使用' },
    'timeline.no_models_setup_btn':   { en: '+ Configure Model', zh: '+ 配置模型' },

    // Toast / notifications
    'toast.context_cleared':      { en: 'Context cleared', zh: '上下文已清空' },
    'toast.clear_failed':         { en: 'Clear failed: ', zh: '清空失败: ' },

    // Errors
    'error.cannot_send':          { en: 'Cannot Send', zh: '无法发送' },
    'error.no_model_msg':         { en: 'No AI model configured. Open Settings (⚙) to add one.', zh: '尚未配置 AI 模型。请点击右上角 ⚙ 进入设置，添加一个 LLM 模型后再试。' },
    'error.missing_model':        { en: 'Missing Model Config', zh: '缺少模型配置' },
    'error.cannot_delete_active': { en: 'Cannot delete active model. Switch to another first.', zh: '不能删除当前激活的模型，请先切换到其他模型' },
    'error.at_least_one_model':   { en: 'Please configure at least one model.', zh: '请至少配置一个模型' },

    // Model card
    'model.new':                  { en: 'New Model', zh: '新建模型' },
    'model.id_label':             { en: 'ID', zh: '标识' },
    'model.id_placeholder':       { en: 'e.g. gpt-4o', zh: '例如 gpt-4o' },
    'model.params_label':         { en: 'Parameters', zh: '参数' },

    // Project
    'project.name_prompt':        { en: 'Project Name:', zh: '项目名称:' },
  };

  // Detect language
  var navLang = (navigator.language || '').toLowerCase();
  _lang = navLang.indexOf('zh') === 0 ? 'zh' : 'en';

  // Build locale dict
  for (var k in dict) {
    if (dict.hasOwnProperty(k)) {
      _locale[k] = dict[k][_lang] || dict[k].en;
    }
  }
})();

function t(key, vars) {
  var s = _locale[key];
  if (s === undefined) return key;
  if (vars) {
    for (var vk in vars) {
      if (vars.hasOwnProperty(vk)) {
        s = s.replace('{' + vk + '}', vars[vk]);
      }
    }
  }
  return s;
}

// Apply data-i18n attributes on DOM ready
function applyI18n() {
  // textContent
  var els = document.querySelectorAll('[data-i18n]');
  for (var i = 0; i < els.length; i++) {
    var key = els[i].getAttribute('data-i18n');
    if (key && _locale[key]) {
      els[i].textContent = _locale[key];
    }
  }
  // title attributes
  var titles = document.querySelectorAll('[data-i18n-title]');
  for (var j = 0; j < titles.length; j++) {
    var tkey = titles[j].getAttribute('data-i18n-title');
    if (tkey && _locale[tkey]) {
      titles[j].setAttribute('title', _locale[tkey]);
    }
  }
  // placeholder attributes
  var phs = document.querySelectorAll('[data-i18n-placeholder]');
  for (var k = 0; k < phs.length; k++) {
    var pkey = phs[k].getAttribute('data-i18n-placeholder');
    if (pkey && _locale[pkey]) {
      phs[k].placeholder = _locale[pkey];
    }
  }
}

document.addEventListener('DOMContentLoaded', applyI18n);
