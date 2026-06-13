package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rufatronics/velkrogo/internal/registry"
	"github.com/rufatronics/velkrogo/internal/search"
)

// WebSearch is a T0 (read-only) tool: DuckDuckGo scrape, no browser needed.
type WebSearch struct{}

func (WebSearch) Name() string          { return "web_search" }
func (WebSearch) Tier() registry.Tier   { return registry.TierReadOnly }
func (WebSearch) World() registry.World { return registry.WorldShared }
func (WebSearch) Description() string {
	return "Search the web using DuckDuckGo and return titles, URLs, and snippets. No browser required."
}
func (WebSearch) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"Search query"},"max_results":{"type":"integer","description":"Max results (1-10, default 6)"}},"required":["query"]}`)
}
func (WebSearch) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Query      string `json:"query"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 6
	}
	results, err := search.Search(in.Query, in.MaxResults)
	if err != nil {
		return registry.Result{Content: err.Error(), IsError: true}, nil
	}
	var sb strings.Builder
	for i, r := range results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n   %s\n\n", i+1, r.Title, r.URL, r.Snippet)
	}
	if sb.Len() == 0 {
		return registry.Result{Content: "no results found"}, nil
	}
	return registry.Result{Content: sb.String()}, nil
}

// FetchPage is a T0 tool that fetches a URL and returns its readable text.
type FetchPage struct{}

func (FetchPage) Name() string          { return "fetch_page" }
func (FetchPage) Tier() registry.Tier   { return registry.TierReadOnly }
func (FetchPage) World() registry.World { return registry.WorldShared }
func (FetchPage) Description() string {
	return "Fetch a web page and return its readable text content (scripts and styles stripped)."
}
func (FetchPage) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"Full URL to fetch"}},"required":["url"]}`)
}
func (FetchPage) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	text, err := search.FetchText(in.URL)
	if err != nil {
		return registry.Result{Content: err.Error(), IsError: true}, nil
	}
	return registry.Result{Content: text}, nil
}
