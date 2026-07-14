package tools

import (
	"beleader/runtime/engine"

	"github.com/sashabaranov/go-openai"
)

// mkTool creates an openai.Tool definition from a ToolDef.
func mkTool(name, description string, properties map[string]any, required []string) openai.Tool {
	params := map[string]any{"type": "object"}
	if properties != nil {
		params["properties"] = properties
	}
	if len(required) > 0 {
		params["required"] = required
	}
	return openai.Tool{
		Type: "function",
		Function: &openai.FunctionDefinition{
			Name:        name,
			Description: description,
			Parameters:  params,
		},
	}
}

func readFileToolForVision(vision bool) openai.Tool {
	t := readFileTool
	if !vision {
		return t
	}
	return openai.Tool{
		Type: t.Type,
		Function: &openai.FunctionDefinition{
			Name:        t.Function.Name,
			Description: t.Function.Description + " For image files (png/jpg/gif/webp/bmp), returns image content for visual analysis.",
			Parameters:  t.Function.Parameters,
		},
	}
}

// ── Tool definitions ──

var readFileTool = mkTool("read_file",
	"Read a file at the given absolute path. Read files before editing.",
	map[string]any{
		"path":   map[string]any{"type": "string", "description": "Absolute file path to read"},
		"offset": map[string]any{"type": "integer", "description": "Line number to start reading from. 1-indexed. Default 1."},
		"limit":  map[string]any{"type": "integer", "description": "Number of lines to read. Default all."},
	},
	[]string{"path"},
)

var readDirTool = mkTool("read_dir",
	"List files and subdirectories at the given absolute path.",
	map[string]any{
		"path": map[string]any{"type": "string", "description": "Absolute directory path to list"},
	},
	[]string{"path"},
)

var searchContentTool = mkTool("search_content",
	"Search for a keyword or pattern in a specific file. path must be a file, not a directory. Use search_files to discover files first.",
	map[string]any{
		"pattern":       map[string]any{"type": "string", "description": "Text or regex pattern to search for in file contents"},
		"file_pattern":  map[string]any{"type": "string", "description": "Optional glob pattern, e.g. '*.go' or '**/*.tsx'"},
		"path":          map[string]any{"type": "string", "description": "Path to a specific file to search. Must be a file, not a directory."},
		"context_lines": map[string]any{"type": "integer", "description": "Number of surrounding lines to show. Default 0."},
		"offset":        map[string]any{"type": "integer", "description": "Starting position for pagination. Default 0."},
		"limit":         map[string]any{"type": "integer", "description": "Max results to return. Default 50, max 200."},
	},
	[]string{"pattern"},
)

var searchFilesTool = mkTool("search_files",
	"Find files matching a glob pattern. Use for discovering project structure or locating files by name. For searching file contents, use search_content.",
	map[string]any{
		"pattern": map[string]any{"type": "string", "description": "Glob pattern to match filenames, e.g. '*.go', '*.tsx', 'test_*.py'. Default '*'."},
		"path":    map[string]any{"type": "string", "description": "Directory to search in. Defaults to workspace root."},
	},
	nil,
)

var writeFileTool = mkTool("write_file",
	"Create or overwrite a file. Parent directories created automatically. Prefer edit_file for small changes to existing files.",
	map[string]any{
		"path":    map[string]any{"type": "string", "description": "Absolute file path"},
		"content": map[string]any{"type": "string", "description": "Full file content"},
	},
	[]string{"path", "content"},
)

var editFileTool = mkTool("edit_file",
	"Apply targeted edits to an existing file. Use this INSTEAD OF write_file for small changes.",
	map[string]any{
		"path":       map[string]any{"type": "string", "description": "Absolute file path to edit"},
		"old_string": map[string]any{"type": "string", "description": "Exact text to replace"},
		"new_string": map[string]any{"type": "string", "description": "New text to replace with"},
	},
	[]string{"path", "old_string", "new_string"},
)

var runCommandTool = mkTool("run_command",
	`Execute a shell command in the workspace directory.

Two modes - choose correctly:

SYNCHRONOUS (default) - ONLY for fast commands that will finish in under 2 minutes.
Examples: ls, cat, grep, mkdir, git status, go test ./pkg/..., echo, which
Sync timeout max is 120s. If not 100% sure it finishes in time, use background.
Default timeout: 60s. You can set up to 120s.

BACKGROUND - for anything that might run long or hang.
MUST use background for: npm install, pip install, go build ./..., cargo build,
git clone, docker build, ffmpeg, any compilation, any package manager, network ops.
Also use background when the output is long and you want to stream it live.

Background workflow:
  1. Start: { "command": "npm install", "background": true }
     Returns immediately with a session_id.
  2. Poll: { "action": "poll", "session_id": "<id>" }
     Returns new output + status: "running" or "exited" (with exit code).
  3. Read full log: { "action": "log", "session_id": "<id>", "limit": 5000 }
  4. List all sessions: { "action": "list" }
  5. Stop: { "action": "kill", "session_id": "<id>" }
  6. Write to stdin: { "action": "write", "session_id": "<id>", "data": "y\n" }

After starting a service (dev server, etc.), verify it with web_fetch or browser_open.
If background cmd produces no output for 30s, stop polling - check log and decide.`,
	map[string]any{
		"command":    map[string]any{"type": "string", "description": "Shell command to execute. Required for sync/background modes."},
		"timeout":    map[string]any{"type": "integer", "description": "Max execution time in seconds for sync mode. Default 60, max 120."},
		"background": map[string]any{"type": "boolean", "description": "Set true to run in background. REQUIRED for any command that may exceed 120s. Returns session_id immediately."},
		"action":     map[string]any{"type": "string", "description": "Action for managing background sessions: list, poll, log, write, kill."},
		"session_id": map[string]any{"type": "string", "description": "Target session ID for poll/log/write/kill actions."},
		"data":       map[string]any{"type": "string", "description": "Data to write to stdin (for action=write)."},
		"offset":     map[string]any{"type": "integer", "description": "Byte offset for log action."},
		"limit":      map[string]any{"type": "integer", "description": "Max bytes to read for log action. Default 5000."},
	},
	nil,
)

var runHTTPRequestTool = mkTool("run_http_request",
	"Send an HTTP request. Use for testing APIs or fetching data.",
	map[string]any{
		"method":  map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "DELETE", "PATCH"}},
		"url":     map[string]any{"type": "string", "description": "Full URL to request"},
		"headers": map[string]any{"type": "object", "description": "Request headers as key-value pairs"},
		"body":    map[string]any{"type": "string", "description": "Request body for POST/PUT/PATCH"},
	},
	[]string{"method", "url"},
)

var webSearchTool = mkTool("web_search",
	"Search the web for information, documentation, best practices.",
	map[string]any{
		"query": map[string]any{"type": "string", "description": "Search query"},
	},
	[]string{"query"},
)

var webFetchTool = mkTool("web_fetch",
	"Fetch and read content of a web page. Use when you have a specific URL.",
	map[string]any{
		"url": map[string]any{"type": "string", "description": "URL to fetch"},
	},
	[]string{"url"},
)

var readStatusTool = mkTool("read_status",
	"Read the project STATUS.md file to see current project state, progress, and document references.",
	nil, nil,
)

var updateStatusTool = mkTool("update_status",
	"Update STATUS.md — add completed items, update progress, record decisions. Preserve existing content; only change the sections that need updating. Keep a '## Recent Activity' section with timestamped entries.",
	map[string]any{
		"content": map[string]any{
			"type":        "string",
			"description": "The complete updated STATUS.md content, preserving all existing sections and adding new information to the Recent Activity section",
		},
	},
	[]string{"content"},
)

// BaseTools returns file + exec + web tools shared by all agents.
func BaseTools(vision bool) []openai.Tool {
	return []openai.Tool{
		readFileToolForVision(vision), readDirTool, searchContentTool, searchFilesTool, writeFileTool, editFileTool,
		runCommandTool, runHTTPRequestTool,
		webSearchTool, webFetchTool,
	}
}

// DefaultTools returns the 12 builtin tools for a default worker.
func DefaultTools(vision bool) []openai.Tool {
	tools := BaseTools(vision)
	tools = append(tools, readStatusTool, updateStatusTool)
	return tools
}

// RegisterAll registers all builtin tool handlers on the engine.
func RegisterAll(eng *engine.Engine) {
	RegisterFileTools(eng)
	RegisterExecTools(eng)
	RegisterWebTools(eng)
	RegisterStatusTools(eng)
	eng.RegisterTool("spawn_worker", spawnWorkerHandler)
}
