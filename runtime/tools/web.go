package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"beleader/runtime/engine"
)

var bingUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

var htmlTagRe = regexp.MustCompile(`<[^>]*>`)
var whitespaceRe = regexp.MustCompile(`\s+`)

func webFetchHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct{ URL string }
	json.Unmarshal([]byte(args), &p)
	if p.URL == "" {
		return &engine.ToolResult{Error: "url required"}
	}

	content, err := fetchURL(ctx, p.URL)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	return &engine.ToolResult{Content: content}
}

func httpRequestHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	json.Unmarshal([]byte(args), &p)

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

	if p.Headers != nil {
		for k, v := range p.Headers {
			req.Header.Set(k, v)
		}
	}
	req.Header.Set("User-Agent", "BeLeader/1.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 100*1024))

	var out strings.Builder
	fmt.Fprintf(&out, "Status: %d %s\n", resp.StatusCode, resp.Status)
	for k, v := range resp.Header {
		fmt.Fprintf(&out, "%s: %s\n", k, strings.Join(v, ", "))
	}
	fmt.Fprintf(&out, "\n%s", string(body))

	return &engine.ToolResult{Content: out.String()}
}

func fetchURL(ctx context.Context, urlStr string) (string, error) {
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "https://" + urlStr
	}

	req, err := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", bingUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("fetch returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 500*1024))
	if err != nil {
		return "", err
	}

	content := htmlToText(string(body))
	return content, nil
}

func htmlToText(html string) string {
	text := htmlTagRe.ReplaceAllString(html, " ")
	text = whitespaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// RegisterWebTools registers web search, fetch, and HTTP request tools.
func RegisterWebTools(eng *engine.Engine) {
	eng.RegisterTool("web_search", webSearchHandler)
	eng.RegisterTool("web_fetch", webFetchHandler)
	eng.RegisterTool("run_http_request", httpRequestHandler)
}
