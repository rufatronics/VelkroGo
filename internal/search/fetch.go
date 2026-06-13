package search

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"golang.org/x/net/html"
)

// FetchText fetches a URL and returns the main readable text content, stripping
// scripts, styles, and navigation boilerplate. The output is suitable for
// passing directly to the model as an observation.
func FetchText(rawURL string) (string, error) {
	req, err := newGetRequest(rawURL)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch %s: %w", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("fetch %s: HTTP %d", rawURL, resp.StatusCode)
	}
	ct := resp.Header.Get("content-type")
	if !strings.Contains(ct, "text/html") && !strings.Contains(ct, "text/plain") {
		return "", fmt.Errorf("fetch %s: non-text content type %q", rawURL, ct)
	}
	limited := io.LimitReader(resp.Body, 512*1024) // 512 KB cap
	doc, err := html.Parse(limited)
	if err != nil {
		return "", err
	}
	text := extractText(doc)
	if len(text) > 8000 {
		text = text[:8000] + "\n…[truncated]"
	}
	return text, nil
}

func newGetRequest(rawURL string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; VelkroGo/1.0)")
	req.Header.Set("Accept", "text/html,text/plain")
	return req, nil
}

// extractText walks an HTML tree and returns clean readable text.
func extractText(doc *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node, bool)
	walk = func(n *html.Node, skip bool) {
		if n.Type == html.ElementNode {
			switch strings.ToLower(n.Data) {
			case "script", "style", "nav", "footer", "header", "aside", "noscript", "iframe":
				return
			}
		}
		if n.Type == html.TextNode && !skip {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				b.WriteString(t)
				b.WriteByte('\n')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, skip)
		}
	}
	walk(doc, false)
	// Collapse runs of blank lines.
	lines := strings.Split(b.String(), "\n")
	var out []string
	blank := 0
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			blank++
		} else {
			blank = 0
		}
		if blank <= 1 {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}
