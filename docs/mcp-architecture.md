# MCP 支持与多端架构演进

## Context

用户希望产品支持多端（桌面、移动、鸿蒙等），并面临两个现实问题：
1. Go 跑不了鸿蒙/受限制平台，工具实现必须按平台原生
2. LLM 需要操作远程机器（如代码开发），手机/平板没这个能力

解决思路：MCP（Model Context Protocol）作为统一工具协议，让 LLM 不用关心工具跑在哪。客户端内置 SSH 可一键在远程机器部署 MCP server，扩展能力边界。

本文档记录完整愿景、架构决策、三个 Track 的方案，以及未决问题。**Track A 是近期可执行的，Track B/C 需要更多决策才能落地。**

---

## MCP 协议原理

MCP 是 Anthropic 开源的 JSON-RPC 2.0 协议，定义 client/server 之间发现工具（`tools/list`）、调用工具（`tools/call`）、读资源（`resources/list`、`resources/read`）。

两种 transport：

### 1. HTTP/SSE transport（对应"URL + Key"方式）
- MCP server 是远程 HTTP 服务
- client 发 JSON-RPC 请求，server 响应（流式走 SSE，非流式走 POST）
- 认证靠 HTTP header（Bearer token / 自定义 header / OAuth）
- **优点**：零安装、跨机器、易共享
- **缺点**：需要网络、认证配置麻烦、延迟稍高
- 典型：Cloudflare 远程 MCP、Smithery 托管 server

### 2. stdio transport（对应"npx 安装"方式）
- MCP server 是本地子进程，client 作为父进程启动
- stdin/stdout 通信（line-delimited JSON-RPC），stderr 单独收集
- `npx -y <pkg>` 临时下载 npm 包并执行，不用预装
- 也可以是 `python -m`、`uvx`、直接二进制——npx 只是 npm 生态的 launcher
- **优点**：本地快、能访问本地文件/进程、生态最大
- **缺点**：需要 node/python、首次下载慢、进程管理复杂
- 典型：filesystem、git、sqlite、playwright MCP server

**关键点**：两种 transport 之上的协议完全一样，client 代码可抽象出 transport 接口，两种实现共用上层逻辑。

---

## 整体架构愿景

```
[客户端 A (桌面)]                    [客户端 B (鸿蒙)]
  内置 LLM 调用                       内置 LLM 调用
  内置基础工具 (Go)                   内置基础工具 (ArkTS, 受限)
  内置 MCP client                     内置 MCP client (只连自家 server)
  项目/会话状态                       项目/会话状态
       │                                  │
       │ SSH                              │
       ▼                                  ▼
  [远程 MCP servers]                  [远程 MCP servers]
       │                                  │
       └──────────┐    ┌──────────────────┘
                  ▼    ▼
            [Backend (可选)]
              数据同步
              服务端编排（可选）
```

### 架构决策（用户确认）

1. **编排层放客户端**：客户端自己调 LLM API、管对话状态、管项目。Backend 退化为可选服务，提供两个功能：
   - **数据同步**：多端共享对话/项目/知识库/MCP 配置
   - **服务端编排**：可选，用于重任务、多客户端协作、后台任务

2. **基础工具按平台原生**：每个平台用自己的语言实现基础工具（Go/ArkTS/Swift/Kotlin）。但仍提供远端 MCP 工具——因为代码开发等必须在远端，手机没这能力。

3. **MCP server 跨平台策略**：
   - **内置 MCP server**（我们自己写）：任意平台都能跑
   - **网上 MCP server**：在受限平台（鸿蒙）客户端直接限制可用的列表

4. **工具路由**：多机器状态下确实复杂（LLM 调 `read_file`，发到哪台机器？），**需要进一步讨论**。

---

## 两层工具暴露架构（解决工具爆炸问题）

接入 3-4 个 MCP server 后工具数可能上百，全量 schema 塞进 prompt 会吃掉 ~20k token。

### 方案

| 工具来源 | 给 LLM 什么 | token 成本（100 工具） |
|---------|------------|---------------------|
| 内置核心工具 | 完整 schema | ~5k（数量可控） |
| MCP 工具 | 名字 + 一句话描述 | ~3k |
| 元工具 `get_tool_detail(name)` | 完整 schema | ~200 |

LLM 调用流程：看到 `mcp__github__create_issue` → 调 `get_tool_detail` 拿 schema → 调本工具。

### 效果评估

- **纯 meta-tool 方案**（只给 `search_tools` + `call_tool`）：省 token 但 LLM 掉点 15-20%（LLM 不知道有什么才搜，多一轮 round-trip）
- **两层架构**：掉点 < 5%，几乎无感。LLM 仍能看到全宇宙有什么，只是不知道参数细节
- **语义检索**（embedding + top-K）：~90% 保真，但需要 embedding 模型，复杂度高，工具数 > 200 才值得

**选两层架构**——简单、保真度高、解决规模问题。

### 工具命名空间

`mcp__<server_name>__<tool_name>` 前缀避免和内置工具冲突。如 `mcp__filesystem__read_file`、`mcp__github__create_issue`。

---

## Track A：现有 Go backend 加 MCP Client

**近期可执行**。在现有 Go backend 上加 MCP client，支持 stdio + HTTP 两种 transport，让 LLM 能调用任意 MCP server 暴露的工具。

**前提假设**：桌面端短期内仍然 Go backend + Tauri 前端，backend 做编排。未来桌面端转"客户端做编排"架构时，这段 MCP client 代码迁移。

### A.1 依赖

`github.com/mark3labs/mcp-go` — Go 生态主流 MCP 库，client + server 都有，支持 stdio 和 HTTP transport。

不用官方 `modelcontextprotocol/go-sdk`——生态小、迭代慢。

### A.2 DB 层：MCP server 配置表

**文件**：`backend/db/db.go`

新增表 `mcp_servers`：
```go
type MCPServer struct {
    ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
    Name      string    `gorm:"size:64;uniqueIndex" json:"name"`
    Type      string    `gorm:"size:16" json:"type"`                  // "stdio" | "http"
    Enabled   bool      `gorm:"default:0" json:"enabled"`
    Command   string    `gorm:"size:512;default:''" json:"command"`   // stdio: "npx"
    Args      string    `gorm:"type:text;default:''" json:"args"`     // stdio: JSON 数组
    Env       string    `gorm:"type:text;default:''" json:"env"`      // stdio: JSON 对象
    URL       string    `gorm:"size:512;default:''" json:"url"`       // http
    Headers   string    `gorm:"type:text;default:''" json:"headers"`  // http: JSON 对象
    CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
    UpdatedAt time.Time `gorm:"autoUpdateTime" json:"updated_at"`
}
```

CRUD：`CreateMCPServer` / `UpdateMCPServer` / `DeleteMCPServer` / `ListMCPServers` / `GetMCPServerByID`。AutoMigrate 注册。

### A.3 MCP Client Manager

**新文件**：`backend/mcp/manager.go`

```go
type Manager struct {
    db      *db.DB
    mu      sync.RWMutex
    clients map[string]*clientEntry  // name → entry
}

type clientEntry struct {
    client    *mcp.Client
    transport mcp.Transport
    tools     []mcp.Tool
    toolIndex map[string]mcp.Tool     // toolName → tool
}
```

核心方法：
- `Start()` — 启动时加载 enabled server，逐个连接 + `tools/list`
- `Connect(config) error` — 按 type 建 stdio 或 http transport
- `Disconnect(name)` / `Reconnect(name)`
- `CallTool(ctx, fullToolName, args) (*ToolResult, error)` — 解析 `mcp__<server>__<tool>`，调 `tools/call`
- `GetToolSchema(fullToolName) (json.RawMessage, error)` — 给 `get_tool_detail` 用
- `ListExposedTools() []ExposedTool` — 返回所有 MCP 工具的 name+desc

**stdio transport 注意（Windows）**：
- Windows 上 `npx` 实际是 `npx.cmd`，spawn 需 `shell: true` 或 `cmd /c npx ...`
- stderr 单独收集到日志，不混进 stdout（stdout 是 JSON-RPC 通道）
- 首次 `npx -y` 下载可能 30 秒+，UI 要显示"连接中…"

**HTTP transport**：
- mcp-go 支持 streamable HTTP 和 SSE
- headers 带配置里的自定义 header + Bearer token

### A.4 工具注入：两层暴露

**文件**：`backend/tools/tools.go`

改造 `BaseTools` / `MainTools` / `CoordinatorTools` / `WorkerTools`，在末尾注入 MCP 工具 stub + `get_tool_detail` 元工具：

```go
func MainTools(vision bool, mcpMgr *mcp.Manager) []openai.Tool {
    tools := BaseTools(vision)
    tools = append(tools, createProjectTool, listProjectsTool, ...)
    if mcpMgr != nil {
        for _, t := range mcpMgr.ListExposedTools() {
            tools = append(tools, openai.Tool{
                Type: "function",
                Function: &openai.FunctionDefinition{
                    Name:        t.Name,           // mcp__filesystem__read_file
                    Description: t.Description,
                    Parameters: map[string]any{
                        "type": "object",
                        "properties": map[string]any{
                            "_args": map[string]any{
                                "type": "string",
                                "description": "调用前先 get_tool_detail(\"" + t.Name + "\")",
                            },
                        },
                    },
                },
            })
        }
        tools = append(tools, getToolDetailTool)
    }
    return tools
}
```

`get_tool_detail` 元工具：handler 返回指定工具的完整 input schema JSON。

### A.5 MCP 工具 handler 注册

**文件**：`backend/api/session_runner.go`

所有 session 类型的注册流程末尾加：
```go
if h.MCPMgr != nil {
    for _, t := range h.MCPMgr.ListExposedTools() {
        toolName := t.Name
        mgr.RegisterTool(toolName, func(ctx context.Context, args string) *session.ToolResult {
            return h.MCPMgr.CallTool(ctx, toolName, args)
        })
    }
    mgr.RegisterTool("get_tool_detail", func(ctx context.Context, args string) *session.ToolResult {
        var p struct{ Name string `json:"name"` }
        json.Unmarshal([]byte(args), &p)
        schema, err := h.MCPMgr.GetToolSchema(p.Name)
        if err != nil { return &session.ToolResult{Error: err.Error()} }
        return &session.ToolResult{Content: string(schema)}
    })
}
```

### A.6 HTTP API

**新文件**：`backend/api/mcp.go` + `backend/api/handler.go` 注册路由

- `GET /api/mcp/servers` — 列出
- `POST /api/mcp/servers` — 新建
- `PUT /api/mcp/servers/:id` — 更新
- `DELETE /api/mcp/servers/:id` — 删除
- `POST /api/mcp/servers/:id/test` — 测试连接 + `tools/list`，返回工具数和清单
- `POST /api/mcp/servers/:id/connect` — 启用并连接
- `POST /api/mcp/servers/:id/disconnect` — 断开

### A.7 启动集成

**文件**：`backend/server/main.go`

```go
mcpMgr := mcp.NewManager(database)
if err := mcpMgr.Start(); err != nil {
    log.Printf("[MCP] start warning: %v", err)  // 不阻断启动
}
h.MCPMgr = mcpMgr
```

单 server 挂了不影响其他工具。

### A.8 前端

**文件**：`web/index.html`、`web/js/panels.js`、`web/css/base.css`、`web/js/i18n.js`、`web/js/state.js`

- Settings 面板加 MCP Servers section（Models 之后）
- server 列表项：状态点（绿=已连接/灰=未启用/红=失败）+ name + type + 工具数 + [测试][编辑][✕]
- 新建/编辑弹窗（复用现有 modal 组件）：
  - Type 切换：stdio / http
  - stdio 字段：Name、Command、Args（多行）、Env（key=value）、Enabled
  - http 字段：Name、URL、Headers（key: value）、Auth Token、Enabled
  - 测试连接按钮
- i18n 补 `mcp.*` 条目
- state.js 加 `_mcpServersCache`

### A.9 验证

1. **stdio**：配 `npx -y @modelcontextprotocol/server-filesystem /tmp/test` → 测试返回 8-10 工具 → 启用后 LLM 调用 `get_tool_detail` 再调本工具
2. **HTTP**：配远程 MCP server URL → 测试返回工具数 → LLM 调用
3. **两层暴露**：启用 3 个 server（~30 工具）→ 检查 LLM 收到的 tool list（MCP 工具只有 name+desc）→ prompt token 对比
4. **生命周期**：启动时无 MCP server 正常 → 运行中添加 → 测试 → 启用 → 工具立即可用 → 删除正在用的 → 工具消失
5. **错误**：stdio 命令不存在 → 清晰错误；http 不可达 → 超时错误；运行中崩溃 → 调用时报错（本期不自动重连）

### A.10 明确排除

- 工具过滤（用户勾选启用 server 的哪些工具）——本期全量暴露该 server 所有工具
- 自动重连——stdio 崩溃不自动重启，手动重连
- OAuth 流程——手动填 token
- per-project MCP 配置——本期全局
- MCP resources/prompts——本期只支持 tools
- Track B / Track C

---

## Track B：客户端 SSH + 一键装远程 MCP Server

**依赖 Track A 完成**。客户端能 SSH 到其他机器，一键部署默认 MCP server，让 LLM 能操作远程机器。

### B.1 场景

- 用户在手机上用 agent，想让 LLM 操作桌面机上的代码 → 手机客户端 SSH 到桌面机，装 filesystem MCP server → LLM 通过远程 MCP 操作桌面文件
- 用户在平板上让 agent 跑命令 → SSH 到 Linux 服务器，装 shell MCP server

### B.2 功能

1. **SSH 连接管理**：客户端存 SSH 连接配置（host、port、user、key/password）
2. **远程 OS 检测**：连上后自动识别 Linux/macOS/Windows
3. **默认 MCP server 清单**：策划一组常用 server（filesystem、shell、git、sqlite 等），按远程 OS 给出可装清单
4. **一键安装脚本**：每 server 一个安装脚本，检测 node/python 环境，缺则先装运行时
5. **安装后自动配置**：装完在客户端 MCP 配置里加一条 stdio over SSH 的 entry
6. **连接状态监控**：远程 server 掉线感知

### B.3 技术点

- **stdio over SSH**：MCP stdio transport 走 stdin/stdout，SSH 可以转发——`ssh host npx -y @mcp/server-fs /path` 即可。mcp-go 的 stdio transport 改造为走 SSH session 的 stdin/stdout
- **凭证安全**：SSH 私钥/password 存客户端，不上传 backend。可选集成系统 keychain
- **远程运行时检测**：`which node`、`which python`、`which npx`，缺哪个装哪个（apt/brew/winget）
- **默认 server 清单**：维护一个 JSON 配置，列名 server 的安装命令、所需运行时、暴露工具数

### B.4 UI

- 客户端新增"远程机器"管理面板
- 添加 SSH 连接 → 测试 → 连上后显示远程 OS + 已装 MCP server
- "安装 MCP server" 按钮 → 弹出可用清单 → 勾选 → 一键装
- 装完自动加入 MCP server 列表，标记为"remote: <host>"

### B.5 未决问题

- SSH 凭证存哪？系统 keychain / 加密存 DB / 用户每次输入？
- 远程机器没装 node/python 怎么办？自动装还是提示用户？
- 多客户端共享同一远程机器的 MCP server 配置吗？
- 远程 MCP server 崩溃谁负责重启？（客户端检测 + 重连？）

---

## Track C：多端客户端架构

**战略级，需要更多决策**。把编排层从 Go backend 迁到客户端，支持桌面、移动、鸿蒙等多端。

### C.1 架构

```
[客户端 (每平台原生)]
  编排层（调 LLM、管状态、管项目）
  基础工具（平台原生实现）
  MCP client（连本地 + 远程 MCP server）
  SSH 客户端（Track B）
       │
       │ 可选连接
       ▼
  [Backend (可选)]
    数据同步
    服务端编排
```

### C.2 每平台实现策略

| 平台 | 编排层语言 | 基础工具 | MCP 实现 |
|------|----------|---------|---------|
| 桌面 (Win/Mac/Linux) | Go（现状）或 Rust | Go/Rust 原生 | mcp-go 或 rust-mcp |
| 鸿蒙 | ArkTS | ArkTS 原生（受限） | 自家 MCP server（ArkTS），第三方限制 |
| iOS | Swift | Swift 原生（受限） | 自家 MCP server（Swift） |
| Android | Kotlin | Kotlin 原生（受限） | 自家 MCP server（Kotlin） |

### C.3 关键设计点

1. **编排层跨端共享**：编排逻辑（LLM 调用、对话状态机、工具路由）每端重写成本高。可选：
   - 用 TS 写编排层，各端嵌入（React Native / Tauri Mobile / Capacitor）
   - 用 Rust 写编排层，各端 FFI 调用
   - 每端原生重写（成本最高，但平台体验最好）

2. **基础工具接口统一**：每平台原生实现，但接口契约统一（tools/list 返回相同 schema）。MCP 协议天然适合做这个抽象。

3. **数据同步**：
   - 同步什么：对话历史、项目、知识库、MCP 配置、Agent 模板
   - 冲突处理：last-write-wins 还是 CRDT？
   - 离线编辑：客户端本地优先，上线后同步

4. **服务端编排触发条件**：
   - 客户端算力不够（大模型本地推理？不适用，LLM 走 API）
   - 多客户端协作（多人同项目）
   - 后台任务（定时跑、长任务）
   - 重计算任务（向量化、知识库索引）

5. **工具路由（未决）**：
   - LLM 调 `read_file` → 哪台机器的文件系统？
   - 候选方案：
     - (a) 工具名带机器标识：`mcp__desktop__read_file` / `mcp__remote1__read_file`
     - (b) 路由表：`read_file` 默认路由到某机器，用户可切换
     - (c) LLM 主动选：给 LLM 一个 `list_machines` 工具 + `run_on(machine, tool, args)` 元工具
   - **需要进一步讨论**

6. **鸿蒙可行性验证（必须先做）**：
   - 鸿蒙 app 能否 spawn 子进程？（影响 stdio MCP）
   - 鸿蒙 app 能否开本地 HTTP server？（影响 HTTP MCP，后台限制？）
   - 鸿蒙 ArkTS 能否调 LLM API？（网络权限、证书问题）
   - 鸿蒙能否跑 SSH 客户端？（Track B 鸿蒙版）
   - **如果鸿蒙跑不了 MCP，Track C 鸿蒙端只能做瘦客户端（纯显示，重活靠 SSH 到桌面 backend）**

### C.4 未决问题（需用户决策）

1. **桌面端短期形态**：
   - (a) 保持 Go backend + Tauri 前端，backend 做编排（现状）
   - (b) Go backend 合并进 Tauri 进程（sidecar），不再有独立 server
   - (c) 桌面端重写为 Rust/TS
   - 这决定 Track A 代码的长期归宿

2. **Track C MVP 平台**：先支持哪个非桌面端？鸿蒙 / iOS / Android？商业驱动是什么？

3. **数据同步范围**：同步全部数据还是子集？实时同步还是手动？

4. **服务端编排的必要性**：用户真的需要吗？什么场景？如果不需要，backend 可以更轻。

5. **工具路由模型**：见 C.3.5，需要选一个方案

6. **鸿蒙技术可行性**：见 C.6，需要先验证

### C.5 实施路径建议

不要直接跳进 Track C。建议：
1. 先做 Track A（桌面版能用 MCP，接生态）
2. 再做 Track B（扩展到远程机器）
3. 验证鸿蒙可行性（做个最小 demo，看能不能跑 MCP / 调 LLM API）
4. 决定 Track C MVP 平台和编排层技术选型
5. 才开始 Track C 实施

### C.6 鸿蒙可行性验证清单

在规划 Track C 鸿蒙版前，必须验证：
- [ ] 鸿蒙 DevEco Studio 能否开发具备网络权限的 app
- [ ] ArkTS 能否 fetch 外部 HTTPS API（LLM API）
- [ ] app 能否 spawn 子进程（stdio MCP）——预计不行
- [ ] app 能否在后台开 HTTP server（HTTP MCP）——预计受限
- [ ] 能否集成 SSH 客户端库
- [ ] 能否访问本地文件系统

如果 3-4 项不行，鸿蒙端只能做瘦客户端：纯显示 + 轻量本地工具，重活通过 SSH 到桌面 backend 执行。这其实就是 Track B 的场景。

---

## 涉及文件汇总（仅 Track A）

**后端**：
- `backend/db/db.go` — MCPServer 表 + CRUD
- `backend/mcp/manager.go` — **新文件**，MCP client manager
- `backend/tools/tools.go` — `*Tools()` 注入 MCP stub + `get_tool_detail` 元工具
- `backend/api/mcp.go` — **新文件**，HTTP handlers
- `backend/api/handler.go` — 注册 MCP 路由
- `backend/api/session_runner.go` — 注册 MCP 工具 handler
- `backend/server/main.go` — 启动 MCP manager
- `go.mod` — 加 `mark3labs/mcp-go` 依赖

**前端**：
- `web/index.html` — Settings 加 MCP section
- `web/js/panels.js` — MCP server 列表 + 编辑弹窗 + 测试连接
- `web/js/i18n.js` — `mcp.*` 条目
- `web/js/state.js` — `_mcpServersCache`
- `web/css/base.css` — MCP server 列表项样式

---

## 关键技术风险与坑

1. **Windows npx 路径**：`npx` 实际是 `npx.cmd`，Tauri/Go spawn 子进程时需要 `shell: true` 或直接指向 `where npx` 结果。项目 Windows 优先，必须先验证。
2. **stdio stderr 分离**：MCP server 日志走 stderr，不能混进 stdout（stdout 是 JSON-RPC 通道）。client 要单独收集 stderr 到日志文件。
3. **首次 npx 下载慢**：`npx -y` 第一次下包可能 30 秒+。UI 要显示"连接中…"，不能让用户以为卡死。
4. **工具数爆炸**：接 3-4 个 MCP server 后工具列表可能上百个。两层架构是必需的，不是可选优化。
5. **MCP 协议版本演进**：MCP 还在演进（2024-11-05 → 2025-06-18 → 最新），选库时看支持的版本。mcp-go 跟得比较紧。
6. **鸿蒙 MCP 可行性**：未验证。如果不能 spawn 子进程 + 不能开 HTTP server，鸿蒙端 MCP 基本不可行，只能做瘦客户端。
7. **Track A 代码的长期归宿**：如果未来桌面端从 Go backend 迁移到 Tauri 内嵌或 Rust 重写，Track A 的 MCP client 代码要迁移。建议写成与 `session.Manager` 解耦的独立模块。

---

## 当前状态与下一步

- **Track A**：方案已细化，可立即实施。建议先做这个，桌面版立刻能用 MCP 生态。
- **Track B**：方案粗略，依赖 Track A 完成后细化。
- **Track C**：战略级，需要先回答 C.4 的几个决策问题 + 验证鸿蒙可行性，才能出实施方案。

**建议执行顺序**：Track A → Track B → 鸿蒙可行性验证 → Track C 决策 → Track C 实施。
