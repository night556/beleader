package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"iamhuman/backend/session"

	"github.com/sashabaranov/go-openai"
)

// SpeakEnabled controls whether the speak TTS tool is registered.
var SpeakEnabled bool

func readFileToolForVision(vision bool) openai.Tool {
	if !vision {
		return readFileTool
	}
	return openai.Tool{
		Type: readFileTool.Type,
		Function: &openai.FunctionDefinition{
			Name:        readFileTool.Function.Name,
			Description: readFileTool.Function.Description + " For image files (png/jpg/gif/webp/bmp), returns image content for visual analysis.",
			Parameters:  readFileTool.Function.Parameters,
		},
	}
}

// ── Knowledge tools ──

var searchKnowledgeTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "search_knowledge",
		Description: "Search the cross-project knowledge base for relevant experience, conventions, and decisions. Use before starting work on a task to check if similar situations have been handled before.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query in natural language"},
				"limit": map[string]any{"type": "integer", "description": "Max results to return. Default 5, max 10."},
			},
			"required": []string{"query"},
		},
	},
}

var saveKnowledgeTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "save_knowledge",
		Description: "Save a reusable piece of knowledge to the cross-project knowledge base. Only save when the user taught you something you wouldn't have figured out on your own. One paragraph, self-contained, max 500 characters.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{"type": "string", "description": "The knowledge entry. Self-contained, one paragraph."},
				"tags":    map[string]any{"type": "string", "description": "Comma-separated keywords for search. e.g. 'UI, workflow, frontend'"},
			},
			"required": []string{"content", "tags"},
		},
	},
}

var deleteKnowledgeTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "delete_knowledge",
		Description: "Delete a knowledge entry by its ID. Use when a saved entry is outdated or incorrect.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "The knowledge entry ID to delete"},
			},
			"required": []string{"id"},
		},
	},
}

func RegisterKnowledgeTools(mgr *session.Manager, searchFn func(query string, limit int) (string, error), saveFn func(content, tags string) (int64, error), deleteFn func(id int64) error) {
	if searchFn != nil {
		mgr.RegisterTool("search_knowledge", makeSearchKnowledgeHandler(searchFn))
	}
	if saveFn != nil {
		mgr.RegisterTool("save_knowledge", makeSaveKnowledgeHandler(saveFn))
	}
	if deleteFn != nil {
		mgr.RegisterTool("delete_knowledge", makeDeleteKnowledgeHandler(deleteFn))
	}
}

func makeSearchKnowledgeHandler(searchFn func(query string, limit int) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return &session.ToolResult{Error: "invalid args: " + err.Error()}
		}
		result, err := searchFn(p.Query, p.Limit)
		if err != nil {
			return &session.ToolResult{Error: "search failed: " + err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeSaveKnowledgeHandler(saveFn func(content, tags string) (int64, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Content string `json:"content"`
			Tags    string `json:"tags"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return &session.ToolResult{Error: "invalid args: " + err.Error()}
		}
		if len(p.Content) > 500 {
			return &session.ToolResult{Error: "content too long (max 500 characters)"}
		}
		if len(p.Tags) > 200 {
			return &session.ToolResult{Error: "tags too long (max 200 characters)"}
		}
		id, err := saveFn(p.Content, p.Tags)
		if err != nil {
			return &session.ToolResult{Error: "save failed: " + err.Error()}
		}
		return &session.ToolResult{Content: fmt.Sprintf("Knowledge saved with ID %d", id)}
	}
}

func makeDeleteKnowledgeHandler(deleteFn func(id int64) error) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal([]byte(args), &p); err != nil {
			return &session.ToolResult{Error: "invalid args: " + err.Error()}
		}
		if err := deleteFn(p.ID); err != nil {
			return &session.ToolResult{Error: "delete failed: " + err.Error()}
		}
		return &session.ToolResult{Content: fmt.Sprintf("Knowledge %d deleted", p.ID)}
	}
}

// BaseTools returns file + exec + web tools shared by all agents.
func BaseTools(vision bool) []openai.Tool {
	return []openai.Tool{
		readFileToolForVision(vision), readDirTool, searchContentTool, searchFilesTool, writeFileTool, editFileTool,
		runCommandTool, runHTTPRequestTool,
		webSearchTool, webFetchTool,
	}
}

// MainTools returns tools for the main chat session.
func MainTools(vision bool) []openai.Tool {
	tools := BaseTools(vision)
	tools = append(tools, createProjectTool)
	tools = append(tools, listProjectsTool, getProjectStatusTool, searchConversationTool, deleteProjectTool)
	tools = append(tools, showHTMLTool, closeHTMLTool, listHTMLsTool, focusSessionTool, showFileTool)
	tools = append(tools, interveneProjectTool)
	if SpeakEnabled {
		tools = append(tools, speakTool)
	}
	tools = append(tools, listAgentsTool, createAgentTool, editAgentTool, deleteAgentTool)
	tools = append(tools, searchKnowledgeTool, saveKnowledgeTool, deleteKnowledgeTool)
	return tools
}

// CoordinatorTools returns tools for the Coordinator agent.
func CoordinatorTools(vision bool) []openai.Tool {
	return []openai.Tool{
		readFileToolForVision(vision), readDirTool, searchContentTool, searchFilesTool,
		webSearchTool, webFetchTool,
		readStatusTool, writeStatusTool,
		listAgentsTool, listWorkersTool, spawnWorkerTool, interveneWorkerTool, terminateWorkerTool, deleteWorkerTool,
		showHTMLTool, closeHTMLTool, listHTMLsTool, focusSessionTool, showFileTool,
		searchKnowledgeTool, saveKnowledgeTool, deleteKnowledgeTool,
	}
}

// WorkerTools returns tools for Worker agents.
func WorkerTools(vision bool) []openai.Tool {
	return BaseTools(vision)
}

func RegisterAll(mgr *session.Manager, workDir string, createProjectCallback func(title, prompt string) (string, string, error)) {
	RegisterFileTools(mgr, workDir)
	RegisterExecutionTools(mgr, workDir)
	RegisterWebTools(mgr)
	if createProjectCallback != nil {
		mgr.RegisterTool("create_project", makeCreateProjectHandler(createProjectCallback))
	}
}

func RegisterCoordinatorTools(
	mgr *session.Manager,
	spawnWorkerFn func(name, prompt, task string, enableBrowser, enableDesktop bool) (string, error),
	terminateWorkerFn func(workerName string) (string, error),
	deleteWorkerFn func(workerName string) (string, error),
	interveneWorkerFn func(workerName, message string) (string, error),
	listWorkersFn func() (string, error),
) {
	if spawnWorkerFn != nil {
		mgr.RegisterTool("spawn_worker", makeSpawnWorkerHandler(spawnWorkerFn))
	}
	if terminateWorkerFn != nil {
		mgr.RegisterTool("terminate_worker", makeTerminateWorkerHandler(terminateWorkerFn))
	}
	if deleteWorkerFn != nil {
		mgr.RegisterTool("delete_worker", makeDeleteWorkerHandler(deleteWorkerFn))
	}
	if interveneWorkerFn != nil {
		mgr.RegisterTool("intervene_worker", makeInterveneWorkerHandler(interveneWorkerFn))
	}
	if listWorkersFn != nil {
		mgr.RegisterTool("list_workers", makeListWorkersHandler(listWorkersFn))
	}
}

func RegisterProjectManagementTools(
	mgr *session.Manager,
	listProjectsFn func() (string, error),
	getProjectStatusFn func(refID string) (string, error),
	searchConversationFn func(query string) (string, error),
) {
	if listProjectsFn != nil {
		mgr.RegisterTool("list_projects", makeListProjectsHandler(listProjectsFn))
	}
	if getProjectStatusFn != nil {
		mgr.RegisterTool("get_project_status", makeGetProjectStatusHandler(getProjectStatusFn))
	}
	if searchConversationFn != nil {
		mgr.RegisterTool("search_conversation", makeSearchConversationHandler(searchConversationFn))
	}
}

func makeListProjectsHandler(fn func() (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		result, err := fn()
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeGetProjectStatusHandler(fn func(refID string) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct{ RefID string `json:"ref_id"` }
		json.Unmarshal([]byte(args), &p)
		if p.RefID == "" {
			return &session.ToolResult{Error: "ref_id required"}
		}
		result, err := fn(p.RefID)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeSearchConversationHandler(fn func(query string) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct{ Pattern string `json:"pattern"` }
		json.Unmarshal([]byte(args), &p)
		if p.Pattern == "" {
			return &session.ToolResult{Error: "pattern is required"}
		}
		result, err := fn(p.Pattern)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

var readFileTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "read_file",
		Description: "Read a file at the given absolute path. Read files before editing.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Absolute file path to read"},
				"offset": map[string]any{"type": "integer", "description": "Line number to start reading from. 1-indexed. Default 1."},
				"limit":  map[string]any{"type": "integer", "description": "Number of lines to read. Default all."},
			},
			"required": []string{"path"},
		},
	},
}

var readDirTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "read_dir",
		Description: "List files and subdirectories at the given absolute path.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Absolute directory path to list"},
			},
			"required": []string{"path"},
		},
	},
}

var searchContentTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "search_content",
		Description: "Search for a keyword or pattern across files. Use before creating new code to check if patterns already exist.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern":      map[string]any{"type": "string", "description": "Text or regex pattern to search for in file contents"},
				"file_pattern": map[string]any{"type": "string", "description": "Optional glob pattern, e.g. '*.go' or '**/*.tsx'"},
				"path":         map[string]any{"type": "string", "description": "Optional directory path to limit search scope"},
				"context_lines": map[string]any{"type": "integer", "description": "Number of surrounding lines to show. Default 0."},
			},
			"required": []string{"pattern"},
		},
	},
}


var searchFilesTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "search_files",
		Description: "Find files matching a glob pattern. Use for discovering project structure or locating files by name. For searching file contents, use search_content.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{"type": "string", "description": "Glob pattern to match filenames, e.g. '*.go', '*.tsx', 'test_*.py'. Default '*'."},
				"path":    map[string]any{"type": "string", "description": "Directory to search in. Defaults to workspace root."},
			},
		},
	},
}

var writeFileTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "write_file",
		Description: "Create or overwrite a file. Parent directories created automatically. Prefer edit_file for small changes to existing files.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "Absolute file path"},
				"content": map[string]any{"type": "string", "description": "Full file content"},
			},
			"required": []string{"path", "content"},
		},
	},
}

var editFileTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "edit_file",
		Description: "Apply targeted edits to an existing file. Use this INSTEAD OF write_file for small changes.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string", "description": "Absolute file path to edit"},
				"old_string": map[string]any{"type": "string", "description": "Exact text to replace"},
				"new_string": map[string]any{"type": "string", "description": "New text to replace with"},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
	},
}

var runCommandTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name: "run_command",
		Description: `Execute a shell command in the workspace directory.

Two modes:

SYNCHRONOUS (default) — for quick commands (<60s): tests, git, ls, cat, grep, mkdir.
  { "command": "go test ./..." }
  Returns combined output when the command finishes.

BACKGROUND — for long commands or persistent services:
  1. Start: { "command": "npm run dev", "background": true }
     Returns immediately with a session_id.
  2. Check: { "action": "poll", "session_id": "<id>" }
     Waits for new output. Returns status: "running" or "exited".
  3. Read log: { "action": "log", "session_id": "<id>", "offset": 0, "limit": 2000 }
     Gets buffered output without blocking.
  4. List all: { "action": "list" }
  5. Stop: { "action": "kill", "session_id": "<id>" }
  6. Write to stdin: { "action": "write", "session_id": "<id>", "data": "y\\n" }

When to use BACKGROUND:
- Commands that run indefinitely: npm run dev, go run ., python -m http.server
- Long builds/installs: go build, npm install, pip install
- Any command you want to monitor progress on

When to use SYNCHRONOUS:
- Quick one-shot commands: ls, cat, mkdir, git status, go test, echo
- Commands where you only need the final result

After starting a service in background, verify it with browser or web_fetch.
If a background command produces no new output for 30s, don't keep polling — check the last log and decide.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command":    map[string]any{"type": "string", "description": "Shell command to execute. Required for sync/background modes."},
				"timeout":    map[string]any{"type": "integer", "description": "Max execution time in seconds for sync mode. Default 60."},
				"background": map[string]any{"type": "boolean", "description": "Set true to run in background. Returns session_id immediately."},
				"action":     map[string]any{"type": "string", "description": "Action for managing background sessions: list, poll, log, write, kill."},
				"session_id": map[string]any{"type": "string", "description": "Target session ID for poll/log/write/kill actions."},
				"data":       map[string]any{"type": "string", "description": "Data to write to stdin (for action=write)."},
				"offset":     map[string]any{"type": "integer", "description": "Byte offset for log action."},
				"limit":      map[string]any{"type": "integer", "description": "Max bytes to read for log action. Default 5000."},
			},
		},
	},
}

var runHTTPRequestTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "run_http_request",
		Description: "Send an HTTP request. Use for testing APIs or fetching data.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"method":  map[string]any{"type": "string", "enum": []string{"GET", "POST", "PUT", "DELETE", "PATCH"}},
				"url":     map[string]any{"type": "string", "description": "Full URL to request"},
				"headers": map[string]any{"type": "object", "description": "Request headers as key-value pairs"},
				"body":    map[string]any{"type": "string", "description": "Request body for POST/PUT/PATCH"},
			},
			"required": []string{"method", "url"},
		},
	},
}

var webSearchTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "web_search",
		Description: "Search the web for information, documentation, best practices.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query"},
			},
			"required": []string{"query"},
		},
	},
}

var webFetchTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "web_fetch",
		Description: "Fetch and read content of a web page. Use when you have a specific URL.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{"type": "string", "description": "URL to fetch"},
			},
			"required": []string{"url"},
		},
	},
}

var createProjectTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "create_project",
		Description: "Create a new project. If prompt is given, the Coordinator will work on the task. If prompt is empty, the project is a conversation space where the Coordinator acts as a chat partner. When unsure, pass the user's original words as prompt.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":  map[string]any{"type": "string", "description": "Short title for the project"},
				"prompt": map[string]any{"type": "string", "description": "What to do, or leave empty for casual chat. When unsure, the user's original words."},
			},
		},
	},
}

var spawnWorkerTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "spawn_worker",
		Description: "Create a new Worker session to execute a task. Worker names must be unique per project — use list_workers first to check what exists, and intervene_worker to reuse an existing Worker. Name also looks up agent templates — matching templates load automatically. Use list_agents to check available templates. Provide prompt for one-off Workers not in the library.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":            map[string]any{"type": "string", "description": "Unique worker name. Must not already exist in this project. Also looks up agent templates — if an agent with this name exists, its prompt loads automatically."},
				"prompt":          map[string]any{"type": "string", "description": "Custom role definition. Overrides any matching template. Use for one-off Workers not in the template library."},
				"task":             map[string]any{"type": "string", "description": "The task for the Worker to execute. Becomes its first message. Should be self-contained with clear goals and constraints."},
				"enable_browser":  map[string]any{"type": "boolean", "description": "Enable browser automation tools for this Worker. Default false."},
				"enable_desktop":  map[string]any{"type": "boolean", "description": "Enable desktop automation tools for this Worker. Default false."},
			},
			"required": []string{"name", "task"},
		},
	},
}

var terminateWorkerTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "terminate_worker",
		Description: "Stop a Worker and free its resources. The Worker stays in the project and can be restarted via intervene_worker.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_name": map[string]any{"type": "string", "description": "Name of the Worker to terminate"},
			},
			"required": []string{"worker_name"},
		},
	},
}

var deleteWorkerTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "delete_worker",
		Description: "Permanently delete a Worker and all its data. ONLY use when explicitly asked by the user. For all other cases where you want to stop a Worker, use terminate_worker.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_name": map[string]any{"type": "string", "description": "Name of the Worker to delete"},
			},
			"required": []string{"worker_name"},
		},
	},
}

var listWorkersTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "list_workers",
		Description: "List all Workers in the current project with their name and status (running/idle). Use when you need to know which Workers are active before intervene_worker or terminate_worker.",
	},
}

var interveneWorkerTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "intervene_worker",
		Description: "Send a message to a Worker — give a follow-up task, ask for progress, or adjust direction. Works whether the Worker is running or idle.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"worker_name": map[string]any{"type": "string", "description": "Name of the Worker to message"},
				"message":     map[string]any{"type": "string", "description": "Message to inject into the Worker's session"},
			},
			"required": []string{"worker_name", "message"},
		},
	},
}

var listProjectsTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "list_projects",
		Description: "List all projects with their status. Use to find active projects.",
	},
}

var getProjectStatusTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "get_project_status",
		Description: "Get the status and progress of a project by reading its STATUS.md file.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref_id": map[string]any{"type": "string", "description": "Project reference ID"},
			},
			"required": []string{"ref_id"},
		},
	},
}

var searchConversationTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "search_conversation",
		Description: "Search past conversations. Use when user references something from earlier discussions.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Keyword or phrase to search for"},
			},
			"required": []string{"query"},
		},
	},
}

var listAgentsTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "list_agents",
		Description: "List all available agent templates from the agent library. Returns each agent's name and description.",
	},
}

var readStatusTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "read_status",
		Description: "Read the project STATUS.md file to see current project state, progress, and document references.",
	},
}

var writeStatusTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "write_status",
		Description: "Replace the entire STATUS.md file with new content. Use after reviewing current status and deciding what needs to change.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The complete new content for STATUS.md",
				},
			},
			"required": []string{"content"},
		},
	},
}

var createAgentTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "create_agent",
		Description: "Create a new agent (role) in the agent library. Stored directly in the database.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Agent name"},
				"content": map[string]any{"type": "string", "description": "Full system prompt content for the agent"},
			},
			"required": []string{"name", "content"},
		},
	},
}

var editAgentTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "edit_agent",
		Description: "Edit an existing agent's system prompt in the database.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Agent name to edit"},
				"content": map[string]any{"type": "string", "description": "New system prompt content"},
			},
			"required": []string{"name", "content"},
		},
	},
}

var deleteProjectTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "delete_project",
		Description: "Permanently delete a project by its ref_id. Cancels all sessions, releases resources, and removes the project directory from disk. Use list_projects first to find the ref_id.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref_id": map[string]any{"type": "string", "description": "Project reference ID to delete."},
			},
			"required": []string{"ref_id"},
		},
	},
}

var interveneProjectTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "intervene_project",
		Description: "Intervene in a project — send a message or instruction to its Coordinator. Use when the user wants to check progress, adjust direction, or give a new task without switching to the project tab. Use list_projects first to find the ref_id.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref_id":  map[string]any{"type": "string", "description": "Project reference ID to intervene in"},
				"message": map[string]any{"type": "string", "description": "Message to inject into the Coordinator's session"},
			},
			"required": []string{"ref_id", "message"},
		},
	},
}

var deleteAgentTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "delete_agent",
		Description: "Delete an agent from the database.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{"type": "string", "description": "Agent name to delete"},
			},
			"required": []string{"name"},
		},
	},
}


func validateRequired(args string, required []string) *session.ToolResult {
	var m map[string]any
	if err := json.Unmarshal([]byte(args), &m); err != nil {
		return &session.ToolResult{Error: "invalid args: " + err.Error()}
	}
	for _, k := range required {
		v, ok := m[k]
		if !ok || v == nil {
			return &session.ToolResult{Error: fmt.Sprintf("%%s is required", k)}
		}
		if s, ok := v.(string); ok && s == "" {
			return &session.ToolResult{Error: fmt.Sprintf("%%s is required", k)}
		}
	}
	return nil
}

func RegisterFileTools(mgr *session.Manager, workDir string) {
	mgr.RegisterTool("read_file", func(ctx context.Context, args string) *session.ToolResult {
		var p struct{ Path string; Offset int; Limit int }
		json.Unmarshal([]byte(args), &p)
		if p.Path == "" {
			return &session.ToolResult{Error: "path is required"}
		}
		data, err := os.ReadFile(p.Path)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}

		mime := detectImageMIME(data, filepath.Ext(p.Path))
		vision, _ := ctx.Value(session.CtxKeyVisionEnabled).(bool)
		if vision && mime != "" {
			data = compressImage(data, 1920, 75)
			b64 := base64.StdEncoding.EncodeToString(data)
			uri := fmt.Sprintf("data:%s;base64,%s", mime, b64)
			dims := ""
			if cfg, _, err := image.DecodeConfig(strings.NewReader(string(data))); err == nil {
				dims = fmt.Sprintf(", %dx%d", cfg.Width, cfg.Height)
			}
			fname := filepath.Base(p.Path)
			return &session.ToolResult{
				Content: fmt.Sprintf("Image: %s (%s%s)", fname, formatSizeForImage(len(data)), dims),
				Images:  []string{uri},
			}
		}

		// Apply offset/limit for text files
		if p.Offset > 0 || p.Limit > 0 {
			lines := strings.Split(string(data), "\n")
			start := p.Offset - 1
			if start < 0 {
				start = 0
			}
			if start >= len(lines) {
				start = len(lines) - 1
			}
			end := len(lines)
			if p.Limit > 0 {
				end = start + p.Limit
				if end > len(lines) {
					end = len(lines)
				}
			}
			data = []byte(strings.Join(lines[start:end], "\n"))
		}

		return &session.ToolResult{Content: string(data)}
	})

	mgr.RegisterTool("read_dir", func(ctx context.Context, args string) *session.ToolResult {
		var p struct{ Path string }
		json.Unmarshal([]byte(args), &p)
		if p.Path == "" {
			return &session.ToolResult{Error: "path is required"}
		}
		entries, err := os.ReadDir(p.Path)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		var lines []string
		for _, e := range entries {
			t := "file"
			if e.IsDir() {
				t = "dir"
			}
			lines = append(lines, fmt.Sprintf("%s  [%s]", e.Name(), t))
		}
		return &session.ToolResult{Content: strings.Join(lines, "\n")}
	})

	mgr.RegisterTool("search_content", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Pattern      string `json:"pattern"`
			FilePattern  string `json:"file_pattern"`
			Path         string `json:"path"`
			ContextLines int    `json:"context_lines"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.Pattern == "" {
			return &session.ToolResult{Error: "pattern is required"}
		}
		searchPath := p.Path
		if searchPath == "" {
			searchPath = workDir
		}
		pattern := p.FilePattern
		if pattern == "" {
			pattern = "*"
		}
		if p.ContextLines > 10 {
			p.ContextLines = 10
		}
		results, _ := searchFiles(searchPath, pattern, p.Pattern, p.ContextLines)
		if len(results) == 0 {
			return &session.ToolResult{Content: "No matches found."}
		}
		return &session.ToolResult{Content: strings.Join(results, "\n")}
	})

	mgr.RegisterTool("search_files", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Pattern string `json:"pattern"`
			Path    string `json:"path"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.Pattern == "" {
			p.Pattern = "*"
		}
		searchPath := p.Path
		if searchPath == "" {
			searchPath = workDir
		}
		var matches []string
		filepath.Walk(searchPath, func(fp string, fi os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if fi.IsDir() {
				base := filepath.Base(fp)
				if base == ".git" || base == "node_modules" || base == "__pycache__" || (len(base) > 0 && base[0] == '.') {
					return filepath.SkipDir
				}
				return nil
			}
			rel, _ := filepath.Rel(searchPath, fp)
			if matched, _ := filepath.Match(p.Pattern, filepath.Base(fp)); matched {
				size := fi.Size()
				var sz string
				switch {
				case size < 1024:
					sz = fmt.Sprintf("%dB", size)
				case size < 1024*1024:
					sz = fmt.Sprintf("%.1fKB", float64(size)/1024)
				default:
					sz = fmt.Sprintf("%.1fMB", float64(size)/(1024*1024))
				}
				matches = append(matches, fmt.Sprintf("%s  (%s)", rel, sz))
			}
			return nil
		})
		if len(matches) == 0 {
			return &session.ToolResult{Content: fmt.Sprintf("No files matching '%s' in %s", p.Pattern, searchPath)}
		}
		if len(matches) > 200 {
			remaining := len(matches) - 200
			matches = matches[:200]
			return &session.ToolResult{Content: strings.Join(matches, "\n") + fmt.Sprintf("\n\n... and %d more (truncated at 200). Refine pattern.", remaining)}
		}
		return &session.ToolResult{Content: strings.Join(matches, "\n")}
	})


	mgr.RegisterTool("write_file", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.Path == "" {
			return &session.ToolResult{Error: "path is required"}
		}
		dir := filepath.Dir(p.Path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		if err := os.WriteFile(p.Path, []byte(p.Content), 0644); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: fmt.Sprintf("File written: %s (%d bytes)", p.Path, len(p.Content))}
	})

	mgr.RegisterTool("edit_file", func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Path      string `json:"path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.Path == "" {
			return &session.ToolResult{Error: "path is required"}
		}
		if p.OldString == "" {
			return &session.ToolResult{Error: "old_string is required"}
		}
		data, err := os.ReadFile(p.Path)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		content := string(data)
		count := strings.Count(content, p.OldString)
		if count == 0 {
			return &session.ToolResult{Error: "old_string not found in file"}
		}
		if count > 1 {
			return &session.ToolResult{Error: fmt.Sprintf("old_string found %d times — provide more context to make it unique", count)}
		}
		newContent := strings.Replace(content, p.OldString, p.NewString, 1)
		if err := os.WriteFile(p.Path, []byte(newContent), 0644); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: fmt.Sprintf("File edited: %s", p.Path)}
	})

	// ── Status tools (Coordinator only) ──

	mgr.RegisterTool("read_status", func(ctx context.Context, args string) *session.ToolResult {
		statusPath := filepath.Join(workDir, "STATUS.md")
		data, err := os.ReadFile(statusPath)
		if err != nil {
			return &session.ToolResult{Error: "STATUS.md not found: " + err.Error()}
		}
		return &session.ToolResult{Content: string(data)}
	})

	mgr.RegisterTool("write_status", func(ctx context.Context, args string) *session.ToolResult {
		var p struct{ Content string `json:"content"` }
		json.Unmarshal([]byte(args), &p)
		if p.Content == "" {
			return &session.ToolResult{Error: "content is required"}
		}
		statusPath := filepath.Join(workDir, "STATUS.md")
		dir := filepath.Dir(statusPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		if err := os.WriteFile(statusPath, []byte(p.Content), 0644); err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: "STATUS.md updated (" + fmt.Sprintf("%d", len(p.Content)) + " bytes)"}
	})
}

func RegisterExecutionTools(mgr *session.Manager, workDir string) {
	setExecWorkDir(workDir)
	mgr.RegisterTool("run_command", execHandler)

	mgr.RegisterTool("run_http_request", httpRequestHandler)

	RegisterBrowserTools(mgr)
}

func RegisterWebTools(mgr *session.Manager) {
	mgr.RegisterTool("web_search", webSearchHandler)

	mgr.RegisterTool("web_fetch", webFetchHandler)
}

func makeCreateProjectHandler(callback func(title, prompt string) (string, string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Title  string `json:"title"`
			Prompt string `json:"prompt"`
		}
		json.Unmarshal([]byte(args), &p)
		if callback == nil {
			return &session.ToolResult{Error: "create_project not available in this session"}
		}
		refID, _, err := callback(p.Title, p.Prompt)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: fmt.Sprintf("Project created. ref_id=%s", refID)}
	}
}

func makeSpawnWorkerHandler(callback func(name, prompt, task string, enableBrowser, enableDesktop bool) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			Name           string `json:"name"`
			Prompt         string `json:"prompt"`
			Task           string `json:"task"`
			EnableBrowser  bool   `json:"enable_browser"`
			EnableDesktop  bool   `json:"enable_desktop"`
		}
		json.Unmarshal([]byte(args), &p)
		if callback == nil {
			return &session.ToolResult{Error: "spawn_worker not available in this session"}
		}
		result, err := callback(p.Name, p.Prompt, p.Task, p.EnableBrowser, p.EnableDesktop)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeTerminateWorkerHandler(callback func(workerName string) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			WorkerName string `json:"worker_name"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.WorkerName == "" {
			return &session.ToolResult{Error: "worker_name required"}
		}
		if callback == nil {
			return &session.ToolResult{Error: "terminate_worker not available in this session"}
		}
		result, err := callback(p.WorkerName)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeDeleteWorkerHandler(callback func(workerName string) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			WorkerName string `json:"worker_name"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.WorkerName == "" {
			return &session.ToolResult{Error: "worker_name required"}
		}
		if callback == nil {
			return &session.ToolResult{Error: "delete_worker not available in this session"}
		}
		result, err := callback(p.WorkerName)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeInterveneWorkerHandler(callback func(workerName, message string) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			WorkerName string `json:"worker_name"`
			Message    string `json:"message"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.WorkerName == "" || p.Message == "" {
			return &session.ToolResult{Error: "worker_name and message required"}
		}
		if callback == nil {
			return &session.ToolResult{Error: "intervene_worker not available in this session"}
		}
		result, err := callback(p.WorkerName, p.Message)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeListWorkersHandler(callback func() (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		result, err := callback()
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func makeInterveneProjectHandler(callback func(refID, message string) (string, error)) func(ctx context.Context, args string) *session.ToolResult {
	return func(ctx context.Context, args string) *session.ToolResult {
		var p struct {
			RefID   string `json:"ref_id"`
			Message string `json:"message"`
		}
		json.Unmarshal([]byte(args), &p)
		if p.RefID == "" || p.Message == "" {
			return &session.ToolResult{Error: "ref_id and message required"}
		}
		if callback == nil {
			return &session.ToolResult{Error: "intervene_project not available in this session"}
		}
		result, err := callback(p.RefID, p.Message)
		if err != nil {
			return &session.ToolResult{Error: err.Error()}
		}
		return &session.ToolResult{Content: result}
	}
}

func searchFiles(root, pattern, query string, ctxLines int) ([]string, error) {
	var results []string
	query = strings.ToLower(query)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		match, _ := filepath.Match(pattern, filepath.Base(path))
		if !match {
			match, _ = filepath.Match("*."+pattern, filepath.Base(path))
		}
		if match {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				if strings.Contains(strings.ToLower(line), query) {
					if ctxLines > 0 {
						start := i - ctxLines
						if start < 0 {
							start = 0
						}
						end := i + ctxLines + 1
						if end > len(lines) {
							end = len(lines)
						}
						for j := start; j < end; j++ {
							marker := "  "
							if j == i {
								marker = "> "
							}
							results = append(results, fmt.Sprintf("%s:%d:%s%s", path, j+1, marker, lines[j]))
						}
						results = append(results, "---")
					} else {
						results = append(results, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
					}
				}
			}
		}
		return nil
	})
	return results, nil
}

var imageExts = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".gif":  "image/gif",
	".webp": "image/webp",
	".bmp":  "image/bmp",
}

func detectImageMIME(data []byte, ext string) string {
	if len(data) < 4 {
		return ""
	}
	lower := strings.ToLower(ext)
	switch {
	case data[0] == 0x89 && data[1] == 0x50 && data[2] == 0x4E && data[3] == 0x47:
		return "image/png"
	case data[0] == 0xFF && data[1] == 0xD8 && data[2] == 0xFF:
		return "image/jpeg"
	case data[0] == 'G' && data[1] == 'I' && data[2] == 'F' && data[3] == '8':
		return "image/gif"
	case data[0] == 'B' && data[1] == 'M':
		return "image/bmp"
	case data[0] == 'R' && data[1] == 'I' && data[2] == 'F' && data[3] == 'F' && len(data) > 11 &&
		data[8] == 'W' && data[9] == 'E' && data[10] == 'B' && data[11] == 'P':
		return "image/webp"
	}
	if mime, ok := imageExts[lower]; ok {
		return mime
	}
	return ""
}

func formatSizeForImage(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
}

var showHTMLTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "show_html",
		Description: "Show HTML content in a floating card on the desktop. Returns html_id for later close_html calls. Use for displaying results, diagrams, or dashboards to the user.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "Card title"},
				"content": map[string]any{"type": "string", "description": "HTML content to display"},
				"width":   map[string]any{"type": "integer", "description": "Card width in pixels. Default 800."},
				"height":  map[string]any{"type": "integer", "description": "Card height in pixels. Default 600."},
			},
			"required": []string{"title", "content"},
		},
	},
}

var closeHTMLTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "close_html",
		Description: "Close an HTML display card by its ID (returned by show_html or list_htmls).",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"html_id": map[string]any{"type": "string", "description": "Display ID to close, e.g. 'content-1'"},
			},
			"required": []string{"html_id"},
		},
	},
}

var listHTMLsTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "list_htmls",
		Description: "List all open HTML display cards with their IDs, titles, and session IDs.",
	},
}

var focusSessionTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name:        "focus_session",
		Description: "Switch the desktop UI to a specific session or project tab, identified by session ID or project ref ID.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"ref_id": map[string]any{"type": "string", "description": "Session ID or project ref ID to switch to, e.g. 'main' or a project ref ID"},
			},
			"required": []string{"ref_id"},
		},
	},
}
