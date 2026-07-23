package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"beleader/gateway/db"
	"beleader/gateway/engine"
)

// RegisterLocalTools registers all local (Gateway-side) tool handlers.
func RegisterLocalTools(r *Router) {
	r.RegisterLocal("web_search", webSearchHandler)
	r.RegisterLocal("web_fetch", webFetchHandler)
	r.RegisterLocal("run_http_request", runHTTPRequestHandler)
	r.RegisterLocal("read_status", readStatusHandler)
	r.RegisterLocal("update_status", updateStatusHandler)
}

func webSearchHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct{ Query string `json:"query"` }
	json.Unmarshal([]byte(args), &p)
	if p.Query == "" {
		return &engine.ToolResult{Error: "query is required"}
	}

	url := "https://api.bing.microsoft.com/v7.0/search?q=" + queryEscape(p.Query) + "&count=20"
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Ocp-Apim-Subscription-Key", "")

	// Fallback: use DuckDuckGo HTML
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 200 {
		return ddgSearch(ctx, p.Query)
	}
	defer resp.Body.Close()

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)

	var b strings.Builder
	if pages, ok := result["webPages"].(map[string]any); ok {
		if vals, ok := pages["value"].([]any); ok {
			for i, v := range vals {
				if i >= 10 {
					break
				}
				m := v.(map[string]any)
				b.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n",
					i+1,
					getStr(m, "name"),
					getStr(m, "url"),
					getStr(m, "snippet")))
			}
		}
	}
	if b.Len() == 0 {
		return ddgSearch(ctx, p.Query)
	}
	return &engine.ToolResult{Content: b.String()}
}

func ddgSearch(ctx context.Context, query string) *engine.ToolResult {
	url := "https://html.duckduckgo.com/html/?q=" + queryEscape(query)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &engine.ToolResult{Error: "search failed: " + err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 50000))
	// Simple HTML scrape
	content := string(body)
	results := scrapeDDG(content)
	if results == "" {
		return &engine.ToolResult{Content: "No results found for: " + query}
	}
	return &engine.ToolResult{Content: results}
}

func scrapeDDG(html string) string {
	var b strings.Builder
	// Extract result links and snippets
	idx := 0
	count := 0
	for {
		start := strings.Index(html[idx:], `<a rel="nofollow" class="result__a"`)
		if start < 0 {
			break
		}
		idx += start
		// Find URL
		hrefStart := strings.Index(html[idx:], "href=\"")
		if hrefStart < 0 {
			break
		}
		hrefStart += idx + 6
		hrefEnd := strings.Index(html[hrefStart:], "\"")
		if hrefEnd < 0 {
			break
		}
		url := html[hrefStart : hrefStart+hrefEnd]
		// Decode redirect URL
		if strings.Contains(url, "uddg=") {
			url = extractDDGUrl(url)
		}

		// Find title text
		titleStart := hrefStart + hrefEnd + 1
		titleEnd := strings.Index(html[titleStart:], "</a>")
		if titleEnd < 0 {
			break
		}
		title := stripTags(html[titleStart : titleStart+titleEnd])

		// Find snippet
		snippetStart := strings.Index(html[titleStart+titleEnd:], `<a class="result__snippet"`)
		snippet := ""
		if snippetStart >= 0 {
			snippetStart += titleStart + titleEnd
			snippetEnd := strings.Index(html[snippetStart:], "</a>")
			if snippetEnd > 0 {
				snippet = stripTags(html[snippetStart : snippetStart+snippetEnd])
			}
		}

		count++
		b.WriteString(fmt.Sprintf("%d. %s\n   %s\n   %s\n\n", count, title, url, snippet))
		idx = titleStart + titleEnd

		if count >= 10 {
			break
		}
	}
	return b.String()
}

func webFetchHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		URL  string `json:"url"`
		JSON bool   `json:"json"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.URL == "" {
		return &engine.ToolResult{Error: "url is required"}
	}

	req, _ := http.NewRequestWithContext(ctx, "GET", p.URL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &engine.ToolResult{Error: "fetch failed: " + err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200000))
	content := string(body)

	if p.JSON {
		return &engine.ToolResult{Content: content}
	}

	// Simple HTML to text
	text := htmlToText(content)
	return &engine.ToolResult{Content: text}
}

func runHTTPRequestHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.URL == "" {
		return &engine.ToolResult{Error: "url is required"}
	}
	if p.Method == "" {
		p.Method = "GET"
	}

	var bodyReader io.Reader
	if p.Body != "" {
		bodyReader = strings.NewReader(p.Body)
	}
	req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, bodyReader)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200000))
	result := map[string]any{
		"status_code": resp.StatusCode,
		"body":        string(body),
	}
	b, _ := json.Marshal(result)
	return &engine.ToolResult{Content: string(b)}
}

func readStatusHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	t, err := h_getThread(thread.ID)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	if t.StatusContent == "" {
		return &engine.ToolResult{Content: "No STATUS.md content found."}
	}
	return &engine.ToolResult{Content: t.StatusContent}
}

func updateStatusHandler(ctx context.Context, thread *db.Thread, args string) *engine.ToolResult {
	var p struct{ Content string `json:"content"` }
	json.Unmarshal([]byte(args), &p)
	if p.Content == "" {
		return &engine.ToolResult{Error: "content is required"}
	}
	if err := h_updateThreadStatus(thread.ID, p.Content); err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: "Status updated."}
}

// ── Helpers ──

func queryEscape(s string) string {
	return strings.ReplaceAll(s, " ", "+")
}

func getStr(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func stripTags(s string) string {
	var b strings.Builder
	inTag := false
	for _, c := range s {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}

func htmlToText(html string) string {
	// Remove scripts and styles
	html = removeTag(html, "script")
	html = removeTag(html, "style")
	// Strip all tags
	text := stripTags(html)
	// Decode common entities
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	// Collapse whitespace
	lines := strings.Split(text, "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func removeTag(html, tag string) string {
	for {
		start := strings.Index(strings.ToLower(html), "<"+tag)
		if start < 0 {
			break
		}
		end := strings.Index(html[start:], "</"+tag+">")
		if end < 0 {
			html = html[:start]
			break
		}
		html = html[:start] + html[start+end+len(tag)+3:]
	}
	return html
}

func extractDDGUrl(url string) string {
	start := strings.Index(url, "uddg=")
	if start < 0 {
		return url
	}
	start += 5
	end := strings.Index(url[start:], "&")
	if end < 0 {
		return url[start:]
	}
	decoded := url[start : start+end]
	// URL decode
	decoded = strings.ReplaceAll(decoded, "%3A", ":")
	decoded = strings.ReplaceAll(decoded, "%2F", "/")
	decoded = strings.ReplaceAll(decoded, "%3F", "?")
	decoded = strings.ReplaceAll(decoded, "%3D", "=")
	decoded = strings.ReplaceAll(decoded, "%26", "&")
	return decoded
}
