package tools

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func readFileHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &ToolResult{Error: "path is required"}
	}
	var err error
	p.Path, err = resolvePath(p.Path, workspace, restrict)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	info, err := os.Stat(p.Path)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	if info.IsDir() {
		return &ToolResult{Error: "is a directory, use read_dir instead"}
	}
	const maxSize = 2 * 1024 * 1024
	if info.Size() > maxSize {
		return &ToolResult{Error: fmt.Sprintf("file too large (%s), use offset/limit", formatFileSize(info.Size()))}
	}

	data, err := os.ReadFile(p.Path)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}

	if isBinary(data) {
		return &ToolResult{Error: fmt.Sprintf("binary file (%s) — cannot display as text", formatFileSize(int64(len(data))))}
	}

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

	return &ToolResult{Content: string(data)}
}

func readDirHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct{ Path string `json:"path"` }
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &ToolResult{Error: "path is required"}
	}
	var err error
	p.Path, err = resolvePath(p.Path, workspace, restrict)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	entries, err := os.ReadDir(p.Path)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	var lines []string
	for _, e := range entries {
		t := "file"
		if e.IsDir() {
			t = "dir"
		}
		lines = append(lines, fmt.Sprintf("%s  [%s]", e.Name(), t))
	}
	return &ToolResult{Content: strings.Join(lines, "\n")}
}

func writeFileHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &ToolResult{Error: "path is required"}
	}
	var err error
	p.Path, err = resolvePath(p.Path, workspace, restrict)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	dir := filepath.Dir(p.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &ToolResult{Error: err.Error()}
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0644); err != nil {
		return &ToolResult{Error: err.Error()}
	}
	return &ToolResult{Content: fmt.Sprintf("File written: %s (%d bytes)", p.Path, len(p.Content))}
}

func editFileHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &ToolResult{Error: "path is required"}
	}
	if p.OldString == "" {
		return &ToolResult{Error: "old_string is required"}
	}
	var err error
	p.Path, err = resolvePath(p.Path, workspace, restrict)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	content := string(data)
	count := strings.Count(content, p.OldString)
	if count == 0 {
		return &ToolResult{Error: "old_string not found in file"}
	}
	if count > 1 {
		return &ToolResult{Error: fmt.Sprintf("old_string found %d times — provide more context to make it unique", count)}
	}
	newContent := strings.Replace(content, p.OldString, p.NewString, 1)
	if err := os.WriteFile(p.Path, []byte(newContent), 0644); err != nil {
		return &ToolResult{Error: err.Error()}
	}
	return &ToolResult{Content: fmt.Sprintf("File edited: %s", p.Path)}
}

func deleteFileHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		Paths []string `json:"paths"`
	}
	json.Unmarshal([]byte(args), &p)
	if len(p.Paths) == 0 {
		return &ToolResult{Error: "paths is required (array of paths)"}
	}

	threadDir := filepath.Join(workspaceRoot, "threads", threadID)
	ts := time.Now().Format("20060102_150405")
	trashDir := filepath.Join(threadDir, ".trash", ts)
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return &ToolResult{Error: fmt.Sprintf("create trash dir: %v", err)}
	}

	var moved, failed []string
	for _, rawPath := range p.Paths {
		path, err := resolvePath(filepath.Clean(rawPath), workspace, restrict)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", rawPath, err))
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		name := filepath.Base(path)
		dest := filepath.Join(trashDir, name)
		if err := os.Rename(path, dest); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		kind := "file"
		if info.IsDir() {
			kind = "dir"
		}
		moved = append(moved, fmt.Sprintf("[%s] %s", kind, path))
	}

	var lines []string
	if len(moved) > 0 {
		lines = append(lines, fmt.Sprintf("Moved %d item(s) to %s:", len(moved), trashDir))
		lines = append(lines, moved...)
	}
	if len(failed) > 0 {
		lines = append(lines, "Failed:")
		lines = append(lines, failed...)
	}
	return &ToolResult{Content: strings.Join(lines, "\n")}
}

func searchContentHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		Pattern      string `json:"pattern"`
		Path         string `json:"path"`
		ContextLines int    `json:"context_lines"`
		Offset       int    `json:"offset"`
		Limit        int    `json:"limit"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Pattern == "" {
		return &ToolResult{Error: "pattern is required"}
	}
	searchPath := p.Path
	if searchPath == "" {
		return &ToolResult{Error: "path is required (must be a single file)"}
	}
	var err error
	searchPath, err = resolvePath(searchPath, workspace, restrict)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	fi, err := os.Stat(searchPath)
	if err != nil {
		return &ToolResult{Error: fmt.Sprintf("path not found: %s", searchPath)}
	}
	if fi.IsDir() {
		return &ToolResult{Error: fmt.Sprintf("path must be a file, not a directory: %s", searchPath)}
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

	f, err := os.Open(searchPath)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	defer f.Close()

	query := strings.ToLower(p.Pattern)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var allResults []string
	lineNum := 0
	// Ring buffer for context lines before match
	var ctxBefore []string
	ctxCap := p.ContextLines

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		if isBinaryLine([]byte(line)) {
			return &ToolResult{Error: "binary file — cannot search as text"}
		}

		if strings.Contains(strings.ToLower(line), query) {
			if p.ContextLines > 0 {
				// Output context before
				startLine := lineNum - len(ctxBefore)
				for j, ctxLine := range ctxBefore {
					allResults = append(allResults, fmt.Sprintf("%s:%d:  %s", searchPath, startLine+j, ctxLine))
				}
				// Matched line
				allResults = append(allResults, fmt.Sprintf("%s:%d:> %s", searchPath, lineNum, line))
				// Read context after
				for k := 0; k < p.ContextLines; k++ {
					if !scanner.Scan() {
						break
					}
					lineNum++
					allResults = append(allResults, fmt.Sprintf("%s:%d:  %s", searchPath, lineNum, scanner.Text()))
				}
				allResults = append(allResults, "---")
			} else {
				allResults = append(allResults, fmt.Sprintf("%s:%d: %s", searchPath, lineNum, strings.TrimSpace(line)))
			}
		}

		// Update context ring buffer
		if ctxCap > 0 {
			if len(ctxBefore) >= ctxCap {
				ctxBefore = ctxBefore[1:]
			}
			ctxBefore = append(ctxBefore, line)
		}
	}

	total := len(allResults)
	if total == 0 {
		return &ToolResult{Content: "No matches found."}
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
	return &ToolResult{Content: out.String()}
}

func searchFilesHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
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
		searchPath = workspace
	} else {
		var err error
		searchPath, err = resolvePath(searchPath, workspace, restrict)
		if err != nil {
			return &ToolResult{Error: err.Error()}
		}
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
		return &ToolResult{Content: fmt.Sprintf("No files matching '%s' in %s", p.Pattern, searchPath)}
	}
	if len(matches) > 200 {
		remaining := len(matches) - 200
		matches = matches[:200]
		return &ToolResult{Content: strings.Join(matches, "\n") + fmt.Sprintf("\n\n... and %d more (truncated at 200). Refine pattern.", remaining)}
	}
	return &ToolResult{Content: strings.Join(matches, "\n")}
}

// ── Helpers ──

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

// isBinaryLine checks if a single line contains null bytes.
func isBinaryLine(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

func formatFileSize(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%dB", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1fKB", float64(n)/1024)
	}
	return fmt.Sprintf("%.1fMB", float64(n)/(1024*1024))
}

func init() {
	register("read_file",
		"Read a file from the filesystem. Returns file contents as text.",
		map[string]any{
			"path":   map[string]any{"type": "string", "description": "Path to the file (relative to workspace, or absolute)."},
			"offset": map[string]any{"type": "integer", "description": "Line number to start reading from (1-based)."},
			"limit":  map[string]any{"type": "integer", "description": "Maximum number of lines to read."},
		}, []string{"path"}, readFileHandler)

	register("read_dir",
		"List contents of a directory.",
		map[string]any{
			"path": map[string]any{"type": "string", "description": "Path to the directory (relative to workspace, or absolute)."},
		}, []string{"path"}, readDirHandler)

	register("write_file",
		"Create or overwrite a file. Parent directories are created automatically.",
		map[string]any{
			"path":    map[string]any{"type": "string", "description": "Path to the file."},
			"content": map[string]any{"type": "string", "description": "File content."},
		}, []string{"path", "content"}, writeFileHandler)

	register("edit_file",
		"Replace a single occurrence of old_string with new_string in a file.",
		map[string]any{
			"path":       map[string]any{"type": "string", "description": "Path to the file."},
			"old_string": map[string]any{"type": "string", "description": "Exact text to replace."},
			"new_string": map[string]any{"type": "string", "description": "Replacement text."},
		}, []string{"path", "old_string", "new_string"}, editFileHandler)

	register("delete_file",
		"Move files/directories to a .trash directory.",
		map[string]any{
			"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Array of paths to delete."},
		}, []string{"paths"}, deleteFileHandler)

	register("search_content",
		"Search for a text pattern in a single file. Returns matching lines with optional context. Use search_files to find files by name first.",
		map[string]any{
			"pattern":       map[string]any{"type": "string", "description": "Text pattern to search for (case-insensitive)."},
			"path":          map[string]any{"type": "string", "description": "Path to the file to search."},
			"context_lines": map[string]any{"type": "integer", "description": "Lines of context around matches (max 10)."},
			"offset":        map[string]any{"type": "integer", "description": "Result offset for pagination."},
			"limit":         map[string]any{"type": "integer", "description": "Max results (max 200)."},
		}, []string{"pattern", "path"}, searchContentHandler)

	register("search_files",
		"Find files by glob pattern recursively.",
		map[string]any{
			"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. '*.go', '**/*.tsx')."},
			"path":    map[string]any{"type": "string", "description": "Directory to search in. Defaults to workspace."},
		}, []string{"pattern"}, searchFilesHandler)
}
