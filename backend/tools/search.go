package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"beleader/backend/session"

	"golang.org/x/net/html"
)

var bingUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

func webSearchHandler(ctx context.Context, args string) *session.ToolResult {
	var p struct{ Query string }
	json.Unmarshal([]byte(args), &p)
	if p.Query == "" {
		return &session.ToolResult{Error: "query required"}
	}
	results, err := searchBing(ctx, p.Query)
	if err != nil {
		return &session.ToolResult{Error: err.Error()}
	}
	if len(results) == 0 {
		return &session.ToolResult{Content: "No results found."}
	}
	var out strings.Builder
	for i, r := range results {
		fmt.Fprintf(&out, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	return &session.ToolResult{Content: strings.TrimSpace(out.String())}
}

type searchResult struct {
	Title   string
	URL     string
	Snippet string
}

func searchBing(ctx context.Context, query string) ([]searchResult, error) {
	u := "https://cn.bing.com/search?q=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", bingUA)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}

	return parseBingResults(string(body)), nil
}

func parseBingResults(htmlContent string) []searchResult {
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
					r := extractResult(n)
					if r.Title != "" && r.URL != "" {
						results = append(results, r)
					}
					return
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

func extractResult(li *html.Node) searchResult {
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
		if n.Data == "p" && r.Snippet == "" {
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
