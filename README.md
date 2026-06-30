# BeLeader

[🇨🇳 中文](./README_zh.md)

**Be the Leader. Let AI do the work.**

BeLeader is an AI collaboration platform. You chat, it brings the right AI workers to the task — for development, research, automation, or anything you need done.

## What makes it different

### You control everything through chat

Config files are managed for you; you never need to touch them.

**Control projects.** Say what you want done — "build a payment page," "research competitor pricing" — and Main creates a project. The Coordinator plans the work, spawns Workers, and tracks progress. You can ask about status, change direction, or review results at any time.

**Configure the platform.** This is where BeLeader goes beyond a task runner. Through conversation you can:

- Find a code review skill online, install it, and turn it into an Agent template — all in one flow: web search → read the docs → install dependencies → create and configure the agent.
- Discover an MCP server for GitHub, read its documentation, install it, connect it — from browser to running tool without leaving the chat.
- Build a knowledge base: when you teach a reusable pattern or fix, save it. Before every new task, relevant knowledge is automatically searched.

### Optimized for token caching

BeLeader is designed from the ground up to keep prompt caches warm. Four design decisions drive this:

**1. Persistent Worker context.** Workers keep their full conversation history. Wake a Worker and the LLM provider hits the prompt cache for the entire unchanged prefix — faster and cheaper.

**2. Tool-set-based Agent templates.** An Agent is defined by its tool set and system prompt — a stable, named entity, not per-task config. Because the tool set doesn't change, the system prompt never shifts, and the cache never invalidates. Add new tools to the platform; existing Workers stay warm.

**3. MCP servers auto-become Agent templates.** Connect an MCP server — its tools are discovered, registered, and the server becomes an Agent template automatically. Same tool-set model, same caching benefits.

**4. Skills are agents too.** A skill is an Agent template with a custom prompt and tool set — same mechanism, same caching behavior. Define once, use everywhere.

### Intervene between iterations, not after the fact

When an AI agent loops — LLM call, tool execution, LLM call, tool execution — a single task can span dozens or hundreds of iterations. Most tools give you two options: watch, or cancel. Cancel means losing everything since the last checkpoint.

BeLeader lets you intervene at the iteration boundary: after the current tool results come back, before the next LLM call goes out. Your feedback is injected into the context, and the LLM sees it on its very next request. No iterations are wasted, no progress is lost. The Worker simply continues — with your correction in its conversation.

### Emergency stop for the entire project

When something goes wrong — a Worker heading in the wrong direction, a tool call about to cause damage — you can stop the entire project. All LLM calls across all Workers halt at the next iteration boundary. The situation is contained.

But stopping doesn't mean destroying. Every Worker keeps its full context. You can correct course and resume, or come back later — the token cache is still there.

## Architecture

```
You (chat)
  │
  ▼
Main (platform controller)     ← manages projects, agents, knowledge, MCP
  │
  ▼
Project
  ├── Coordinator              ← plans, delegates, reviews
  │     │  spawn_worker
  │     ▼
  ├── Worker A                 ← persistent context
  ├── Worker B
  └── Worker C
        ↑
        │  tools from Agent template
  ┌─────┴──────────────────────────┐
  │  Agent templates               │
  │  • general (file/exec/web)     │
  │  • browser (web automation)    │
  │  • desktop (GUI automation)    │
  │  • custom agents you create    │
  │  • MCP servers (external tools)│
  └────────────────────────────────┘
```

## Quick Start

### Prerequisites

- [Go](https://go.dev/dl/) 1.21+
- [Rust](https://rustup.rs/) (desktop agent, optional)
- [Node.js](https://nodejs.org/) (Tauri app, optional)

### Start

```bash
make dev-backend     # macOS / Linux
# or
.\make.ps1 dev-backend   # Windows
```

Open http://localhost:8080. Add your API key in Settings.

## License

MIT
