package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"beleader/runtime/engine"

	"golang.org/x/net/html"
)

func webSearchHandler(ctx context.Context, args string) *engine.ToolResult {
	var p struct{ Query string }
	json.Unmarshal([]byte(args), &p)
	if p.Query == "" {
		return &engine.ToolResult{Error: "query required"}
	}

	results, err := searchBingHTTP(ctx, p.Query)
	if err != nil {
		results, err = searchDDG(ctx, p.Query)
	}
	if err != nil {
		return &engine.ToolResult{Error: err.Error()}
	}
	if len(results) == 0 {
		return &engine.ToolResult{Content: "No results found."}
	}

	var out strings.Builder
	for i, r := range results {
		fmt.Fprintf(&out, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return &engine.ToolResult{Content: strings.TrimSpace(out.String())}
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func searchBingHTTP(ctx context.Context, query string) ([]searchResult, error) {
	reqURL := "https://www.bing.com/search?" + url.Values{"q": {query}}.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", bingUA)
	httpReq.Header.Set("Accept", "text/html")
	httpReq.Header.Set("Accept-Language", "en-US,en;q=0.9")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("bing request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("bing returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024))
	if err != nil {
		return nil, err
	}

	results := parseBingHTML(string(body))
	if len(results) == 0 {
		return nil, fmt.Errorf("no results parsed from Bing")
	}
	return results, nil
}

func parseBingHTML(htmlContent string) []searchResult {
	var results []searchResult
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return results
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "li" {
			for _, attr := range n.Attr {
				if attr.Key == "class" && strings.Contains(attr.Val, "b_algo") {
					r := extractBingResult(n)
					if r.Title != "" && r.URL != "" {
						results = append(results, r)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	if len(results) > 10 {
		results = results[:10]
	}
	return results
}

func extractBingResult(li *html.Node) searchResult {
	var r searchResult
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "h2" {
			for a := n.FirstChild; a != nil; a = a.NextSibling {
				if a.Type == html.ElementNode && a.Data == "a" {
					for _, attr := range a.Attr {
						if attr.Key == "href" {
							r.URL = attr.Val
						}
					}
					r.Title = textContent(a)
				}
			}
		}
		if n.Type == html.ElementNode && n.Data == "p" && r.Snippet == "" {
			t := strings.TrimSpace(textContent(n))
			if len(t) > 20 {
				r.Snippet = t
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(li)
	return r
}

func searchDDG(ctx context.Context, query string) ([]searchResult, error) {
	reqURL := "https://lite.duckduckgo.com/lite/?" + url.Values{"q": {query}}.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", bingUA)
	httpReq.Header.Set("Accept", "text/html")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ddg request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("ddg returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 200*1024))
	if err != nil {
		return nil, err
	}

	results := parseDDGLite(string(body))
	if len(results) == 0 {
		return nil, fmt.Errorf("no results parsed from DuckDuckGo")
	}
	return results, nil
}

func parseDDGLite(htmlContent string) []searchResult {
	var results []searchResult
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return results
	}

	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" && strings.Contains(attr.Val, "uddg=") {
					href := extractDDGURL(attr.Val)
					if href == "" {
						continue
					}
					snippet := nextSiblingText(n)
					results = append(results, searchResult{
						Title:   strings.TrimSpace(textContent(n)),
						URL:     href,
						Snippet: strings.TrimSpace(snippet),
					})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	if len(results) > 10 {
		results = results[:10]
	}
	return results
}

func nextSiblingText(n *html.Node) string {
	for s := n.NextSibling; s != nil; s = s.NextSibling {
		if s.Type == html.ElementNode && (s.Data == "br" || s.Data == "span") {
			continue
		}
		t := strings.TrimSpace(textContent(s))
		if t != "" {
			return t
		}
	}
	return ""
}

func extractDDGURL(raw string) string {
	if strings.HasPrefix(raw, "//") {
		raw = "https:" + raw
	}
	if u, err := url.Parse(raw); err == nil {
		if uddg := u.Query().Get("uddg"); uddg != "" {
			return uddg
		}
	}
	return raw
}

func textContent(n *html.Node) string {
	var buf strings.Builder
	var collect func(*html.Node)
	collect = func(n *html.Node) {
		if n.Type == html.TextNode {
			buf.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			collect(c)
		}
	}
	collect(n)
	return buf.String()
}
