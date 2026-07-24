# BeLeader

[🇺🇸 English](./README.md)

**Be the Leader. Let AI do the work.**

BeLeader 是一个 AI 协作平台。你跟它对话，它调度合适的 AI Worker 来完成任务——开发、调研、自动化，做什么都行。

> 内置的 Agent 提示词均为英文。LLM 对英文指令的遵循能力更强，尤其在结构化、系统级 prompt 上表现更稳定。UI 和对话本身支持中文。

## 与同类工具的不同

### 一切通过聊天控制

配置文件由系统管理，你不需要手动编辑。

**控制项目。** 说出你想要什么——「做个支付页面」「调研竞品定价」——Main 创建项目。Coordinator 规划工作、创建 Worker、追踪进度。你可以随时询问进展、调整方向、审查结果。

**配置平台。** 这是 BeLeader 不止于任务执行的地方。通过对话你可以：

- 在网上找到一个代码审查 skill，安装并变成 Agent 模板——全程在对话中完成：搜索 → 读文档 → 安装依赖 → 创建并配置 agent。
- 发现一个 GitHub MCP server，读它的文档，安装，接入——从网页到可用工具，不离开聊天。
- 构建知识库：当你教给平台一个可复用的模式或经验，保存它。每次新任务启动前，相关知识自动检索。

### 针对 Token 缓存优化

BeLeader 从设计层面围绕缓存优化。四个设计决策驱动：

**1. Worker 上下文持久化。** Worker 保留完整对话历史。唤醒 Worker 时，LLM provider 对整个未变的前缀直接命中 Prompt 缓存——更快更省钱。

**2. 以工具集合为主体的 Agent 模板。** Agent 由工具集和系统提示词定义——稳定、有名字的实体，不是每次任务的临时配置。工具集不变，系统提示词不变，缓存不失效。给平台新增工具，已有 Worker 的缓存不受影响。

**3. MCP 服务器自动变成 Agent 模板。** 接入 MCP 服务器——工具自动发现、注册，服务器自动成为 Agent 模板。同一套工具集合模型，同样的缓存收益。

**4. Skill 也是 Agent。** Skill 就是带自定义提示词和工具集的 Agent 模板——同一套机制，同样的缓存行为。定义一次，随处使用。

### 在迭代间隙介入，而非事后

AI agent 运行一次任务可能循环几十上百次——LLM 调用、工具执行、LLM 调用、工具执行，反复交替。大多数工具只给你两个选择：看着，或取消。取消意味着丢掉所有进度。

BeLeader 让你在迭代边界介入：当前工具结果返回后，下一次 LLM 调用发出前。你的反馈注入上下文，LLM 在下一次请求中直接看到。没有浪费的迭代，没有丢失的进度。Worker 继续运行——你的修正已经在对话里。

### 紧急停止能力

当事情不对——Worker 方向跑偏、某个工具调用即将造成破坏——你可以停止整个项目。所有 Worker 的所有 LLM 调用在迭代边界停住。局面被控制住。

但停止不是销毁。每个 Worker 保留完整上下文。你可以修正方向后继续，或者稍后再回来——token 缓存还在。

## 架构

```
你（聊天）
  │
  ▼
Main（平台控制器）         ← 管理项目、Agent、知识库、MCP
  │
  ▼
Project
  ├── Coordinator          ← 规划、分配、审查
  │     │  spawn_worker
  │     ▼
  ├── Worker A             ← 持久上下文
  ├── Worker B
  └── Worker C
        ↑
        │  工具来自 Agent 模板
  ┌─────┴──────────────────────────┐
  │  Agent 模板                     │
  │  • general（文件/执行/网页）     │
  │  • browser（浏览器自动化）       │
  │  • desktop（桌面自动化）         │
  │  • 自定义 Agent                 │
  │  • MCP 服务器（外部工具）        │
  └─────────────────────────────────┘
```

## 快速开始

### 环境要求

- [Go](https://go.dev/dl/) 1.21+
- [Node.js](https://nodejs.org/) 22+

### 开发模式

```bash
# 终端 1 — Gateway
cd gateway
cp .env.example .env    # 编辑 API Key
go run .

# 终端 2 — Tool Agent
cd tool-agent
cp .env.example .env
go run .

# 终端 3 — Web
cd web
npm install
npm run dev
```

浏览器打开 http://localhost:5173。在 Settings 中添加 API Key 即可开始。

### Docker

```bash
docker compose up -d
```

浏览器打开 http://localhost:8080。

### 桌面版（单个 .exe）

构建一个自包含的可执行文件，无需 Docker，无需命令行。双击即用。

**环境要求：** Go 1.21+、Node.js

**Windows（PowerShell）：**
```powershell
git clone https://github.com/night556/beleader.git
cd beleader\desktop
.\build.ps1
# → dist\beleader-windows-amd64.exe
```

**macOS / Linux：**
```bash
git clone https://github.com/night556/beleader.git
cd beleader/desktop
./build.sh
# → dist/beleader-<os>-<arch>
```

exe 包含一切：内嵌 Web UI、SQLite 数据库、所有服务合一进程。数据存储于 `~/.beleader/`。

## 配置项

### Gateway

复制 `gateway/.env.example` 为 `gateway/.env` 并修改。

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PORT` | `8082` | HTTP 服务端口 |
| `DB_DRIVER` | `sqlite` | 数据库驱动：`sqlite`、`mysql`、`postgres` |
| `DB_PATH` | `./data/gateway/gateway.db` | SQLite 数据库路径 |
| `DB_HOST` | `127.0.0.1` | MySQL/PostgreSQL 主机 |
| `DB_PORT` | `3306` / `5432` | MySQL/PostgreSQL 端口 |
| `DB_USER` | `beleader` | 数据库用户 |
| `DB_PASSWORD` | | 数据库密码 |
| `DB_NAME` | `beleader` | 数据库名 |
| `GATEWAY_TOKEN` | `rt_dev_xxx` | Tool-agent 注册密钥 |
| `LOG_DIR` | (stdout) | 日志文件目录 |

### Tool Agent

复制 `tool-agent/.env.example` 为 `tool-agent/.env` 并修改。

| 环境变量 | 默认值 | 说明 |
|---------|--------|------|
| `PORT` | `8083` | HTTP 服务端口 |
| `GATEWAY_URL` | `http://gateway:8082` | Gateway 地址，用于自动注册 |
| `GATEWAY_TOKEN` | `rt_dev_xxx` | 须与 Gateway 的 `GATEWAY_TOKEN` 一致 |
| `POOL` | (hostname) | 加入的 Pool 名称 |
| `WORKSPACE_ROOT` | `/app/data` | 线程工作区根目录 |
| `RESTRICT_WORKSPACE` | `false` | 设为 `true` 时限制文件操作在工作区内 |
| `TOOLS` | (全部启用) | 逗号分隔的工具名列表。可用：`read_file`、`read_dir`、`write_file`、`edit_file`、`delete_file`、`search_content`、`search_files`、`run_command`、`task_output`、`task_stop`、`web_search`、`web_fetch`、`run_http_request` |

## 协议

MIT
