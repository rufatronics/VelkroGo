// Package supabase provides tools for interacting with a Supabase project.
package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/rufatronics/velkrogo/internal/registry"
)

func supabaseURL() string { return os.Getenv("SUPABASE_URL") }
func supabaseKey() string {
	if k := os.Getenv("SUPABASE_SERVICE_KEY"); k != "" {
		return k
	}
	return os.Getenv("SUPABASE_ANON_KEY")
}

func supaReq(ctx context.Context, method, path string, body any) ([]byte, int, error) {
	url := supabaseURL()
	key := supabaseKey()
	if url == "" || key == "" {
		return nil, 0, fmt.Errorf("SUPABASE_URL and SUPABASE_SERVICE_KEY (or SUPABASE_ANON_KEY) must be set")
	}
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		reqBody = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url+path, reqBody)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "return=representation")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return b, resp.StatusCode, nil
}

// SupabaseSelect reads rows from a table.
type SupabaseSelect struct{}

func (SupabaseSelect) Name() string         { return "supabase_select" }
func (SupabaseSelect) Description() string  { return "Read rows from a Supabase table. Optionally filter with PostgREST query string (e.g. 'id=eq.1')." }
func (SupabaseSelect) Tier() registry.Tier  { return registry.TierReadOnly }
func (SupabaseSelect) World() registry.World { return registry.WorldShared }
func (SupabaseSelect) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"table":{"type":"string"},"filter":{"type":"string","description":"PostgREST filter query string, e.g. id=eq.1"}},"required":["table"]}`)
}
func (SupabaseSelect) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Table  string `json:"table"`
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	path := "/rest/v1/" + in.Table + "?select=*"
	if in.Filter != "" {
		path += "&" + in.Filter
	}
	b, status, err := supaReq(ctx, "GET", path, nil)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if status >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	return registry.Result{Content: string(b)}, nil
}

// SupabaseInsert inserts a row into a table.
type SupabaseInsert struct{}

func (SupabaseInsert) Name() string         { return "supabase_insert" }
func (SupabaseInsert) Description() string  { return "Insert a row into a Supabase table." }
func (SupabaseInsert) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (SupabaseInsert) World() registry.World { return registry.WorldShared }
func (SupabaseInsert) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"table":{"type":"string"},"row":{"type":"object","description":"Key-value pairs for the new row"}},"required":["table","row"]}`)
}
func (SupabaseInsert) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Table string         `json:"table"`
		Row   map[string]any `json:"row"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	b, status, err := supaReq(ctx, "POST", "/rest/v1/"+in.Table, in.Row)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if status >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	return registry.Result{Content: string(b)}, nil
}

// SupabaseUpdate updates rows matching a filter.
type SupabaseUpdate struct{}

func (SupabaseUpdate) Name() string         { return "supabase_update" }
func (SupabaseUpdate) Description() string  { return "Update rows in a Supabase table matching a PostgREST filter." }
func (SupabaseUpdate) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (SupabaseUpdate) World() registry.World { return registry.WorldShared }
func (SupabaseUpdate) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"table":{"type":"string"},"filter":{"type":"string","description":"PostgREST filter, e.g. id=eq.1"},"updates":{"type":"object"}},"required":["table","filter","updates"]}`)
}
func (SupabaseUpdate) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Table   string         `json:"table"`
		Filter  string         `json:"filter"`
		Updates map[string]any `json:"updates"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	path := "/rest/v1/" + in.Table + "?" + in.Filter
	b, status, err := supaReq(ctx, "PATCH", path, in.Updates)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if status >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	return registry.Result{Content: string(b)}, nil
}

// SupabaseDelete deletes rows matching a filter.
type SupabaseDelete struct{}

func (SupabaseDelete) Name() string         { return "supabase_delete" }
func (SupabaseDelete) Description() string  { return "Delete rows from a Supabase table matching a PostgREST filter." }
func (SupabaseDelete) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (SupabaseDelete) World() registry.World { return registry.WorldShared }
func (SupabaseDelete) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"table":{"type":"string"},"filter":{"type":"string","description":"PostgREST filter, e.g. id=eq.1"}},"required":["table","filter"]}`)
}
func (SupabaseDelete) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Table  string `json:"table"`
		Filter string `json:"filter"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	path := "/rest/v1/" + in.Table + "?" + in.Filter
	b, status, err := supaReq(ctx, "DELETE", path, nil)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if status >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	return registry.Result{Content: "deleted"}, nil
}

// SupabaseStorageUpload uploads a file to Supabase Storage.
type SupabaseStorageUpload struct{}

func (SupabaseStorageUpload) Name() string         { return "supabase_storage_upload" }
func (SupabaseStorageUpload) Description() string  { return "Upload a local file to a Supabase Storage bucket." }
func (SupabaseStorageUpload) Tier() registry.Tier  { return registry.TierExternal }
func (SupabaseStorageUpload) World() registry.World { return registry.WorldShared }
func (SupabaseStorageUpload) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"bucket":{"type":"string"},"object_path":{"type":"string","description":"Destination path in the bucket"},"local_path":{"type":"string","description":"Local file path to upload"},"content_type":{"type":"string"}},"required":["bucket","object_path","local_path"]}`)
}
func (SupabaseStorageUpload) Execute(ctx context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Bucket      string `json:"bucket"`
		ObjectPath  string `json:"object_path"`
		LocalPath   string `json:"local_path"`
		ContentType string `json:"content_type"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if in.ContentType == "" {
		in.ContentType = "application/octet-stream"
	}
	url := supabaseURL()
	key := supabaseKey()
	if url == "" || key == "" {
		return registry.Result{IsError: true, Content: "SUPABASE_URL and SUPABASE_SERVICE_KEY must be set"}, nil
	}
	data, err := os.ReadFile(in.LocalPath)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	apiURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", url, in.Bucket, in.ObjectPath)
	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(data))
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	req.Header.Set("apikey", key)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", in.ContentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return registry.Result{IsError: true, Content: string(b)}, nil
	}
	return registry.Result{Content: fmt.Sprintf("uploaded to %s/%s", in.Bucket, in.ObjectPath)}, nil
}

// AllSupabaseTools returns all Supabase tools.
func AllSupabaseTools() []registry.Tool {
	return []registry.Tool{
		SupabaseSelect{},
		SupabaseInsert{},
		SupabaseUpdate{},
		SupabaseDelete{},
		SupabaseStorageUpload{},
	}
}
