// Package tools holds built-in capabilities. Phase 1 ships read-only (T0)
// filesystem tools to prove tool routing end to end; write/shell tools arrive
// with the full policy previews in later phases.
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rufatronics/velkrogo/internal/registry"
)

// ReadFile is a T0 tool that returns the contents of a file.
type ReadFile struct{}

func (ReadFile) Name() string           { return "read_file" }
func (ReadFile) Description() string    { return "Read a text file from disk and return its contents." }
func (ReadFile) Tier() registry.Tier    { return registry.TierReadOnly }
func (ReadFile) World() registry.World  { return registry.WorldShared }
func (ReadFile) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Absolute or relative file path"}},"required":["path"]}`)
}

func (ReadFile) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	const maxBytes = 256 << 10
	b, err := os.ReadFile(in.Path)
	if err != nil {
		return registry.Result{Content: err.Error(), IsError: true}, nil
	}
	if len(b) > maxBytes {
		b = b[:maxBytes]
	}
	return registry.Result{Content: string(b)}, nil
}

// ListDir is a T0 tool that lists a directory.
type ListDir struct{}

func (ListDir) Name() string          { return "list_dir" }
func (ListDir) Description() string   { return "List the entries of a directory." }
func (ListDir) Tier() registry.Tier   { return registry.TierReadOnly }
func (ListDir) World() registry.World { return registry.WorldShared }
func (ListDir) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Directory path"}},"required":["path"]}`)
}

func (ListDir) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	entries, err := os.ReadDir(in.Path)
	if err != nil {
		return registry.Result{Content: err.Error(), IsError: true}, nil
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			fmt.Fprintf(&sb, "%s/\n", e.Name())
		} else {
			fmt.Fprintf(&sb, "%s\n", e.Name())
		}
	}
	return registry.Result{Content: sb.String()}, nil
}

// WriteFile is a T1 tool: reversible local write, gated by the policy engine
// with a content preview before approval.
type WriteFile struct{}

func (WriteFile) Name() string          { return "write_file" }
func (WriteFile) Description() string   { return "Write a text file to disk (creates parent directories)." }
func (WriteFile) Tier() registry.Tier   { return registry.TierReversibleLocal }
func (WriteFile) World() registry.World { return registry.WorldShared }
func (WriteFile) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"]}`)
}

func (WriteFile) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(in.Path), 0o755); err != nil {
		return registry.Result{Content: err.Error(), IsError: true}, nil
	}
	if err := os.WriteFile(in.Path, []byte(in.Content), 0o644); err != nil {
		return registry.Result{Content: err.Error(), IsError: true}, nil
	}
	return registry.Result{Content: fmt.Sprintf("wrote %d bytes to %s", len(in.Content), in.Path)}, nil
}
