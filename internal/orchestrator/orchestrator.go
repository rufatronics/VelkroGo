// Package orchestrator runs the agent loop: build context, ask the provider for
// the next step, route any tool call through the policy gate, execute, observe,
// repeat. It owns the visible plan and chooses single- vs. multi-agent
// execution based on cost mode. See ARCHITECTURE.md §5.1.
package orchestrator

// CostMode selects the execution strategy.
type CostMode int

const (
	// ModeNormal may spawn specialised sub-agents (planner/coder/reviewer),
	// uses richer prompts/tools and stronger models, and verifies more.
	ModeNormal CostMode = iota
	// ModeSaver runs one agent at a time with a minimal prompt, a trimmed tool
	// schema, aggressive context summarisation, and the cheap model.
	ModeSaver
)

// StepStatus tracks an item in the visible plan (Manus-style outline).
type StepStatus string

const (
	StepPending StepStatus = "pending"
	StepActive  StepStatus = "active"
	StepDone    StepStatus = "done"
	StepBlocked StepStatus = "blocked"
)

// Step is one item in the task plan the user can watch and edit.
type Step struct {
	ID     string
	Title  string
	Status StepStatus
}

// Plan is the ordered, user-editable outline produced before execution.
type Plan struct {
	Steps []Step
}
