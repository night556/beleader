package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"beleader/backend/session"
)

// ── Standalone handlers for static tools ──
// These read workDir from context (set by runSession) instead of closure-capture.

func readDirHandler(ctx context.Context, args string) *session.ToolResult {
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
}

func searchContentHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct {
		Pattern      string `json:"pattern"`
		FilePattern  string `json:"file_pattern"`
		Path         string `json:"path"`
		ContextLines int    `json:"context_lines"`
		Offset       int    `json:"offset"`
		Limit        int    `json:"limit"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Pattern == "" {
		return &session.ToolResult{Error: "pattern is required"}
	}
	searchPath := p.Path
	if searchPath == "" {
		searchPath, _ = ctx.Value(session.CtxKeyWorkDir).(string)
	}
	if fi, err := os.Stat(searchPath); err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("path not found: %s", searchPath)}
	} else if fi.IsDir() {
		return &session.ToolResult{Error: fmt.Sprintf("path must be a file, not a directory: %s. Use search_files to find files, then search_content on specific files.", searchPath)}
	}
	pattern := p.FilePattern
	if pattern == "" {
		pattern = "*"
	}
	if p.ContextLines > 10 {
		p.ContextLines = 10
	}
	if p.Limit <= 0 {
		p.Limit = 50
	} else if p.Limit > 200 {
		p.Limit = 200
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	allResults, _ := searchFiles(searchPath, pattern, p.Pattern, p.ContextLines)
	total := len(allResults)
	if total == 0 {
		return &session.ToolResult{Content: "No matches found."}
	}
	start := p.Offset
	if start > total {
		start = total
	}
	end := start + p.Limit
	if end > total {
		end = total
	}
	page := allResults[start:end]
	var out strings.Builder
	fmt.Fprintf(&out, "Showing %d-%d of %d total matches.", start+1, end, total)
	if end < total {
		fmt.Fprintf(&out, " Use offset=%d for next page.", end)
	}
	fmt.Fprintf(&out, "\n\n%s", strings.Join(page, "\n"))
	return &session.ToolResult{Content: out.String()}
}

func searchFilesHandler(ctx context.Context, args string) *session.ToolResult {
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
		searchPath, _ = ctx.Value(session.CtxKeyWorkDir).(string)
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
}

func editFileHandler(ctx context.Context, args string) *session.ToolResult {
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
}

func readStatusHandler(ctx context.Context, args string) *session.ToolResult {
	workDir, _ := ctx.Value(session.CtxKeyWorkDir).(string)
	statusPath := filepath.Join(workDir, "STATUS.md")
	data, err := os.ReadFile(statusPath)
	if err != nil {
		return &session.ToolResult{Error: "STATUS.md not found: " + err.Error()}
	}
	return &session.ToolResult{Content: string(data)}
}

func updateStatusHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct{ Content string `json:"content"` }
	json.Unmarshal([]byte(args), &p)
	if p.Content == "" {
		return &session.ToolResult{Error: "content is required"}
	}
	workDir, _ := ctx.Value(session.CtxKeyWorkDir).(string)
	statusPath := filepath.Join(workDir, "STATUS.md")
	if err := os.MkdirAll(filepath.Dir(statusPath), 0755); err != nil {
		return &session.ToolResult{Error: err.Error()}
	}
	if err := os.WriteFile(statusPath, []byte(p.Content), 0644); err != nil {
		return &session.ToolResult{Error: err.Error()}
	}
	return &session.ToolResult{Content: "STATUS.md updated"}
}

// execHandlerWithWorkDir is a wrapper that sets execWorkDir from context before calling execHandler.
func execHandlerWithWorkDir(ctx context.Context, args string) *session.ToolResult {
	if wd, ok := ctx.Value(session.CtxKeyWorkDir).(string); ok && wd != "" {
		setExecWorkDir(wd)
	}
	return execHandler(ctx, args)
}

// ── RegisterBuiltinTools registers all builtin tools into the global Registry. ──

func RegisterBuiltinTools() {
	// File tools
	RegisterBuiltin("read_file", readFileTool, readFileHandler, "Read a file at the given absolute path. Read files before editing.")
	RegisterBuiltin("read_dir", readDirTool, readDirHandler, "List files and subdirectories at the given absolute path.")
	RegisterBuiltin("write_file", writeFileTool, writeFileHandler, "Create or overwrite a file. Prefer edit_file for small changes.")
	RegisterBuiltin("edit_file", editFileTool, editFileHandler, "Apply targeted edits to an existing file.")
	RegisterBuiltin("search_content", searchContentTool, searchContentHandler, "Search for a keyword or pattern across files with pagination.")
	RegisterBuiltin("search_files", searchFilesTool, searchFilesHandler, "Find files matching a glob pattern.")

	// Status tools
	RegisterBuiltin("read_status", readStatusTool, readStatusHandler, "Read the project STATUS.md file.")
	RegisterBuiltin("update_status", updateStatusTool, updateStatusHandler, "Update STATUS.md, preserving existing content and adding recent activity.")

	// Execution tools
	RegisterBuiltin("run_command", runCommandTool, execHandlerWithWorkDir, "Execute a shell command (sync or background).")
	RegisterBuiltin("run_http_request", runHTTPRequestTool, httpRequestHandler, "Send an HTTP request for testing APIs or fetching data.")

	// Web tools
	RegisterBuiltin("web_search", webSearchTool, webSearchHandler, "Search the web for information, documentation, best practices.")
	RegisterBuiltin("web_fetch", webFetchTool, webFetchHandler, "Fetch and read content of a web page by URL.")

	// Browser tools
	for _, t := range BrowserTools() {
		name := t.Function.Name
		desc := t.Function.Description
		if desc == "" {
			desc = name
		}
		RegisterBuiltin(name, t, func(ctx context.Context, args string) *session.ToolResult {
			return browserDispatch(name, args)
		}, desc)
	}
}
