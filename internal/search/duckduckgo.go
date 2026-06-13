// Package search provides lightweight web search via DuckDuckGo HTML scraping
// and readable-text extraction from arbitrary URLs. Both are T0 capabilities
// (read-only) so they auto-allow and work well in saver mode.
package search

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

var httpClient = &http.Client{
	Timeout: 15 * time.Second,
	// Don't follow too many redirects.
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return nil
	},
}

// Result is one search result.
type Result struct {
	Title   string
	URL     string
	Snippet string
}

// Search queries DuckDuckGo HTML (lite endpoint) and returns up to maxResults
// results. Respects a 1 s delay between calls (caller responsibility).
func Search(query string, maxResults int) ([]Result, error) {
	if maxResults <= 0 {
		maxResults = 8
	}
	endpoint := "https://html.duckduckgo.com/html/?q=" + url.QueryEscape(query)
	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; VelkroGo/1.0)")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search: HTTP %d", resp.StatusCode)
	}
	return parseDDG(resp.Body, maxResults)
}

func parseDDG(r io.Reader, max int) ([]Result, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var results []Result
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if len(results) >= max {
			return
		}
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "result__body") {
			r := extractResult(n)
			if r.Title != "" && r.URL != "" {
				results = append(results, r)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return results, nil
}

func extractResult(n *html.Node) Result {
	var r Result
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch {
			case n.Data == "a" && hasClass(n, "result__a"):
				r.Title = strings.TrimSpace(textContent(n))
				for _, a := range n.Attr {
					if a.Key == "href" {
						r.URL = resolveURL(a.Val)
					}
				}
			case n.Data == "a" && hasClass(n, "result__snippet"):
				r.Snippet = strings.TrimSpace(textContent(n))
			case hasClass(n, "result__snippet"):
				r.Snippet = strings.TrimSpace(textContent(n))
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return r
}

func hasClass(n *html.Node, cls string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == cls {
					return true
				}
			}
		}
	}
	return false
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// resolveURL extracts the real destination from DDG's redirect wrapper.
func resolveURL(raw string) string {
	if strings.HasPrefix(raw, "//duckduckgo.com/l/?uddg=") || strings.HasPrefix(raw, "/l/?uddg=") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return raw
		}
		if u := parsed.Query().Get("uddg"); u != "" {
			decoded, err := url.QueryUnescape(u)
			if err == nil {
				return decoded
			}
		}
	}
	if strings.HasPrefix(raw, "http") {
		return raw
	}
	return raw
}
