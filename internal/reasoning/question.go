// Package reasoning implements the Claude-style question box. When the agent is
// uncertain or about to take a consequential action on an inferred target, it
// pauses and asks the user a structured question before proceeding. See
// ARCHITECTURE.md §5.2.
package reasoning

import "context"

// Option is one selectable answer.
type Option struct {
	Label       string
	Description string
}

// Question is a single structured question (2-4 options; "Other" free-text is
// always implicitly available, mirroring Claude's clarifying-question UX).
type Question struct {
	Header      string // short chip, e.g. "Auth method"
	Prompt      string
	Options     []Option
	MultiSelect bool
}

// Answer carries the user's response.
type Answer struct {
	Selected []string
	OtherText string
}

// Asker surfaces questions to the active frontend and blocks the agent loop
// until the user answers. The orchestrator exposes this to the model as the
// first-class `ask_user` tool.
type Asker interface {
	Ask(ctx context.Context, qs []Question) ([]Answer, error)
}
