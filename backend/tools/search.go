package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"beleader/backend/session"

	"github.com/go-rod/rod/lib/input"
	"github.com/go-rod/rod/lib/proto"
	"github.com/go-rod/stealth"
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
	bs, err := getOrCreateBrowser()
	if err != nil {
		return nil, fmt.Errorf("browser: %w", err)
	}

	page, err := stealth.Page(bs.browser)
	if err != nil {
		return nil, fmt.Errorf("create page: %w", err)
	}
	defer page.Close()

	page = page.Timeout(30 * time.Second).Context(ctx)

	if err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             1920,
		Height:            1080,
		DeviceScaleFactor: 1,
		Mobile:            false,
	}); err != nil {
		return nil, fmt.Errorf("viewport: %w", err)
	}

	// Open Bing homepage and use the search box like a real user.
	// Direct navigation to /search?q=... triggers Bing's rdr=1 redirect
	// and returns degraded results.
	if err := page.Navigate("https://cn.bing.com/"); err != nil {
		return nil, fmt.Errorf("navigate: %w", err)
	}
	if err := page.WaitLoad(); err != nil {
		return nil, fmt.Errorf("wait load: %w", err)
	}

	el, err := page.Element("#sb_form_q")
	if err != nil {
		return nil, fmt.Errorf("find search box: %w", err)
	}
	if err := el.Input(query); err != nil {
		return nil, fmt.Errorf("type query: %w", err)
	}
	if err := page.Keyboard.Press(input.Enter); err != nil {
		return nil, fmt.Errorf("press enter: %w", err)
	}

	page.MustElement("li.b_algo")

	html, err := page.HTML()
	if err != nil {
		return nil, fmt.Errorf("get html: %w", err)
	}

	return parseBingResults(html), nil
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
