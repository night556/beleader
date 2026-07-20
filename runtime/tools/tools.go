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
	"Execute a shell command in the workspace directory. Set background=true for long-running commands like npm install, go build, or dev servers — returns a session_id. Use task_output to check or wait for results, and task_stop to kill.",
	map[string]any{
		"command":    map[string]any{"type": "string", "description": "Shell command to execute."},
		"timeout":    map[string]any{"type": "integer", "description": "Max seconds for sync mode. Default 60, max 120."},
		"background": map[string]any{"type": "boolean", "description": "Set true for long-running commands. Returns session_id immediately."},
	},
	[]string{"command"},
)

var taskOutputTool = mkTool("task_output",
	"Get output from a background command started with run_command(background=true). Two modes: block=false (check immediately, returns incremental output since last check) and block=true (wait up to wait seconds for completion).",
	map[string]any{
		"id":    map[string]any{"type": "string", "description": "Session ID returned by run_command."},
		"block": map[string]any{"type": "boolean", "description": "Whether to block until the command completes. Default false."},
		"wait":  map[string]any{"type": "integer", "description": "Max seconds to wait when block=true. Default 30."},
	},
	[]string{"id"},
)

var taskStopTool = mkTool("task_stop",
	"Stop a running background command started with run_command(background=true). Returns the final output of the killed process.",
	map[string]any{
		"id": map[string]any{"type": "string", "description": "Session ID to kill."},
	},
	[]string{"id"},
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
		runCommandTool, taskOutputTool, taskStopTool,
		runHTTPRequestTool,
		webSearchTool, webFetchTool,
	}
}

// DefaultTools returns the builtin tools for a default worker.
func DefaultTools(vision bool) []openai.Tool {
	tools := BaseTools(vision)
	tools = append(tools, readStatusTool, updateStatusTool)
	return tools
}

// AllToolDefs returns every builtin tool definition. This is the single source of
// truth for the Gateway's /v1/tools endpoint and for worker baseToolDefs.
func AllToolDefs() []engine.ToolDef {
	allDefs := []engine.ToolDef{
		engine.MkTool("read_file",
			"Read a file from the filesystem. Returns file contents as text, or image data for image files when vision is enabled.",
			map[string]any{
				"path":   map[string]any{"type": "string", "description": "Absolute path to the file."},
				"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)."},
				"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
			}, []string{"path"}),
		engine.MkTool("read_dir",
			"List contents of a directory.",
			map[string]any{
				"path": map[string]any{"type": "string", "description": "Absolute path to the directory."},
			}, []string{"path"}),
		engine.MkTool("write_file",
			"Create or overwrite a file. Creates parent directories if needed. For STATUS.md, use update_status instead.",
			map[string]any{
				"path":    map[string]any{"type": "string", "description": "Absolute path to the file."},
				"content": map[string]any{"type": "string", "description": "File content."},
			}, []string{"path", "content"}),
		engine.MkTool("edit_file",
			"Replace a single occurrence of old_string with new_string in a file.",
			map[string]any{
				"path":       map[string]any{"type": "string", "description": "Absolute path to the file."},
				"old_string": map[string]any{"type": "string", "description": "Exact text to replace."},
				"new_string": map[string]any{"type": "string", "description": "Replacement text."},
			}, []string{"path", "old_string", "new_string"}),
		engine.MkTool("delete_file",
			"Move files/directories to a .trash directory.",
			map[string]any{
				"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Array of absolute paths to delete."},
			}, []string{"paths"}),
		engine.MkTool("search_content",
			"Search for a pattern in file contents. path must be a single file, not a directory.",
			map[string]any{
				"pattern":       map[string]any{"type": "string", "description": "Regex pattern to search for."},
				"file_pattern":  map[string]any{"type": "string", "description": "Glob pattern for file matching."},
				"path":          map[string]any{"type": "string", "description": "Absolute path to a file."},
				"context_lines": map[string]any{"type": "integer", "description": "Lines of context around matches (max 10)."},
				"offset":        map[string]any{"type": "integer", "description": "Result offset for pagination."},
				"limit":         map[string]any{"type": "integer", "description": "Max results (max 200)."},
			}, []string{"pattern"}),
		engine.MkTool("search_files",
			"Find files by glob pattern recursively.",
			map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. '*.go', '**/*.tsx')."},
				"path":    map[string]any{"type": "string", "description": "Directory to search in."},
			}, []string{"pattern"}),
		engine.MkTool("read_status",
			"Read the project STATUS.md file to see current project state, progress, completed tasks, and document references. Always read STATUS.md before updating it.",
			nil, nil),
		engine.MkTool("update_status",
			"Update STATUS.md — add completed items, update progress, record decisions. Preserve existing content; only change the sections that need updating. Keep a '## Recent Activity' section with timestamped entries. Use this ONLY for STATUS.md; use write_file for all other files.",
			map[string]any{
				"content": map[string]any{"type": "string", "description": "The complete updated STATUS.md content, preserving all existing sections and adding new information to the Recent Activity section."},
			}, []string{"content"}),
		engine.MkTool("run_command",
			"Execute a shell command. Set background=true for long-running commands (returns session_id). Use task_output to check/wait and task_stop to kill.",
			map[string]any{
				"command":    map[string]any{"type": "string", "description": "Shell command to execute."},
				"background": map[string]any{"type": "boolean", "description": "Run in background. Returns session_id for task_output/task_stop."},
				"timeout":    map[string]any{"type": "integer", "description": "Timeout in seconds (default 60, max 120)."},
			}, []string{"command"}),
		engine.MkTool("task_output",
			"Get output from a background command. Use block=false to check immediately; block=true to wait for completion.",
			map[string]any{
				"id":    map[string]any{"type": "string", "description": "Session ID returned by run_command."},
				"block": map[string]any{"type": "boolean", "description": "Whether to block until the command completes (default false)."},
				"wait":  map[string]any{"type": "integer", "description": "Max seconds to wait when block=true (default 30)."},
			}, []string{"id"}),
		engine.MkTool("task_stop",
			"Stop a running background command and return its final output.",
			map[string]any{
				"id": map[string]any{"type": "string", "description": "Session ID to kill."},
			}, []string{"id"}),
		engine.MkTool("web_search",
			"Search the web using Bing and return results.",
			map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query."},
			}, []string{"query"}),
		engine.MkTool("web_fetch",
			"Fetch content from a URL and return as text.",
			map[string]any{
				"url":  map[string]any{"type": "string", "description": "URL to fetch."},
				"json": map[string]any{"type": "boolean", "description": "Parse response as JSON."},
			}, []string{"url"}),
		engine.MkTool("run_http_request",
			"Make an HTTP request with custom method, headers, and body.",
			map[string]any{
				"url":     map[string]any{"type": "string", "description": "URL to request."},
				"method":  map[string]any{"type": "string", "description": "HTTP method (GET, POST, PUT, DELETE, etc.)."},
				"headers": map[string]any{"type": "object", "description": "Request headers."},
				"body":    map[string]any{"type": "string", "description": "Request body."},
			}, []string{"url"}),
		engine.MkTool("spawn_worker",
			"Spawn a sub-agent to handle a focused task autonomously.",
			map[string]any{
				"system_prompt": map[string]any{"type": "string", "description": "System prompt for the worker agent."},
				"task":          map[string]any{"type": "string", "description": "Task description for the worker to execute."},
				"tools":         map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "List of tool names the worker may use."},
			}, []string{"system_prompt", "task"}),
	}
	allDefs = append(allDefs, ManagementToolDefs()...)
	return allDefs
}

// RegisterAll registers all builtin tool handlers on the engine.
func RegisterAll(eng *engine.Engine) {
	RegisterFileTools(eng)
	RegisterExecTools(eng)
	RegisterWebTools(eng)
	RegisterStatusTools(eng)
	eng.RegisterTool("spawn_worker", spawnWorkerHandler)
	RegisterManagementTools(eng)
}
