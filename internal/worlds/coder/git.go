// Package coder implements World 1 tools: VCS, build/test, and GitHub.
package coder

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/rufatronics/velkrogo/internal/registry"
)

func gitRun(ctx context.Context, dir string, args ...string) (string, error) {
	ctx2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx2, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// GitStatus is T0 — read-only.
type GitStatus struct{}

func (GitStatus) Name() string          { return "git_status" }
func (GitStatus) Tier() registry.Tier   { return registry.TierReadOnly }
func (GitStatus) World() registry.World { return registry.WorldCoder }
func (GitStatus) Description() string   { return "Show git status of a repository." }
func (GitStatus) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Repo directory"}},"required":["path"]}`)
}
func (GitStatus) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct{ Path string `json:"path"` }
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	out, err := gitRun(ctx, in.Path, "status", "--short", "--branch")
	if err != nil {
		return registry.Result{Content: out, IsError: true}, nil
	}
	return registry.Result{Content: out}, nil
}

// GitDiff is T0.
type GitDiff struct{}

func (GitDiff) Name() string          { return "git_diff" }
func (GitDiff) Tier() registry.Tier   { return registry.TierReadOnly }
func (GitDiff) World() registry.World { return registry.WorldCoder }
func (GitDiff) Description() string   { return "Show unstaged or staged git diff." }
func (GitDiff) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"staged":{"type":"boolean","description":"If true, show staged diff"}},"required":["path"]}`)
}
func (GitDiff) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path   string `json:"path"`
		Staged bool   `json:"staged"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	gitArgs := []string{"diff"}
	if in.Staged {
		gitArgs = append(gitArgs, "--cached")
	}
	out, err := gitRun(ctx, in.Path, gitArgs...)
	if err != nil {
		return registry.Result{Content: out, IsError: true}, nil
	}
	if out == "" {
		out = "(no changes)"
	}
	return registry.Result{Content: out}, nil
}

// GitLog is T0.
type GitLog struct{}

func (GitLog) Name() string          { return "git_log" }
func (GitLog) Tier() registry.Tier   { return registry.TierReadOnly }
func (GitLog) World() registry.World { return registry.WorldCoder }
func (GitLog) Description() string   { return "Show recent git commit log." }
func (GitLog) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"n":{"type":"integer","description":"Number of commits (default 10)"}},"required":["path"]}`)
}
func (GitLog) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path string `json:"path"`
		N    int    `json:"n"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.N <= 0 {
		in.N = 10
	}
	out, err := gitRun(ctx, in.Path, "log", fmt.Sprintf("-%d", in.N), "--oneline", "--decorate")
	if err != nil {
		return registry.Result{Content: out, IsError: true}, nil
	}
	return registry.Result{Content: out}, nil
}

// GitClone is T1: creates local files.
type GitClone struct{}

func (GitClone) Name() string          { return "git_clone" }
func (GitClone) Tier() registry.Tier   { return registry.TierReversibleLocal }
func (GitClone) World() registry.World { return registry.WorldCoder }
func (GitClone) Description() string   { return "Clone a git repository to a local directory." }
func (GitClone) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"},"dest":{"type":"string","description":"Destination directory"}},"required":["url","dest"]}`)
}
func (GitClone) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		URL  string `json:"url"`
		Dest string `json:"dest"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	ctx2, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx2, "git", "clone", "--depth=1", in.URL, in.Dest)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return registry.Result{Content: out.String(), IsError: true}, nil
	}
	return registry.Result{Content: fmt.Sprintf("cloned %s → %s", in.URL, in.Dest)}, nil
}

// GitCommit is T1: local commit only, no push.
type GitCommit struct{}

func (GitCommit) Name() string          { return "git_commit" }
func (GitCommit) Tier() registry.Tier   { return registry.TierReversibleLocal }
func (GitCommit) World() registry.World { return registry.WorldCoder }
func (GitCommit) Description() string   { return "Stage all changes and create a local git commit." }
func (GitCommit) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"message":{"type":"string","description":"Commit message"}},"required":["path","message"]}`)
}
func (GitCommit) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if _, err := gitRun(ctx, in.Path, "add", "-A"); err != nil {
		return registry.Result{Content: "git add failed", IsError: true}, nil
	}
	out, err := gitRun(ctx, in.Path, "commit", "-m", in.Message)
	if err != nil {
		return registry.Result{Content: out, IsError: true}, nil
	}
	return registry.Result{Content: out}, nil
}

// GitPush is T2: external/outward action, always previewed before approval.
type GitPush struct{}

func (GitPush) Name() string          { return "git_push" }
func (GitPush) Tier() registry.Tier   { return registry.TierExternal }
func (GitPush) World() registry.World { return registry.WorldCoder }
func (GitPush) Description() string   { return "Push local branch to the remote. Requires approval." }
func (GitPush) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"remote":{"type":"string","default":"origin"},"branch":{"type":"string","description":"Branch name (empty = current)"}},"required":["path"]}`)
}
func (GitPush) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path   string `json:"path"`
		Remote string `json:"remote"`
		Branch string `json:"branch"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.Remote == "" {
		in.Remote = "origin"
	}
	pushArgs := []string{"push", "-u", in.Remote}
	if in.Branch != "" {
		pushArgs = append(pushArgs, in.Branch)
	}
	out, err := gitRun(ctx, in.Path, pushArgs...)
	if err != nil {
		return registry.Result{Content: out, IsError: true}, nil
	}
	return registry.Result{Content: out}, nil
}

// GitCreateBranch is T1.
type GitCreateBranch struct{}

func (GitCreateBranch) Name() string          { return "git_branch" }
func (GitCreateBranch) Tier() registry.Tier   { return registry.TierReversibleLocal }
func (GitCreateBranch) World() registry.World { return registry.WorldCoder }
func (GitCreateBranch) Description() string   { return "Create and checkout a new git branch." }
func (GitCreateBranch) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"name":{"type":"string","description":"New branch name"}},"required":["path","name"]}`)
}
func (GitCreateBranch) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	out, err := gitRun(ctx, in.Path, "checkout", "-b", in.Name)
	if err != nil {
		return registry.Result{Content: out, IsError: true}, nil
	}
	return registry.Result{Content: "created branch " + in.Name}, nil
}

// CoderTools returns all World 1 git tools for registration.
func CoderTools() []registry.Tool {
	return []registry.Tool{
		GitStatus{}, GitDiff{}, GitLog{},
		GitClone{}, GitCommit{}, GitPush{}, GitCreateBranch{},
	}
}
