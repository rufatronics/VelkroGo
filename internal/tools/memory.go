package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/rufatronics/velkrogo/internal/memory"
	"github.com/rufatronics/velkrogo/internal/registry"
)

// MemoryStore is the shared DB used by memory tools. Set before registering.
var MemoryStore *memory.DB

type MemoryGet struct{}

func (MemoryGet) Name() string         { return "memory_get" }
func (MemoryGet) Description() string  { return "Recall a persistent memory fact by key." }
func (MemoryGet) Tier() registry.Tier  { return registry.TierReadOnly }
func (MemoryGet) World() registry.World { return registry.WorldShared }
func (MemoryGet) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"key":{"type":"string","description":"The fact key to retrieve"}},"required":["key"]}`)
}
func (MemoryGet) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if MemoryStore == nil {
		return registry.Result{Content: "memory not available"}, nil
	}
	val, err := MemoryStore.GetMemory(in.Key)
	if errors.Is(err, memory.ErrNotFound) {
		return registry.Result{Content: "(not set)"}, nil
	}
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: val}, nil
}

type MemorySet struct{}

func (MemorySet) Name() string         { return "memory_set" }
func (MemorySet) Description() string  { return "Store a persistent memory fact. Persists across sessions." }
func (MemorySet) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (MemorySet) World() registry.World { return registry.WorldShared }
func (MemorySet) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"},"value":{"type":"string","description":"The fact to remember"}},"required":["key","value"]}`)
}
func (MemorySet) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if MemoryStore == nil {
		return registry.Result{Content: "memory not available"}, nil
	}
	if err := MemoryStore.SetMemory(in.Key, in.Value); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "remembered: " + in.Key}, nil
}

type MemoryList struct{}

func (MemoryList) Name() string         { return "memory_list" }
func (MemoryList) Description() string  { return "List all remembered facts." }
func (MemoryList) Tier() registry.Tier  { return registry.TierReadOnly }
func (MemoryList) World() registry.World { return registry.WorldShared }
func (MemoryList) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}
func (MemoryList) Execute(_ context.Context, _ json.RawMessage) (registry.Result, error) {
	if MemoryStore == nil {
		return registry.Result{Content: "(no memory store)"}, nil
	}
	facts, err := MemoryStore.ListMemory()
	if err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	if len(facts) == 0 {
		return registry.Result{Content: "(no facts stored)"}, nil
	}
	var sb strings.Builder
	for _, f := range facts {
		fmt.Fprintf(&sb, "%s: %s\n", f.Key, f.Value)
	}
	return registry.Result{Content: strings.TrimSpace(sb.String())}, nil
}

type MemoryDelete struct{}

func (MemoryDelete) Name() string         { return "memory_delete" }
func (MemoryDelete) Description() string  { return "Delete a remembered fact by key." }
func (MemoryDelete) Tier() registry.Tier  { return registry.TierReversibleLocal }
func (MemoryDelete) World() registry.World { return registry.WorldShared }
func (MemoryDelete) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"key":{"type":"string"}},"required":["key"]}`)
}
func (MemoryDelete) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	var in struct {
		Key string `json:"key"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return registry.Result{}, err
	}
	if MemoryStore == nil {
		return registry.Result{Content: "memory not available"}, nil
	}
	if err := MemoryStore.DeleteMemory(in.Key); err != nil {
		return registry.Result{IsError: true, Content: err.Error()}, nil
	}
	return registry.Result{Content: "deleted: " + in.Key}, nil
}
