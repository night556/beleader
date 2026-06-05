# BeLeader

[🇺🇸 English](./README.md)

**Be the Leader. Let AI do the work.**

BeLeader 是一个 AI 编码助手，像真正的开发团队一样工作。告诉它你想要什么——「帮我做一个 Todo 应用」或「修复登录 BUG」——它会启动 Coordinator 规划任务，然后并行创建多个 Worker 读取代码、搜索、浏览网页、执行命令。每个 Worker 有独立的工具和上下文，你实时看着整个团队协作。

## 工作流程

1. **你下指令** — 在主聊天框输入需求，如「给支付页面接入 Stripe」
2. **BeLeader 创建项目** — 分配专属 Coordinator，拆解任务
3. **Worker 并行执行** — Coordinator 创建多个 Worker：一个查 Stripe API 文档，一个读现有代码，一个写集成代码。多个 Worker 同时进行，上下文隔离
4. **你随时介入** — 实时查看每个 Worker 的进展。中途发现不对，暂停纠正后继续
5. **完成** — Worker 自动回收。每个项目保留完整对话历史

### 紧急停止

**停止按钮** — 点击项目停止按钮，终止 Coordinator 和所有 Worker。通过 context 取消中断正在进行的 LLM 调用，并阻止后续工具执行。

**托盘 → 退出** — 直接终止整个应用进程。当你需要立刻中断一切，包括正在进行的 LLM 请求时，这是最快的方式。

## 架构

```
你 (Leader)
    │
    ▼
┌─────────────────┐
│  Coordinator     │  ← 只负责规划、分配、审查，不能写代码
└────────┬────────┘
    │        │
    ▼        ▼
┌────────┐ ┌────────┐ ┌────────┐
│Worker 1│ │Worker 2│ │Worker N│  ← 拥有完整开发工具 + 独立上下文
└────────┘ └────────┘ └────────┘
```

- **Coordinator** — 纯管理角色。读取项目、制定计划、分配 Worker、审查结果。它不能写代码 —— 只有 Worker 能执行。
- **Workers** — 拥有完整开发工具的专业 Agent。按需创建或唤醒，独立上下文互不干扰。
- **Desktop Agent**（Rust）— 原生二进制程序，负责鼠标键盘控制、截图、窗口管理、剪贴板读写。

## 特性

### 多 Agent 协作
Coordinator 制定计划，Worker 执行任务。Coordinator 通过 STATUS.md 追踪进度，按需创建 Worker，在 Worker 跑偏时介入纠正，完成后自动回收。Worker 并行运行，上下文隔离 —— 不会互相污染。

### 知识库（跨项目记忆）

BeLeader 会从你的纠正中学习。当你教它可复用的经验——*「不对，应该先设计 UI 再写后端」* 或 *「别过度设计，先做 MVP」*——它会保存这条心得。之后处理新任务时，Coordinator 会通过 SQLite FTS5 全文检索搜索知识库，找到相关的过往经验，应用到当前任务中。越用越聪明，项目之间经验互通。你可以随时通过顶部 📚 知识库面板查看和管理所有知识条目。
### 桌面自动化
原生 Rust Agent 支持截图、鼠标移动点击、键盘输入、滚动、窗口管理、剪贴板读写。跨平台支持 Windows、macOS、Linux。Coordinator 可以指挥 Worker「看看屏幕上显示了什么」或「填写这个表单」。

### 浏览器自动化
支持无头浏览器，用于网页抓取、自动化测试、与 Web 应用交互。Worker 可以导航页面、点击元素、提取数据。

### 人在回路
随时介入 —— 暂停正在运行的 Worker，中途给出反馈，然后继续执行。Coordinator 也可以主动判断需要你审查某些内容，暂停等你确认。

### 实时流式推送
所有内容通过 SSE 实时推送：助手消息、工具调用、执行结果。展开任意消息即可查看完整细节 —— 读了什么文件、运行了什么命令、搜索结果 —— 一切透明可见。

### Tauri 桌面应用
原生桌面体验，支持系统托盘、开机自启、后端内嵌。一个单独的应用包含 Go 后端、Rust Agent、Web 前端。无需 Docker，无需云服务 —— 全部在本地运行。

### 自定义 Agent 角色
通过自定义系统提示词定义 Agent 角色。创建一个「代码审查」Agent、「测试编写」Agent，或者任何你需要的角色。Agent 配置跨会话持久化。

### 多项目页签
同时处理多个项目 —— 每个页签是独立的会话，拥有自己的聊天记录、上下文和 Agent 团队。

### 语音输出
可选 TTS 支持 —— 助手可以直接朗读回复。

### OpenAI 兼容
支持所有兼容 OpenAI 协议的 API：OpenAI、Anthropic（通过兼容端点）、Ollama 本地模型、自部署方案均可使用。

### Agent 与 Worker

- **Agent** — 可复用的角色模板，本质上是一张「技能卡」。你通过系统提示词定义它的身份和专长（「你是一个资深 Rust 工程师，追求零开销抽象」）。Agent 本身不带工具——它纯粹是一个行为预设，塑造 AI 的推理风格、专业领域和输出方式。一段精心设计的提示词本身就是强大的工具。一次创建，存入库中，需要时 spawn 成 Worker 即可。
- **Worker** — Agent 的运行实例，由 Coordinator 为具体任务创建。每个 Worker 拥有干净的独立上下文 —— 任务之间不会串扰，不会记忆污染。任务完成即停止执行，但 Worker 和完整对话历史会持久保留。你可以随时**唤醒**它继续之前的工作 —— 不需要重新解释背景。也可以创建全新的 Worker，从零开始。

## 使用示例

### 唤醒还是新建，你来定

**你：**「数据库表结构还是上次 Worker B 改的那套，把它叫醒，让它基于上次的上下文继续加几个字段。别开新的，新 Worker 还得重新读一遍 schema。」

Coordinator 唤醒 Worker B —— 它的完整对话历史还在，记得自己改过的 schema，直接接着干。如果你说「开个新的」，Coordinator 就会创建一个零上下文的 Worker 从零开始。

### 替换被污染的 Worker

**你：**「Worker A 好像卡住了，读了那个大文件快十分钟了。我觉得它上下文已经被污染了，停掉它，建一个新的 Worker 重做这件事。」

Coordinator 终止 Worker A，用同样的任务创建 Worker B，干干净净的上下文。B 不再背着 3000 行遗留代码的记忆，两分钟搞定。

### 中途纠正方向

**你：**「Worker A 搞错了——我只让它重命名函数名，它怎么连 import 都动了。暂停它，告诉它只改函数名，别碰 import。」

Coordinator 介入，把纠正指令发给正在执行的 Worker。Worker A 收到反馈，调整方向继续。不用重启，不丢进度。

### 创建新项目开始研究

**你（主会话）：**「帮我新建一个项目，叫『小程序调研』，我想研究一下微信小程序的开发流程和最佳实践。」

主会话调用 `create_project`，新页签自动打开，Coordinator 就位。你切换到项目页签：「先搜索微信小程序的官方文档，梳理开发环境和工具链，然后把核心概念整理成一份概要。」Coordinator 创建 Worker 开始干活。多个项目可以并行进行——每个项目有自己独立的 Coordinator 和 Worker 团队。

### 从网页「偷」Agent 提示词

**你（主会话）：**「去这个链接，看看他们的 Agent 提示词是怎么写的，把提示词提取出来，存成 Agent 库里的一个新 Agent，叫『安全审计专家』。」

主会话直接打开 URL，抓取提示词，调用 `create_agent` 存入库中。一句话搞定。下次需要安全审计，spawn 成 Worker 就能用。

### 并行审查

**你：**「刚才的 PR 改了不少，从安全漏洞和性能退化两个角度审查一下，两个 Worker 各负责一个方向。」

Coordinator 同时创建两个 Worker。Worker A 审计 SQL 注入、XSS、权限绕过。Worker B 分析热点路径、N+1 查询。并行执行，两份报告一起收。

## 快速开始

### 环境要求

- [Go](https://go.dev/dl/) 1.26+
- [Rust](https://rustup.rs/)（桌面 Agent 和 Tauri 应用需要）
- [Node.js](https://nodejs.org/)（Tauri 桌面版需要）

### 启动

**Windows：**
```powershell
.\make.ps1 dev-backend
```

**macOS / Linux：**
```bash
make dev-backend
```

浏览器打开 http://localhost:8080。首次启动会自动创建配置文件和项目目录。在页面右上角 **Settings** 中添加 API Key 和模型即可使用。

启动 Tauri 桌面版（需要后端在 :8080 运行）：

**Windows：**
```powershell
.\make.ps1 dev-desktop
```

**macOS / Linux：**
```bash
make dev-desktop
```

## 协议

MIT
