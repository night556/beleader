# IAmHuman

AI chat platform with tool execution. Chat with AI agents that run shell commands, edit files, search the web — everything streamed in real time.

## Architecture

```
Browser (React SPA)
    │  SSE + REST
    ▼
Gateway (Go + Gin)
    │  REST + SSE
    ▼
Runtime (Go) ── LLM API ── OpenAI / DeepSeek / Gemini
    │
    ├── Shell (bash/powershell)
    ├── File read/write/edit
    ├── Web search / fetch
    └── MCP servers
```

- **Gateway** — REST API, SQLite persistence, SSE broker, Runtime pool, agent/model/MCP management
- **Runtime** — stateless agent loop: receives turns, calls LLM, executes tools, streams events back
- **Web** — React 19 + TypeScript, Vite build, Nginx serving

## Quick Start

### Prerequisites

- Go 1.21+
- Node.js 22+

### Development

```bash
# Terminal 1 — Gateway
cd gateway
cp .env.example .env    # edit with your API keys
go run .

# Terminal 2 — Runtime
cd runtime
cp .env.example .env
go run .

# Terminal 3 — Web
cd web
npm install
npm run dev
```

Open http://localhost:5173. Add a model in Settings, then start chatting.

### Docker

```bash
docker compose up -d
```

## Key Features

### Stateless Runtime
Runtime instances are interchangeable. Gateway passes `thread_dir` and `workspace_dir` per request. All state lives on a shared volume. Kill one Runtime, the next turn routes to another.

### Reasoning Effort Control
Per-turn reasoning effort toggle (off / low / medium / high / max). Supported on OpenAI (o-series), DeepSeek (R1), and Gemini.

### MCP Support
Connect MCP servers via stdio or HTTP. Tools are auto-discovered and namespaced. Built-in tools cover shell execution, file operations, web search, and worker spawning.

### Background Commands
Long-running shell commands run in background. Results are injected into the next turn automatically.

### Context Compression
When the conversation exceeds the context threshold, older messages are auto-summarized. Recent turns stay verbatim. Token cache stays warm.

### Chat Insertion
Send a new message while the AI is responding — the current loop is cancelled and the new message takes priority. No page refresh, no lost state.

### Workspace Restriction
Limit file and command access to a specific directory. Configured per Runtime at registration.

## Configuration

| Service | Env File | Key Variables |
|---------|----------|---------------|
| Gateway | `gateway/.env.example` | `PORT`, `DATA_DIR`, `GATEWAY_TOKEN`, `LLM_BASE_URL`, `LLM_API_KEY` |
| Runtime | `runtime/.env.example` | `PORT`, `DATA_DIR`, `GATEWAY_URL`, `GATEWAY_TOKEN` |
| Web | `web/.env.example` | `VITE_API_URL` |

## Endpoints

### Gateway (default :8082)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/chat` | Send message, returns SSE stream |
| GET/POST/DELETE | `/api/threads` | Thread CRUD |
| GET/POST/PUT/DELETE | `/api/agents` | Agent CRUD |
| GET/POST/PUT/DELETE | `/api/models` | Model CRUD |
| GET/POST/PUT/DELETE | `/api/mcp` | MCP server CRUD |
| GET/DELETE | `/api/runtimes` | Runtime management |
| GET | `/api/tools` | List available tools |
| POST | `/api/runtimes/register` | Runtime registration |
| POST | `/api/runtimes/heartbeat` | Runtime heartbeat |

### Runtime (default :8083)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/v1/threads` | Create thread |
| POST | `/v1/threads/{id}/turns` | Execute turn, returns SSE stream |
| GET | `/v1/threads/{id}/events` | Poll events (SSE) |
| GET | `/v1/tools` | Tool definitions |
| GET | `/v1/health` | Health check |

## SSE Event Model

Events follow the CodeWhale format:

| Event | Description |
|-------|-------------|
| `turn.started` | New turn begins |
| `item.started` | Item begins (user_message / agent_message / tool_call / command_execution) |
| `item.delta` | Streaming content delta |
| `item.completed` | Item finished |
| `item.failed` | Item errored |
| `turn.completed` | Turn finished (completed / interrupted) |

Every event carries `thread_id` and `turn_id`. The frontend uses `turn_id` to filter stale events when a chat insertion cancels a running turn.

## License

MIT
