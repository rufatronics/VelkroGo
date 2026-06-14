package tools

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/rufatronics/velkrogo/internal/registry"
)

type MakeDir struct{}

func (MakeDir) Name() string         { return "make_dir" }
func (MakeDir) Description() string  { return "Create a directory (and any missing parents)." }
func (MakeDir) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (MakeDir) World() registry.World { return registry.WorldShared }
func (MakeDir) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Directory path to create"}},"required":["path"]}`)
}
func (MakeDir) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if err := os.MkdirAll(in.Path, 0o755); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "created: " + in.Path}, nil
}

type DeletePath struct{}

func (DeletePath) Name() string         { return "delete_path" }
func (DeletePath) Description() string  { return "Delete a file or directory (recursive)." }
func (DeletePath) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (DeletePath) World() registry.World { return registry.WorldShared }
func (DeletePath) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"File or directory to delete"}},"required":["path"]}`)
}
func (DeletePath) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if err := os.RemoveAll(in.Path); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "deleted: " + in.Path}, nil
}

type MovePath struct{}

func (MovePath) Name() string         { return "move_path" }
func (MovePath) Description() string  { return "Move or rename a file or directory." }
func (MovePath) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (MovePath) World() registry.World { return registry.WorldShared }
func (MovePath) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"src":{"type":"string"},"dst":{"type":"string"}},"required":["src","dst"]}`)
}
func (MovePath) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Src string `json:"src"`
		Dst string `json:"dst"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(in.Dst), 0o755); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if err := os.Rename(in.Src, in.Dst); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "moved: " + in.Src + " -> " + in.Dst}, nil
}

type CopyFile struct{}

func (CopyFile) Name() string         { return "copy_file" }
func (CopyFile) Description() string  { return "Copy a single file to a new location." }
func (CopyFile) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (CopyFile) World() registry.World { return registry.WorldShared }
func (CopyFile) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"src":{"type":"string"},"dst":{"type":"string"}},"required":["src","dst"]}`)
}
func (CopyFile) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Src string `json:"src"`
		Dst string `json:"dst"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if err := os.MkdirAll(filepath.Dir(in.Dst), 0o755); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	src, err := os.Open(in.Src)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	defer src.Close()
	dst, err := os.Create(in.Dst)
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "copied: " + in.Src + " -> " + in.Dst}, nil
}
