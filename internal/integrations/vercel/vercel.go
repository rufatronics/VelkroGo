// Package vercel provides tools for Vercel deployments.
package vercel

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

func vercelToken() string { return os.Getenv("VERCEL_TOKEN") }

func vercelReq(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	tok := vercelToken()
	if tok == "" {
		return nil, 0, fmt.Errorf("VERCEL_TOKEN not set")
	}
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.vercel.com"+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

// VercelListDeployments lists recent deployments.
type VercelListDeployments struct{}

func (VercelListDeployments) Name() string         { return "vercel_list_deployments" }
func (VercelListDeployments) Description() string  { return "List recent Vercel deployments." }
func (VercelListDeployments) Tier() registry.Tier  { return registry.TierReadOnly }
func (VercelListDeployments) World() registry.World { return registry.WorldShared }
func (VercelListDeployments) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"project_id":{"type":"string","description":"Filter by project name or ID"},"limit":{"type":"integer"}}}`)
}
func (VercelListDeployments) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		ProjectID string `json:"project_id"`
		Limit     int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &in)
	if in.Limit == 0 {
		in.Limit = 10
	}
	path := fmt.Sprintf("/v6/deployments?limit=%d", in.Limit)
	if in.ProjectID != "" {
		path += "&projectId=" + in.ProjectID
	}
	b, status, err := vercelReq(ctx, "GET", path, nil)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if status >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	var resp struct {
		Deployments []struct {
			UID   string `json:"uid"`
			URL   string `json:"url"`
			State string `json:"state"`
			Name  string `json:"name"`
		} `json:"deployments"`
	}
	_ = json.Unmarshal(b, &resp)
	if len(resp.Deployments) == 0 {
		return registry.Result{Content: "No deployments found."}, nil
	}
	var sb strings.Builder
	for _, d := range resp.Deployments {
		fmt.Fprintf(&sb, "[%s] %s — https://%s\n", d.State, d.Name, d.URL)
	}
	return registry.Result{Content: strings.TrimSpace(sb.String())}, nil
}

// VercelDeploy triggers a deployment.
type VercelDeploy struct{}

func (VercelDeploy) Name() string         { return "vercel_deploy" }
func (VercelDeploy) Description() string  { return "Trigger a Vercel deployment for a project." }
func (VercelDeploy) Tier() registry.Tier  { return registry.TierExternal }
func (VercelDeploy) World() registry.World { return registry.WorldShared }
func (VercelDeploy) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"project_id":{"type":"string","description":"Vercel project name or ID"},"git_source":{"type":"object","description":"Optional git source: {type:'github',repoId:'...',ref:'main'}"}},"required":["project_id"]}`)
}
func (VercelDeploy) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		ProjectID string         `json:"project_id"`
		GitSource map[string]any `json:"git_source"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	payload := map[string]any{"name": in.ProjectID}
	if in.GitSource != nil {
		payload["gitSource"] = in.GitSource
	}
	b, status, err := vercelReq(ctx, "POST", "/v13/deployments", payload)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if status >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	var resp struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	_ = json.Unmarshal(b, &resp)
	return registry.Result{Content: fmt.Sprintf("Deployment started: %s — https://%s", resp.ID, resp.URL)}, nil
}

// VercelSetEnv sets an environment variable for a Vercel project.
type VercelSetEnv struct{}

func (VercelSetEnv) Name() string         { return "vercel_set_env" }
func (VercelSetEnv) Description() string  { return "Set an environment variable for a Vercel project." }
func (VercelSetEnv) Tier() registry.Tier  { return registry.TierExternal }
func (VercelSetEnv) World() registry.World { return registry.WorldShared }
func (VercelSetEnv) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"project_id":{"type":"string"},"key":{"type":"string"},"value":{"type":"string"},"target":{"type":"array","items":{"type":"string","enum":["production","preview","development"]}}},"required":["project_id","key","value"]}`)
}
func (VercelSetEnv) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		ProjectID string   `json:"project_id"`
		Key       string   `json:"key"`
		Value     string   `json:"value"`
		Target    []string `json:"target"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if len(in.Target) == 0 {
		in.Target = []string{"production", "preview", "development"}
	}
	payload := map[string]any{
		"key":    in.Key,
		"value":  in.Value,
		"type":   "encrypted",
		"target": in.Target,
	}
	b, status, err := vercelReq(ctx, "POST", fmt.Sprintf("/v10/projects/%s/env", in.ProjectID), payload)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if status >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	return registry.Result{Content: fmt.Sprintf("env var %s set on project %s", in.Key, in.ProjectID)}, nil
}

// AllVercelTools returns all Vercel tools.
func AllVercelTools() []registry.Tool {
	return []registry.Tool{
		VercelListDeployments{},
		VercelDeploy{},
		VercelSetEnv{},
	}
}
