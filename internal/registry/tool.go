// Package registry defines capabilities/tools and the registry that exposes
// them to the orchestrator and provider layer. A capability is the unit of
// power the agent has; the policy engine gates every invocation by its tier.
package registry

import (
	"context"
	"encoding/json"
)

// Tier is the risk classification that drives approval behaviour. See
// ARCHITECTURE.md §3.
type Tier int

const (
	TierReadOnly        Tier = iota // T0: web search, read file, read repo
	TierReversibleLocal             // T1: write in workspace, local git commit
	TierExternal                    // T2: push, PR, email, HTTP POST, form submit
	TierDeviceControl               // T3: host shell, GUI control, install software
	TierSelfModify                  // T4: edit own source, add skills, rebuild
)

// World tags which agent surface a tool belongs to.
type World string

const (
	WorldShared   World = "shared"
	WorldCoder    World = "coder"    // World 1
	WorldOperator World = "operator" // World 2
)

// Tool is a single invokable capability. Tools register their JSON schema so the
// provider layer can advertise only the enabled ones for the current world and
// cost mode (fewer tools = cheaper, safer prompts).
type Tool interface {
	Name() string
	Description() string
	Tier() Tier
	World() World
	// Schema returns the JSON Schema for the tool's arguments.
	Schema() json.RawMessage
	// Execute runs the tool. It is only ever called after the policy engine has
	// authorised the invocation.
	Execute(ctx context.Context, args json.RawMessage) (Result, error)
}

// Result is the observation returned to the agent loop after a tool runs.
type Result struct {
	Content string          // human/model-readable summary
	Data    json.RawMessage // optional structured payload
	IsError bool
}

// Registry holds the set of available tools and filters them by world/mode.
type Registry interface {
	Register(t Tool) error
	Get(name string) (Tool, bool)
	// Enabled returns the tools exposed to the model for the given world,
	// honouring cost-mode trimming.
	Enabled(world World, saverMode bool) []Tool
}
