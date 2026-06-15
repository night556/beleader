package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"beleader/backend/session"
)

// ContentMeta tracks HTML display content for list_htmls queries.
type ContentMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	SessionID string `json:"session_id"`
}

var (
	contentMu       sync.RWMutex
	contentRegistry = map[string]ContentMeta{}
	contentSeq      int
)

// NotifyFunc broadcasts content events to observers.
type NotifyFunc func(eventType string, data map[string]any)

var notifyContent NotifyFunc

// SetContentNotifier sets the function used to broadcast content events.
func SetContentNotifier(fn NotifyFunc) {
	notifyContent = fn
}

// AddContent registers a content entry and returns its ID.
func AddContent(meta ContentMeta) string {
	contentMu.Lock()
	defer contentMu.Unlock()
	contentSeq++
	id := fmt.Sprintf("content-%d", contentSeq)
	meta.ID = id
	contentRegistry[id] = meta
	return id
}

// SetContent registers a content entry with a specific ID, replacing any existing entry with the same ID.
func SetContent(id string, meta ContentMeta) {
	contentMu.Lock()
	defer contentMu.Unlock()
	meta.ID = id
	contentRegistry[id] = meta
}

// RemoveContent removes a content entry.
func RemoveContent(id string) bool {
	contentMu.Lock()
	defer contentMu.Unlock()
	_, ok := contentRegistry[id]
	if ok {
		delete(contentRegistry, id)
	}
	return ok
}

// ListContent returns all registered content entries.
func ListContent() []ContentMeta {
	contentMu.RLock()
	defer contentMu.RUnlock()
	var list []ContentMeta
	for _, m := range contentRegistry {
		list = append(list, m)
	}
	return list
}

// SessionIDFromCtx extracts session ID from context, if set.
func SessionIDFromCtx(ctx context.Context) string {
	if v := ctx.Value(CtxSessionID); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// ctxKey is the context key type for session ID.
type ctxKey string

// CtxSessionID is the context key for the current session ID.
const CtxSessionID ctxKey = "session_id"

// RegisterHTMLTools registers the HTML display tools.
func RegisterHTMLTools(mgr *session.Manager) {
	mgr.RegisterTool("show_html", showHTMLHandler)
	mgr.RegisterTool("close_html", closeHTMLHandler)
	mgr.RegisterTool("list_htmls", listHTMLsHandler)
	mgr.RegisterTool("focus_session", focusSessionHandler)
	mgr.RegisterTool("show_file", showFileHandler)
}

func showHTMLHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct {
		Title   string `json:"title"`
		Content string `json:"content"`
		Path    string `json:"path"`
		Width   int    `json:"width"`
		Height  int    `json:"height"`
	}
	json.Unmarshal([]byte(args), &p)

	var cleanPath string
	if p.Path != "" {
		cleanPath = filepath.Clean(p.Path)
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return &session.ToolResult{Error: fmt.Sprintf("cannot read file: %v", err)}
		}
		p.Content = string(data)
	}
	if p.Content == "" {
		return &session.ToolResult{Error: "content is required"}
	}

	sid := SessionIDFromCtx(ctx)
	var id string
	if cleanPath != "" {
		id = fmt.Sprintf("file-%x", sha256.Sum256([]byte(cleanPath)))
		SetContent(id, ContentMeta{Title: p.Title, SessionID: sid})
	} else {
		id = AddContent(ContentMeta{Title: p.Title, SessionID: sid})
	}

	// Pass source for source/render toggle when showing a file
	var htmlSource string
	var isHTMLFile bool
	if cleanPath != "" {
		htmlSource = p.Content
		isHTMLFile = strings.HasSuffix(strings.ToLower(cleanPath), ".html") || strings.HasSuffix(strings.ToLower(cleanPath), ".htm")
	}

	if notifyContent != nil {
		notifyContent("content_created", map[string]any{
			"id":           id,
			"title":        p.Title,
			"html":         p.Content,
			"html_source":  htmlSource,
			"is_html_file": isHTMLFile,
			"file_path":    cleanPath,
			"session_id":   sid,
			"width":        p.Width,
			"height":       p.Height,
		})
	}

	return &session.ToolResult{Content: fmt.Sprintf("HTML display opened: id=%s title=%s", id, p.Title)}
}

func closeHTMLHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct {
		HTMLID string `json:"html_id"`
	}
	json.Unmarshal([]byte(args), &p)

	if p.HTMLID == "" {
		return &session.ToolResult{Error: "html_id required"}
	}

	if !RemoveContent(p.HTMLID) {
		return &session.ToolResult{Error: fmt.Sprintf("content %s not found", p.HTMLID)}
	}

	if notifyContent != nil {
		notifyContent("content_removed", map[string]any{
			"id": p.HTMLID,
		})
	}

	return &session.ToolResult{Content: fmt.Sprintf("HTML display closed: %s", p.HTMLID)}
}

func listHTMLsHandler(ctx context.Context, args string) *session.ToolResult {
	list := ListContent()
	if len(list) == 0 {
		return &session.ToolResult{Content: "No HTML displays open."}
	}
	var lines []string
	for _, m := range list {
		lines = append(lines, fmt.Sprintf("id=%s title=%s session=%s", m.ID, m.Title, m.SessionID))
	}
	return &session.ToolResult{Content: fmt.Sprintf("%d display(s):\n%s", len(list), strings.Join(lines, "\n"))}
}

func focusSessionHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct {
		RefID string `json:"ref_id"`
	}
	json.Unmarshal([]byte(args), &p)

	if p.RefID == "" {
		return &session.ToolResult{Error: "ref_id required"}
	}

	if notifyContent != nil {
		notifyContent("session_focused", map[string]any{
			"session_id": p.RefID,
		})
	}

	return &session.ToolResult{Content: fmt.Sprintf("Focused session: %s", p.RefID)}
}
