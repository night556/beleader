package tools

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func init() {
	register("web_search",
		"Search the web using DuckDuckGo and return results.",
		map[string]any{
			"query": map[string]any{"type": "string", "description": "Search query"},
		}, []string{"query"}, webSearchHandler)

	register("web_fetch",
		"Fetch content from a URL and return as text.",
		map[string]any{
			"url":  map[string]any{"type": "string", "description": "URL to fetch"},
			"json": map[string]any{"type": "boolean", "description": "Parse response as JSON"},
		}, []string{"url"}, webFetchHandler)

	register("run_http_request",
		"Make an HTTP request with custom method, headers, and body.",
		map[string]any{
			"url":     map[string]any{"type": "string", "description": "URL to request"},
			"method":  map[string]any{"type": "string", "description": "HTTP method (GET, POST, etc.)"},
			"headers": map[string]any{"type": "object", "description": "Request headers"},
			"body":    map[string]any{"type": "string", "description": "Request body"},
		}, []string{"url"}, runHTTPRequestHandler)
}

func webSearchHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct{ Query string `json:"query"` }
	json.Unmarshal([]byte(args), &p)
	if p.Query == "" {
		return &ToolResult{Error: "query is required"}
	}

	url := "https://html.duckduckgo.com/html/?q=" + strings.ReplaceAll(p.Query, " ", "+")
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &ToolResult{Error: "search failed: " + err.Error()}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 50000))
	content := string(body)
	results := scrapeDDG(content)
	if results == "" {
		return &ToolResult{Content: "No results found for: " + p.Query}
	}
	return &ToolResult{Content: results}
}

func scrapeDDG(html string) string {
	var b strings.Builder
	idx := 0
	count := 0
	for {
		start := strings.Index(html[idx:], `<a rel="nofollow" class="result__a"`)
		if start < 0 {
			break
		}
		idx += start
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
		if strings.Contains(url, "uddg=") {
			url = extractDDGUrl(url)
		}

		titleStart := hrefStart + hrefEnd + 1
		titleEnd := strings.Index(html[titleStart:], "</a>")
		if titleEnd < 0 {
			break
		}
		title := stripTags(html[titleStart : titleStart+titleEnd])

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

func webFetchHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		URL  string `json:"url"`
		JSON bool   `json:"json"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.URL == "" {
		return &ToolResult{Error: "url is required"}
	}

	req, _ := http.NewRequest("GET", p.URL, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &ToolResult{Error: "fetch failed: " + err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200000))
	content := string(body)

	if p.JSON {
		return &ToolResult{Content: content}
	}

	text := htmlToText(content)
	return &ToolResult{Content: text}
}

func runHTTPRequestHandler(args, workspace, workspaceRoot string, restrict bool, threadID string) *ToolResult {
	var p struct {
		URL     string            `json:"url"`
		Method  string            `json:"method"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	json.Unmarshal([]byte(args), &p)
	if p.URL == "" {
		return &ToolResult{Error: "url is required"}
	}
	if p.Method == "" {
		p.Method = "GET"
	}

	var bodyReader io.Reader
	if p.Body != "" {
		bodyReader = strings.NewReader(p.Body)
	}
	req, err := http.NewRequest(p.Method, p.URL, bodyReader)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	for k, v := range p.Headers {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &ToolResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 200000))
	result := map[string]any{
		"status_code": resp.StatusCode,
		"body":        string(body),
	}
	b, _ := json.Marshal(result)
	return &ToolResult{Content: string(b)}
}

// ── Helpers ──

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
	html = removeTag(html, "script")
	html = removeTag(html, "style")
	text := stripTags(html)
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
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
	decoded = strings.ReplaceAll(decoded, "%3A", ":")
	decoded = strings.ReplaceAll(decoded, "%2F", "/")
	decoded = strings.ReplaceAll(decoded, "%3F", "?")
	decoded = strings.ReplaceAll(decoded, "%3D", "=")
	decoded = strings.ReplaceAll(decoded, "%26", "&")
	return decoded
}
