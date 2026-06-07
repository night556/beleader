# BeLeader

[🇨🇳 中文](./README_zh.md)

**Be the Leader. Let AI do the work.**

BeLeader is an AI agent that works like a real team. You tell it what you want — "build a todo app", "research the latest AI trends", or "organize my desktop files" — and it spins up a Coordinator to plan the work, then spawns multiple Worker agents that read, write, search, browse, and execute in parallel. Each Worker has its own tools and isolated context. You watch the team work in real time.

## How It Works

1. **You give an instruction** — Type a request in the main chat, e.g. "Add Stripe payments to the checkout page"
2. **BeLeader creates a project** — A dedicated Coordinator is assigned to break down the task
3. **Workers execute in parallel** — The Coordinator spawns Workers: one researches the Stripe API, another reads your existing code, a third writes the integration. Workers run concurrently with isolated contexts
4. **You review and intervene** — See everything in real time. Pause a Worker mid-task if you need to redirect it
5. **Done** — Workers are terminated when finished. You keep full conversation history per project

### Emergency Stop

**Stop button** — Clicking stop on a project terminates the Coordinator and all Workers. The request context is cancelled, aborting in-flight LLM calls and preventing further tool execution.

**Tray → Quit** — Immediately terminates the entire application process. Use this when you need to kill everything instantly, even if an LLM request is in progress.

## Architecture

```
You (Leader)
    │
    ▼
┌─────────────────┐
│  Coordinator     │  ← Plans, delegates, reviews. No dev tools.
└────────┬────────┘
    │        │
    ▼        ▼
┌────────┐ ┌────────┐ ┌────────┐
│Worker 1│ │Worker 2│ │Worker N│  ← Each has full dev tools + own context
└────────┘ └────────┘ └────────┘
```

- **Coordinator** — management-only. Reads the project, plans the work, spawns Workers, reviews results. It cannot write code — only Workers can.
- **Workers** — specialized agents with full development tools. Spawned on demand or woken from history, each with isolated context.
- **Desktop Agent** (Rust) — a native binary for mouse/keyboard control, screenshots, window management, and clipboard access.

## Features

### Multi-Agent Collaboration
Coordinator plans, Workers execute. The Coordinator reads STATUS.md to track progress, spawns Workers with specific tasks, intervenes when they go off track, and terminates them when done. Workers run in parallel with isolated contexts — no context pollution between tasks.

### Knowledge Base (Cross-Project Memory)

BeLeader learns from your corrections. When you teach it a reusable lesson — *"No, design the UI first, then build the backend"* or *"Don't over-engineer, always build MVP first"* — it saves the insight. Before starting future work, the Coordinator searches this knowledge base using SQLite FTS5 full-text search. Relevant past lessons are retrieved and applied to the current task, making the AI smarter project after project. Review and manage everything via the **Knowledge** panel (📚) in the top bar.

### Desktop Automation
A native Rust agent takes screenshots, moves and clicks the mouse, types text, scrolls, manages windows, reads and writes the clipboard. Works across Windows, macOS, and Linux. The Coordinator can instruct Workers to "check what's on screen" or "fill in this form."

### Browser Automation
Headless browser support for web scraping, automated testing, and interacting with web apps. Workers can navigate pages, click elements, and extract data.

### Human-in-the-Loop
Intervene at any time — pause a running Worker, give mid-task feedback, then resume. The Coordinator itself can decide you need to review something and request your input before continuing.

### Real-Time Streaming
Everything streams live via SSE: assistant messages, tool calls, tool results. Expand any message to see every file read, every command run, every search executed — with full detail.

### Tauri Desktop App
Native desktop experience with system tray, auto-start, and a bundled backend. A single app that contains the Go backend, the Rust agent, and the web frontend. No Docker, no cloud — runs entirely on your machine.

### Custom Agent Roles
Define agent personas with custom system prompts. Create a "code reviewer" agent, a "test writer" agent, or whatever role your workflow needs. Agents persist across sessions.

### Multi-Project Tabs
Work on multiple projects in parallel — each tab is an isolated session with its own chat history, context, and agent team.

### Speech Output
Optional TTS support — the assistant can speak responses aloud.

### OpenAI-Compatible
Works with any provider that speaks the OpenAI API: OpenAI, Anthropic (via compatible endpoints), local models via Ollama, or self-hosted solutions.

### Agents vs Workers

- **Agent** — a reusable role template, essentially a skill card. You define who the AI is and how it thinks through a system prompt ("You are a senior Rust engineer who prioritizes zero-cost abstractions"). No tools attached — it's purely a behavior preset that shapes the AI's reasoning style, expertise, and output. A well-crafted prompt is itself a powerful tool. Create once, save to your library, spawn as a Worker whenever you need that skillset.
- **Worker** — a running instance of an Agent, spawned by the Coordinator for a specific task. Each Worker gets a clean, isolated context — no crosstalk, no memory pollution between tasks. Execution stops when the task is done, but the Worker and its full conversation history are persisted. You can **wake it up** anytime to continue where it left off — no need to re-explain the context. Or spawn a fresh one if you want a clean slate.

## Examples

### Wake or Spawn — You Decide

**You:** "数据库表结构还是上次 Worker B 改的那套，把它叫醒，让它基于上次的上下文继续加几个字段。别开新的，新 Worker 还得重新读一遍 schema。"

Coordinator wakes Worker B — its full conversation history is still there, it remembers the schema it modified. Picks up right where it left off. If you'd said "spawn a new one" instead, Coordinator would create a fresh Worker with zero context.

### Replace a Polluted Worker

**You:** "Worker A seems stuck — it's been reading that huge file for 10 minutes. I think its context is polluted. Terminate it and spawn a fresh Worker to redo the task."

Coordinator terminates Worker A, spawns Worker B with the same task and a clean context. B finishes in 2 minutes since it's not carrying 3000 lines of legacy code in its memory.

### Course-Correct Mid-Task

**You:** "Worker A got it wrong — I only asked it to rename the function. Why is it touching the imports too? Pause it, tell it to only rename the function and leave the imports alone."

Coordinator intervenes, sends the correction mid-execution. Worker A reads the feedback, adjusts, continues. No restart, no lost work.

### Start a New Project

**You (main chat):** "Create a new project called 'Mini-Program Research'. I want to study WeChat mini-program development — its workflow and best practices."

Main session calls `create_project`, a new tab opens, Coordinator is assigned. You switch to the project tab: "Search the official docs, map out the dev environment and toolchain, then summarize the core concepts." Coordinator spawns Workers, project is underway. Multiple projects can run in parallel — each has its own Coordinator and Worker team.

### Steal Agent Prompts from the Web

**You (main chat):** "Go to this URL, check out how they wrote their Agent prompt, extract it, and save it as a new Agent called 'Security Auditor'."

The main session opens the URL via the browser, scrapes the prompt template, and calls `create_agent` to save it. One instruction, done. Next time you need a security audit, spawn it as a Worker.

### Parallel Audit

**You:** "That PR touched a lot of code. Review it from two angles — security vulnerabilities and performance regressions. Two Workers, one direction each."

Coordinator spawns two Workers simultaneously. Worker A audits for SQL injection, XSS, auth bypasses. Worker B profiles hot paths, N+1 queries. They run in parallel. You read both reports, merge the findings.

## Quick Start

### Prerequisites

- [Go](https://go.dev/dl/) 1.26+
- [Rust](https://rustup.rs/) (for desktop agent and Tauri app)
- [Node.js](https://nodejs.org/) (for Tauri desktop)

### Start

**Windows:**
```powershell
.\make.ps1 dev-backend
```

**macOS / Linux:**
```bash
make dev-backend
```

Open http://localhost:8080 in your browser. The config file and working directories are auto-created on first launch. Go to **Settings** (top-right corner) to add your API key and model.

To launch the Tauri desktop app (requires backend running on :8080):

**Windows:**
```powershell
.\make.ps1 dev-desktop
```

**macOS / Linux:**
```bash
make dev-desktop
```

## License

MIT
