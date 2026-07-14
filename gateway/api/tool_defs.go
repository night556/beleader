package api

// Tool definitions sent to Runtime as JSON maps.
// These mirror the tool schemas from runtime/tools/tools.go.

func baseToolDefs() []map[string]any {
	return []map[string]any{
		{
			"name":        "read_file",
			"description": "Read a file from the filesystem. Returns file contents as text, or image data for image files when vision is enabled.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":   map[string]any{"type": "string", "description": "Absolute path to the file."},
					"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)."},
					"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "read_dir",
			"description": "List contents of a directory.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path": map[string]any{"type": "string", "description": "Absolute path to the directory."},
				},
				"required": []string{"path"},
			},
		},
		{
			"name":        "write_file",
			"description": "Write content to a file. Creates parent directories if needed.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string", "description": "Absolute path to the file."},
					"content": map[string]any{"type": "string", "description": "File content."},
				},
				"required": []string{"path", "content"},
			},
		},
		{
			"name":        "edit_file",
			"description": "Replace a single occurrence of old_string with new_string in a file.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":       map[string]any{"type": "string", "description": "Absolute path to the file."},
					"old_string": map[string]any{"type": "string", "description": "Exact text to replace."},
					"new_string": map[string]any{"type": "string", "description": "Replacement text."},
				},
				"required": []string{"path", "old_string", "new_string"},
			},
		},
		{
			"name":        "delete_file",
			"description": "Move files/directories to a .trash directory.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Array of absolute paths to delete."},
				},
				"required": []string{"paths"},
			},
		},
		{
			"name":        "search_content",
			"description": "Search for a pattern in file contents. path must be a single file, not a directory.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern":       map[string]any{"type": "string", "description": "Regex pattern to search for."},
					"file_pattern":  map[string]any{"type": "string", "description": "Glob pattern for file matching."},
					"path":          map[string]any{"type": "string", "description": "Absolute path to a file."},
					"context_lines": map[string]any{"type": "integer", "description": "Lines of context around matches (max 10)."},
					"offset":        map[string]any{"type": "integer", "description": "Result offset for pagination."},
					"limit":         map[string]any{"type": "integer", "description": "Max results (max 200)."},
				},
				"required": []string{"pattern"},
			},
		},
		{
			"name":        "search_files",
			"description": "Find files by glob pattern recursively.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. '*.go', '**/*.tsx')."},
					"path":    map[string]any{"type": "string", "description": "Directory to search in."},
				},
				"required": []string{"pattern"},
			},
		},
		{
			"name":        "read_status",
			"description": "Read the STATUS.md file from the project work directory.",
			"parameters": map[string]any{
				"type":       "object",
				"properties": map[string]any{},
			},
		},
		{
			"name":        "update_status",
			"description": "Update the STATUS.md file with new content.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"content": map[string]any{"type": "string", "description": "Full content for STATUS.md."},
				},
				"required": []string{"content"},
			},
		},
		{
			"name":        "run_command",
			"description": "Run a shell command and return its output.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"command":    map[string]any{"type": "string", "description": "Shell command to execute."},
					"work_dir":   map[string]any{"type": "string", "description": "Working directory for the command."},
					"background": map[string]any{"type": "boolean", "description": "Run in background."},
					"timeout":    map[string]any{"type": "integer", "description": "Timeout in seconds (max 600)."},
				},
				"required": []string{"command"},
			},
		},
		{
			"name":        "web_search",
			"description": "Search the web using Bing and return results.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string", "description": "Search query."},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "web_fetch",
			"description": "Fetch content from a URL and return as text.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":  map[string]any{"type": "string", "description": "URL to fetch."},
					"json": map[string]any{"type": "boolean", "description": "Parse response as JSON."},
				},
				"required": []string{"url"},
			},
		},
		{
			"name":        "run_http_request",
			"description": "Make an HTTP request with custom method, headers, and body.",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"url":     map[string]any{"type": "string", "description": "URL to request."},
					"method":  map[string]any{"type": "string", "description": "HTTP method (GET, POST, PUT, DELETE, etc.)."},
					"headers": map[string]any{"type": "object", "description": "Request headers."},
					"body":    map[string]any{"type": "string", "description": "Request body."},
				},
				"required": []string{"url"},
			},
		},
	}
}

// baseToolDefsFiltered returns base tool defs filtered by name.
func baseToolDefsFiltered(toolNames []string) []map[string]any {
	allDefs := baseToolDefs()
	if len(toolNames) == 0 {
		return allDefs
	}
	nameSet := make(map[string]bool, len(toolNames))
	for _, n := range toolNames {
		nameSet[n] = true
	}
	var filtered []map[string]any
	for _, d := range allDefs {
		if nameSet[d["name"].(string)] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}
