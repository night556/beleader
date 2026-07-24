# IAmHuman

AI chat platform with tool execution. Chat with AI agents that run shell commands, edit files, search the web — everything streamed in real time.

## Architecture

```
Browser (React SPA)
    │  SSE + REST
    ▼
Gateway (Go + Gin)
    │  Agent Loop (LLM calls + tool routing)
    │  SQLite/MySQL/PostgreSQL
    │  SSE broker
    │
    ├── Local tools (web_search, web_fetch, read_status, spawn_worker, management)
    │
    └── Tool Agent (remote) ── Shell, File ops, Search
         (stateless executor, multiple instances per pool)
```

- **Gateway** — The brain: agent loop, LLM calls, persistence, SSE, tool routing, agent/model/MCP/pool management
- **Tool Agent** — The hands: stateless tool executor. Receives tool calls + workspace path, returns results. Multiple instances form a pool for horizontal scaling.
- **Web** — React 19 + TypeScript, Vite build, Nginx serving

## Key Concepts

### Pool
A pool groups tool agents that share the same environment (shell, platform, workspace root, tool definitions). Threads bind to pools. Tool calls within a pool are load-balanced across agents.

- Personal mode: each machine is its own pool (size=1)
- Service mode: multiple containers in one pool (shared volume, horizontal scaling)

### Tool Routing
- **Local tools** (web_search, read_status, spawn_worker, management) — executed in Gateway
- **Remote tools** (exec, read_file, write_file, etc.) — forwarded to a tool agent in the thread's pool

### Worker
spawn_worker creates a child thread that runs asynchronously. Worker results are injected back into the parent thread's message history when complete. Workers can bind to a different pool (e.g., one with browser support).

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

# Terminal 2 — Tool Agent
cd tool-agent
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

Open http://localhost:8080. Add a model in Settings, then start chatting.

### Desktop (Single .exe)

Build a self-contained Windows executable — no Docker, no terminal commands. Double-click to run.

**Prerequisites:** Go 1.21+, Node.js

**Windows (PowerShell):**
```powershell
git clone https://github.com/night556/beleader.git
cd beleader\desktop
.\build.ps1
# → dist\beleader-windows-amd64.exe
```

**macOS / Linux:**
```bash
git clone https://github.com/night556/beleader.git
cd beleader/desktop
./build.sh
# → dist/beleader-<os>-<arch>
```

The .exe bundles everything: embedded web UI, SQLite database, all services in one process. Data is stored in `~/.beleader/`.

## Configuration

### Gateway

| Env | Default | Description |
|-----|---------|-------------|
| `PORT` | 8082 | HTTP server port |
| `DB_DRIVER` | sqlite | Database driver: sqlite, mysql, postgres |
| `DB_PATH` | ~/.beleader/beleader.db | SQLite database path |
| `DB_HOST` | 127.0.0.1 | MySQL/PostgreSQL host |
| `DB_PORT` | 3306/5432 | MySQL/PostgreSQL port |
| `DB_USER` | beleader | Database user |
| `DB_PASSWORD` | | Database password |
| `DB_NAME` | beleader | Database name |
| `GATEWAY_TOKEN` | | Shared secret for tool agent registration |
| `DATA_DIR` | ~/.beleader/runtime | Data directory |

### Tool Agent

| Env | Default | Description |
|-----|---------|-------------|
| `PORT` | 8083 | HTTP server port |
| `GATEWAY_URL` | | Gateway URL for auto-registration |
| `GATEWAY_TOKEN` | | Registration token |
| `POOL` | hostname | Pool name to join |
| `WORKSPACE_ROOT` | ~/.beleader | Workspace root directory |
| `RESTRICT_WORKSPACE` | false | Restrict file operations to workspace |

## API Endpoints

### Gateway (default :8082)

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/chat` | Send message, returns thread_id |
| GET | `/api/sse?thread_id=X` | SSE stream for a thread |
| GET | `/api/threads/:id/events?since_id=N` | Replay events from DB |
| GET/DELETE | `/api/threads` | Thread CRUD |
| GET/POST/PUT/DELETE | `/api/agents` | Agent CRUD |
| GET/POST/PUT/DELETE | `/api/models` | Model CRUD |
| GET/POST/PUT/DELETE | `/api/pools` | Pool CRUD |
| POST | `/api/tool-agents/register` | Tool agent registration |
| POST | `/api/tool-agents/heartbeat` | Tool agent heartbeat |
| GET/DELETE | `/api/tool-agents` | Tool agent management |
| GET/POST/PUT/DELETE | `/api/mcp/servers` | MCP server CRUD |

## License

MIT
