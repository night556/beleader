package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"beleader/runtime/engine"
)

// resolvePath joins the given path with the workspace directory from context.
// If the path is already absolute (per the host OS), it is used as-is.
// When RestrictWorkspace is set, absolute paths outside the workspace are rejected.
func resolvePath(ctx context.Context, p string) (string, error) {
	if filepath.IsAbs(p) {
		wd, _ := ctx.Value(engine.CtxKeyWorkDir).(string)
		if restrict, _ := ctx.Value(engine.CtxKeyRestrictWorkspace).(bool); restrict && wd != "" {
			clean := filepath.Clean(p)
			cleanWd := filepath.Clean(wd)
			if !strings.HasPrefix(clean, cleanWd+string(filepath.Separator)) && clean != cleanWd {
				return "", fmt.Errorf("access denied: path is outside workspace (%s)", wd)
			}
		}
		return p, nil
	}
	if wd, _ := ctx.Value(engine.CtxKeyWorkDir).(string); wd != "" {
		return filepath.Join(wd, p), nil
	}
	return p, nil
}

// ── File tool handlers ──

func readFileHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
		Limit  int    `json:"limit"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &engine.ToolResult{Error: "path is required"}
	}
	var err error
	p.Path, err = resolvePath(ctx, p.Path)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}

	mime := detectImageMIME(data, filepath.Ext(p.Path))
	vision, _ := ctx.Value(engine.CtxKeyVisionEnabled).(bool)
	if vision && mime != "" {
		data = compressImage(data, 1920, 75)
		b64 := encodeBase64(data)
		uri := fmt.Sprintf("data:%s;base64,%s", mime, b64)
		dims := ""
		if cfg, _, err := decodeImageConfig(data); err == nil {
			dims = fmt.Sprintf(", %dx%d", cfg.Width, cfg.Height)
		}
		fname := filepath.Base(p.Path)
		return &engine.ToolResult{
			Content: fmt.Sprintf("Image: %s (%s%s)", fname, formatSizeForImage(len(data)), dims),
			Images:  []string{uri},
		}
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

	return &engine.ToolResult{Content: string(data)}
}

func readDirHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct{ Path string }
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &engine.ToolResult{Error: "path is required"}
	}
	var err error
	p.Path, err = resolvePath(ctx, p.Path)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	entries, err := os.ReadDir(p.Path)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	var lines []string
	for _, e := range entries {
		t := "file"
		if e.IsDir() {
			t = "dir"
		}
		lines = append(lines, fmt.Sprintf("%s  [%s]", e.Name(), t))
	}
	return &engine.ToolResult{Content: strings.Join(lines, "\n")}
}

func searchContentHandler(ctx context.Context, args string) *engine.ToolResult {
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
		return &engine.ToolResult{Error: "pattern is required"}
	}
	searchPath := p.Path
	if searchPath == "" {
		searchPath, _ = ctx.Value(engine.CtxKeyWorkDir).(string)
	} else {
		var err error
		searchPath, err = resolvePath(ctx, searchPath)
		if err != nil {
			return &engine.ToolResult{Error: err.Error()}
		}
	}
	if fi, err := os.Stat(searchPath); err != nil {
		return &engine.ToolResult{Error: fmt.Sprintf("path not found: %s", searchPath)}
	} else if fi.IsDir() {
		return &engine.ToolResult{Error: fmt.Sprintf("path must be a file, not a directory: %s. Use search_files to find files, then search_content on specific files.", searchPath)}
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
		return &engine.ToolResult{Content: "No matches found."}
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
	return &engine.ToolResult{Content: out.String()}
}

func searchFilesHandler(ctx context.Context, args string) *engine.ToolResult {
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
		searchPath, _ = ctx.Value(engine.CtxKeyWorkDir).(string)
	} else {
		var err error
		searchPath, err = resolvePath(ctx, searchPath)
		if err != nil {
			return &engine.ToolResult{Error: err.Error()}
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
		return &engine.ToolResult{Content: fmt.Sprintf("No files matching '%s' in %s", p.Pattern, searchPath)}
	}
	if len(matches) > 200 {
		remaining := len(matches) - 200
		matches = matches[:200]
		return &engine.ToolResult{Content: strings.Join(matches, "\n") + fmt.Sprintf("\n\n... and %d more (truncated at 200). Refine pattern.", remaining)}
	}
	return &engine.ToolResult{Content: strings.Join(matches, "\n")}
}

func writeFileHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &engine.ToolResult{Error: "path is required"}
	}
	var err error
	p.Path, err = resolvePath(ctx, p.Path)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	dir := filepath.Dir(p.Path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	if err := os.WriteFile(p.Path, []byte(p.Content), 0644); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("File written: %s (%d bytes)", p.Path, len(p.Content))}
}

func editFileHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.Path == "" {
		return &engine.ToolResult{Error: "path is required"}
	}
	if p.OldString == "" {
		return &engine.ToolResult{Error: "old_string is required"}
	}
	var err error
	p.Path, err = resolvePath(ctx, p.Path)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	data, err := os.ReadFile(p.Path)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	content := string(data)
	count := strings.Count(content, p.OldString)
	if count == 0 {
		return &engine.ToolResult{Error: "old_string not found in file"}
	}
	if count > 1 {
		return &engine.ToolResult{Error: fmt.Sprintf("old_string found %d times — provide more context to make it unique", count)}
	}
	newContent := strings.Replace(content, p.OldString, p.NewString, 1)
	if err := os.WriteFile(p.Path, []byte(newContent), 0644); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: fmt.Sprintf("File edited: %s", p.Path)}
}

func deleteFileHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Paths []string `json:"paths"`
	}
	json.Unmarshal([]byte(args), &p)
	if len(p.Paths) == 0 {
		return &engine.ToolResult{Error: "paths is required (array of absolute paths)"}
	}

	threadDir, _ := ctx.Value(engine.CtxKeyThreadDir).(string)
	if threadDir == "" {
		return &engine.ToolResult{Error: "thread_dir not set in context"}
	}
	ts := time.Now().Format("20060102_150405")
	trashDir := filepath.Join(threadDir, ".trash", ts)
	if err := os.MkdirAll(trashDir, 0755); err != nil {
		return &engine.ToolResult{Error: fmt.Sprintf("create trash dir: %v", err)}
	}

	var moved, skipped, failed []string
	for _, rawPath := range p.Paths {
		path, err := resolvePath(ctx, filepath.Clean(rawPath))
		if err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", rawPath, err))
			continue
		}
		if path == "" {
			skipped = append(skipped, "(empty)")
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
	if len(skipped) > 0 {
		lines = append(lines, "Skipped:")
		lines = append(lines, skipped...)
	}
	if len(failed) > 0 {
		lines = append(lines, "Failed:")
		lines = append(lines, failed...)
	}
	return &engine.ToolResult{Content: strings.Join(lines, "\n")}
}

// RegisterFileTools registers all file operation tools.
func RegisterFileTools(eng *engine.Engine) {
	eng.RegisterTool("read_file", readFileHandler)
	eng.RegisterTool("read_dir", readDirHandler)
	eng.RegisterTool("search_content", searchContentHandler)
	eng.RegisterTool("search_files", searchFilesHandler)
	eng.RegisterTool("write_file", writeFileHandler)
	eng.RegisterTool("edit_file", editFileHandler)
	eng.RegisterTool("delete_file", deleteFileHandler)
}

// searchFiles searches for a pattern in files matching a glob in a directory.
func searchFiles(root, pattern, query string, ctxLines int) ([]string, error) {
	const maxResults = 500
	var results []string
	query = strings.ToLower(query)

	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if len(results) >= maxResults {
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
				if len(results) >= maxResults {
					break
				}
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
