package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"iamhuman/backend/session"

	"github.com/sashabaranov/go-openai"
)

var showFileTool = openai.Tool{
	Type: "function",
	Function: &openai.FunctionDefinition{
		Name: "show_file",
		Description: "Display a local file in a floating card on the desktop. Images, videos, audio, and PDFs render inline. HTML files render as live web pages by default (with a toggle to view source code). Code and text files display with syntax coloring. NOT for web URLs — use browser_automate for those.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":   map[string]any{"type": "string", "description": "Absolute path to the file on disk"},
				"width":  map[string]any{"type": "integer", "description": "Card width in pixels. Default 800."},
				"height": map[string]any{"type": "integer", "description": "Card height in pixels. Default 600."},
			},
			"required": []string{"path"},
		},
	},
}

func showFileHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct {
		Path   string `json:"path"`
		Width  int    `json:"width"`
		Height int    `json:"height"`
	}
	json.Unmarshal([]byte(args), &p)

	if p.Path == "" {
		return &session.ToolResult{Error: "path required"}
	}

	cleanPath := filepath.Clean(p.Path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return &session.ToolResult{Error: fmt.Sprintf("cannot open file: %v", err)}
	}
	if info.IsDir() {
		return &session.ToolResult{Error: fmt.Sprintf("path is a directory, not a file: %s", cleanPath)}
	}

	title := filepath.Base(cleanPath)
	ext := strings.ToLower(filepath.Ext(cleanPath))
	mediaType := detectMediaType(ext)

	encodedPath := url.QueryEscape(cleanPath)
	viewURL := "/api/files/view?path=" + encodedPath

	sizeStr := formatSize(info.Size())
	var doc string
	var htmlSource string
	switch {
	case isImageExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;background:transparent}
img{max-width:100%%;max-height:100vh;object-fit:contain;border-radius:4px}
</style></head><body><img src="%s" alt="%s"></body></html>`, viewURL, html.EscapeString(title))

	case isVideoExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;background:transparent}
video{max-width:100%%;max-height:100vh;border-radius:4px;outline:none}
</style></head><body><video controls autoplay><source src="%s" type="%s"></video></body></html>`, viewURL, mediaType)

	case isAudioExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;display:flex;align-items:center;justify-content:center;min-height:100vh;background:transparent;font-family:system-ui,sans-serif}
audio{width:80%%;max-width:480px;outline:none}
</style></head><body><audio controls autoplay><source src="%s" type="%s"></audio></body></html>`, viewURL, mediaType)

	case isPDFExt(ext), isHTMLExt(ext):
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;overflow:hidden}
iframe{width:100%%;height:100vh;border:none}
</style></head><body><iframe src="%s"></iframe></body></html>`, viewURL)
		if isHTMLExt(ext) {
			if data, err := os.ReadFile(cleanPath); err == nil {
				htmlSource = string(data)
			}
		}

	case isTextExt(ext):
		if info.Size() >= 200*1024 {
			// Large text: serve via iframe for native scrolling, no size limit
			doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{margin:0;overflow:hidden}
iframe{width:100%%;height:100vh;border:none}
</style></head><body><iframe src="%s"></iframe></body></html>`, viewURL)
		} else {
			data, err := os.ReadFile(cleanPath)
			if err != nil {
				return &session.ToolResult{Error: fmt.Sprintf("cannot read file: %v", err)}
			}
			doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{font-family:'JetBrains Mono','Cascadia Code','Fira Code',monospace;font-size:13px;color:#e0d9f5;background:transparent;margin:0;padding:16px;line-height:1.65;white-space:pre-wrap;word-break:break-word;overflow-y:auto;scrollbar-width:thin;scrollbar-color:rgba(167,139,250,0.35) transparent}
::-webkit-scrollbar{width:5px;height:5px}
::-webkit-scrollbar-track{background:transparent}
::-webkit-scrollbar-thumb{background:rgba(167,139,250,0.35);border-radius:3px}
::-webkit-scrollbar-thumb:hover{background:rgba(167,139,250,0.55)}
*{background-color:transparent!important}
</style></head><body>%s</body></html>`, html.EscapeString(string(data)))
		}

	default:
		doc = fmt.Sprintf(`<!DOCTYPE html><html><head><meta charset="UTF-8"><style>
body{font-family:system-ui,sans-serif;font-size:14px;color:#e0d9f5;background:transparent;margin:0;padding:24px;line-height:1.8;display:flex;align-items:center;justify-content:center;min-height:100vh}
.card{text-align:center}
.icon{font-size:48px;margin-bottom:12px}
.name{font-size:18px;font-weight:600;margin-bottom:6px}
.meta{color:#9b8ec4;font-size:13px}
</style></head><body><div class="card"><div class="icon">&#128196;</div><div class="name">%s</div><div class="meta">%s · %s</div><div class="meta" style="margin-top:4px">Cannot preview this file type</div></div></body></html>`,
			html.EscapeString(title), sizeStr, mediaType)
	}

	sid := SessionIDFromCtx(ctx)
	id := AddContent(ContentMeta{Title: title, SessionID: sid})

	if notifyContent != nil {
		notifyContent("content_created", map[string]any{
			"id":         id,
			"title":      title,
			"html":       doc,
			"html_source": htmlSource,
			"file_path":    cleanPath,
			"is_html_file": ext == ".html" || ext == ".htm",
			"session_id": sid,
			"width":      p.Width,
			"height":     p.Height,
		})
	}

	return &session.ToolResult{Content: fmt.Sprintf("Opened file: %s (%s)", title, sizeStr)}
}

// FileViewHandler serves local files for content card iframes.
func FileViewHandler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "path required", http.StatusBadRequest)
		return
	}

	cleanPath := filepath.Clean(path)
	info, err := os.Stat(cleanPath)
	if err != nil {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	if info.IsDir() {
		http.Error(w, "not a file", http.StatusBadRequest)
		return
	}

	ext := strings.ToLower(filepath.Ext(cleanPath))
	contentType := detectMediaType(ext)
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Cache-Control", "max-age=60")
	http.ServeFile(w, r, cleanPath)
}

func detectMediaType(ext string) string {
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	case ".bmp":
		return "image/bmp"
	case ".ico":
		return "image/x-icon"
	case ".mp4":
		return "video/mp4"
	case ".webm":
		return "video/webm"
	case ".mov":
		return "video/quicktime"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".ogg", ".oga":
		return "audio/ogg"
	case ".flac":
		return "audio/flac"
	case ".pdf":
		return "application/pdf"
	case ".json":
		return "application/json"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "text/javascript"
	case ".ts":
		return "text/typescript"
	case ".md":
		return "text/markdown"
	case ".csv":
		return "text/csv"
	case ".yaml", ".yml":
		return "text/yaml"
	default:
		if isTextExt(ext) {
			return "text/plain; charset=utf-8"
		}
		return "application/octet-stream"
	}
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".ico":
		return true
	}
	return false
}

func isVideoExt(ext string) bool {
	switch ext {
	case ".mp4", ".webm", ".mov", ".avi", ".mkv":
		return true
	}
	return false
}

func isAudioExt(ext string) bool {
	switch ext {
	case ".mp3", ".wav", ".ogg", ".flac", ".aac", ".m4a", ".wma":
		return true
	}
	return false
}

func isPDFExt(ext string) bool {
	return ext == ".pdf"
}

func isHTMLExt(ext string) bool {
	return ext == ".html" || ext == ".htm"
}

func isTextExt(ext string) bool {
	switch ext {
	case ".txt", ".md", ".json", ".xml", ".yaml", ".yml", ".toml", ".ini", ".cfg",
		".csv", ".log", ".env", ".proto", ".sql", ".cue",
		".py", ".js", ".ts", ".tsx", ".jsx", ".go", ".rs", ".java", ".kt", ".swift",
		".c", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".hh", ".hxx",
		".css", ".scss", ".less", ".html", ".htm", ".vue", ".svelte",
		".sh", ".bash", ".zsh", ".bat", ".ps1", ".fish",
		".makefile", ".dockerfile", ".gitignore", ".editorconfig",
		".r", ".rb", ".php", ".pl", ".lua", ".zig", ".nim", ".ex", ".exs":
		return true
	}
	return false
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
