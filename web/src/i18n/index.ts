type LocaleDict = Record<string, { en: string; zh: string }>;

const dict: LocaleDict = {
  'app.title':                  { en: 'BeLeader', zh: 'BeLeader' },
  'ctx.clear':                  { en: 'Clear', zh: '清空' },
  'idle.text':                  { en: 'Tell me what you want to do', zh: '告诉我你想做什么' },
  'idle.hint1':                 { en: 'Organize desktop files', zh: '整理桌面文件' },
  'idle.hint2':                 { en: 'Write a Python crawler', zh: '写一个 Python 爬虫' },
  'idle.hint3':                 { en: 'Check today\'s weather', zh: '查今天的天气' },
  'status.ready':               { en: 'Ready', zh: '就绪' },
  'status.thinking':            { en: 'Thinking…', zh: '思考中…' },
  'status.replying':            { en: 'Replying…', zh: '回复中…' },
  'status.executing':           { en: 'Executing', zh: '执行中' },
  'status.failed':              { en: 'Failed', zh: '失败' },
  'input.placeholder':          { en: 'Give instructions…', zh: '下指令…' },
  'input.stop_title':           { en: 'Stop', zh: '停止' },
  'input.upload_title':         { en: 'Upload image', zh: '上传图片' },
  'input.send_title':           { en: 'Send', zh: '发送' },
  'topbar.bookmarks':           { en: 'Bookmarks', zh: '收藏' },
  'topbar.tools':               { en: 'Tools', zh: '工具' },
  'topbar.agents':              { en: 'Agents', zh: 'Agents' },
  'topbar.mcp':                 { en: 'MCP', zh: 'MCP' },
  'topbar.knowledge':           { en: 'Knowledge', zh: '知识库' },
  'topbar.settings':            { en: 'Settings', zh: '设置' },
  'agents.title':               { en: 'Agents', zh: 'Agents' },
  'agents.new':                 { en: '+ New', zh: '+ 新建' },
  'agents.search_placeholder':  { en: 'Search agents...', zh: '搜索 Agent...' },
  'agents.name':                { en: 'Name', zh: '名称' },
  'agents.desc':                { en: 'Description', zh: '描述' },
  'agents.system_prompt':       { en: 'System Prompt', zh: '系统提示词' },
  'agents.edit':                { en: 'Edit', zh: '编辑' },
  'agents.delete':              { en: 'Delete', zh: '删除' },
  'agents.delete_confirm':      { en: 'Delete agent "$1"?', zh: '删除 Agent「$1」？' },
  'agents.empty':               { en: 'No agents yet.', zh: '暂无 Agent。' },
  'agents.new_title':           { en: 'New Agent', zh: '新建 Agent' },
  'agents.edit_title':          { en: 'Edit Agent', zh: '编辑 Agent' },
  'agents.save':                { en: 'Save', zh: '保存' },
  'agents.delete_btn':          { en: 'Delete', zh: '删除' },
  'agents.tools':               { en: 'Tools', zh: '工具' },
  'agents.tools_search':        { en: 'Search tools...', zh: '搜索工具...' },
  'agents.no_tools':            { en: 'No tools selected.', zh: '未选择工具。' },
  'mcp.title':                  { en: 'MCP Servers', zh: 'MCP Servers' },
  'mcp.search_placeholder':     { en: 'Search MCP...', zh: '搜索 MCP...' },
  'mcp.new':                    { en: '+ New', zh: '+ 新建' },
  'mcp.connect':                { en: 'Connect', zh: '连接' },
  'mcp.disconnect':             { en: 'Disconnect', zh: '断开' },
  'mcp.test':                   { en: 'Test', zh: '测试' },
  'mcp.edit':                   { en: 'Edit', zh: '编辑' },
  'mcp.delete':                 { en: 'Delete', zh: '删除' },
  'modal.cancel':               { en: 'Cancel', zh: '取消' },
  'settings.title':             { en: 'Settings', zh: '设置' },
  'settings.models':            { en: 'Models', zh: '模型' },
  'settings.add_model':         { en: '+ Add Model', zh: '+ 添加模型' },
  'settings.save':              { en: 'Save Settings', zh: '保存设置' },
  'timeline.ai_reply':          { en: 'AI Reply', zh: 'AI 回复' },
  'timeline.no_model_title':    { en: 'No AI Model Configured', zh: '尚未配置 AI 模型' },
  'timeline.no_model_desc':     { en: 'Configure an LLM model to start chatting', zh: '需要配置一个 LLM 模型才能开始对话' },
  'timeline.no_models_setup_title': { en: 'No Models Configured', zh: '尚未配置模型' },
  'timeline.no_models_setup_hint':  { en: 'Add at least one LLM model to get started', zh: '请添加至少一个 LLM 模型以开始使用' },
  'timeline.no_models_setup_btn':   { en: '+ Configure Model', zh: '+ 配置模型' },
  'error.cannot_delete_active': { en: 'Cannot delete active model.', zh: '不能删除当前活跃的模型。' },
  'error.at_least_one_model':   { en: 'Please configure at least one model.', zh: '请至少配置一个模型' },
  'model.new':                  { en: 'New Model', zh: '新建模型' },
  'model.id_label':             { en: 'ID', zh: '标识' },
  'model.id_placeholder':       { en: 'e.g. gpt-4o', zh: '例如 gpt-4o' },
  'model.base_url':             { en: 'Base URL', zh: 'Base URL' },
  'model.api_key':              { en: 'API Key', zh: 'API Key' },
  'model.model_select':         { en: 'Model', zh: 'Model' },
  'model.base_url_placeholder': { en: 'https://api.openai.com/v1', zh: 'https://api.openai.com/v1' },
  'model.model_placeholder':    { en: 'Enter model name...', zh: '输入模型名称...' },
  'conn.disconnected_banner':   { en: 'Connection lost, reconnecting…', zh: '连接已断开，正在重连…' },
  'conn.reconnect':             { en: 'Reconnect Now', zh: '立即重连' },
  'sidebar.toggle':             { en: 'Toggle Sidebar', zh: '菜单' },
  'sidebar.home':               { en: 'Home', zh: '首页' },
  'sidebar.new_thread':         { en: 'New Thread', zh: '新建会话' },
  'ctx.model_title':            { en: 'Current model', zh: '当前模型' },
  'ctx.tokens_title':           { en: 'Total token usage', zh: '累计 token 使用量' },
  'bookmark.title':             { en: '★ Bookmarks', zh: '★ 收藏消息' },
  'bookmark.empty':             { en: 'No bookmarks yet', zh: '暂无收藏' },
  'bookmark.home_hint':         { en: 'Open a thread to bookmark messages', zh: '请在会话中收藏消息' },
  'knowledge.title':            { en: 'Knowledge', zh: '知识库' },
  'knowledge.search_placeholder': { en: 'Search knowledge...', zh: '搜索知识...' },
  'knowledge.edit':             { en: 'Edit', zh: '编辑' },
  'knowledge.delete':           { en: 'Delete', zh: '删除' },
  'tools.title':                { en: 'Tools', zh: '工具列表' },
  'card.rendered':              { en: 'Rendered', zh: '渲染' },
  'card.source':                { en: 'Source', zh: '源码' },
};

const navLang = (navigator.language || '').toLowerCase();
const lang = navLang.startsWith('zh') ? 'zh' : 'en';

const locale: Record<string, string> = {};
for (const k in dict) {
  locale[k] = dict[k][lang] || dict[k].en;
}

export function t(key: string, vars?: Record<string, string | number>): string {
  let s = locale[key];
  if (s === undefined) return key;
  if (vars) {
    for (const vk in vars) {
      s = s.replace(`$${vk}`, String(vars[vk]));
    }
  }
  return s;
}
