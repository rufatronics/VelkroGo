package coder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/rufatronics/velkrogo/internal/registry"
)

func githubToken() string {
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN"} {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return ""
}

func githubReq(ctx context.Context, method, path string, body any) (map[string]any, error) {
	tok := githubToken()
	if tok == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN or GH_TOKEN not set")
	}
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.github.com"+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var result map[string]any
	_ = json.Unmarshal(b, &result)
	if resp.StatusCode >= 400 {
		msg, _ := result["message"].(string)
		return nil, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, msg)
	}
	return result, nil
}

// GitHubCreatePR creates a pull request.
type GitHubCreatePR struct{}

func (GitHubCreatePR) Name() string         { return "github_create_pr" }
func (GitHubCreatePR) Description() string  { return "Create a GitHub pull request." }
func (GitHubCreatePR) Tier() registry.Tier  { return registry.TierExternal }
func (GitHubCreatePR) World() registry.World { return registry.WorldCoder }
func (GitHubCreatePR) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"},"title":{"type":"string"},"body":{"type":"string"},"head":{"type":"string","description":"Branch to merge from"},"base":{"type":"string","description":"Branch to merge into, e.g. main"}},"required":["owner","repo","title","head","base"]}`)
}
func (GitHubCreatePR) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		Title string `json:"title"`
		Body  string `json:"body"`
		Head  string `json:"head"`
		Base  string `json:"base"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	res, err := githubReq(ctx, "POST", fmt.Sprintf("/repos/%s/%s/pulls", in.Owner, in.Repo), map[string]any{
		"title": in.Title, "body": in.Body, "head": in.Head, "base": in.Base,
	})
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	url, _ := res["html_url"].(string)
	num := res["number"]
	return registry.Result{Content: fmt.Sprintf("PR #%v created: %s", num, url)}, nil
}

// GitHubCreateIssue creates an issue.
type GitHubCreateIssue struct{}

func (GitHubCreateIssue) Name() string         { return "github_create_issue" }
func (GitHubCreateIssue) Description() string  { return "Create a GitHub issue." }
func (GitHubCreateIssue) Tier() registry.Tier  { return registry.TierExternal }
func (GitHubCreateIssue) World() registry.World { return registry.WorldCoder }
func (GitHubCreateIssue) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"},"title":{"type":"string"},"body":{"type":"string"},"labels":{"type":"array","items":{"type":"string"}}},"required":["owner","repo","title"]}`)
}
func (GitHubCreateIssue) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Owner  string   `json:"owner"`
		Repo   string   `json:"repo"`
		Title  string   `json:"title"`
		Body   string   `json:"body"`
		Labels []string `json:"labels"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	payload := map[string]any{"title": in.Title, "body": in.Body}
	if len(in.Labels) > 0 {
		payload["labels"] = in.Labels
	}
	res, err := githubReq(ctx, "POST", fmt.Sprintf("/repos/%s/%s/issues", in.Owner, in.Repo), payload)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	url, _ := res["html_url"].(string)
	num := res["number"]
	return registry.Result{Content: fmt.Sprintf("Issue #%v created: %s", num, url)}, nil
}

// GitHubListPRs lists pull requests.
type GitHubListPRs struct{}

func (GitHubListPRs) Name() string         { return "github_list_prs" }
func (GitHubListPRs) Description() string  { return "List pull requests in a GitHub repo." }
func (GitHubListPRs) Tier() registry.Tier  { return registry.TierReadOnly }
func (GitHubListPRs) World() registry.World { return registry.WorldCoder }
func (GitHubListPRs) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"},"state":{"type":"string","enum":["open","closed","all"]}},"required":["owner","repo"]}`)
}
func (GitHubListPRs) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
		State string `json:"state"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.State == "" {
		in.State = "open"
	}
	tok := githubToken()
	if tok == "" {
		return registry.Result{IsError: true, Content: "GITHUB_TOKEN not set"}, nil
	}
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls?state=%s&per_page=20", in.Owner, in.Repo, in.State)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var prs []map[string]any
	_ = json.Unmarshal(b, &prs)
	if len(prs) == 0 {
		return registry.Result{Content: "No " + in.State + " PRs."}, nil
	}
	var sb strings.Builder
	for _, pr := range prs {
		num := pr["number"]
		title, _ := pr["title"].(string)
		htmlURL, _ := pr["html_url"].(string)
		fmt.Fprintf(&sb, "#%v %s — %s\n", num, title, htmlURL)
	}
	return registry.Result{Content: strings.TrimSpace(sb.String())}, nil
}

// GitHubMergePR merges a pull request.
type GitHubMergePR struct{}

func (GitHubMergePR) Name() string         { return "github_merge_pr" }
func (GitHubMergePR) Description() string  { return "Merge a GitHub pull request." }
func (GitHubMergePR) Tier() registry.Tier  { return registry.TierExternal }
func (GitHubMergePR) World() registry.World { return registry.WorldCoder }
func (GitHubMergePR) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"owner":{"type":"string"},"repo":{"type":"string"},"number":{"type":"integer"},"merge_method":{"type":"string","enum":["merge","squash","rebase"]}},"required":["owner","repo","number"]}`)
}
func (GitHubMergePR) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		Number      int    `json:"number"`
		MergeMethod string `json:"merge_method"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.MergeMethod == "" {
		in.MergeMethod = "squash"
	}
	_, err := githubReq(ctx, "PUT", fmt.Sprintf("/repos/%s/%s/pulls/%d/merge", in.Owner, in.Repo, in.Number), map[string]any{
		"merge_method": in.MergeMethod,
	})
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: fmt.Sprintf("PR #%d merged", in.Number)}, nil
}

// AllGitHubAPITools returns all GitHub API tools.
func AllGitHubAPITools() []registry.Tool {
	return []registry.Tool{
		GitHubCreatePR{},
		GitHubCreateIssue{},
		GitHubListPRs{},
		GitHubMergePR{},
	}
}
