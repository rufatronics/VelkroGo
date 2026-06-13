package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/reasoning"
	"github.com/rufatronics/velkrogo/internal/registry"
)

// Event is a structured update emitted to whichever frontend is attached.
type Event struct {
	Kind    string // "text" | "tool_start" | "tool_done" | "plan" | "usage" | "error"
	Text    string
	Tool    string
	Plan    *Plan
	InToks  int
	OutToks int
}

// Approver is implemented by frontends to gate side-effecting tool calls. The
// returned grant (if non-nil) is added to the session so "allow for session"
// works.
type Approver interface {
	Approve(ctx context.Context, tool registry.Tool, preview string) (approved bool, grant *policy.Grant, err error)
}

// Engine is the in-process agent loop. The daemon hosts this same engine
// behind the local API so frontends connect to it remotely.
type Engine struct {
	Provider provider.Provider
	Model    string
	Registry registry.Registry
	Policy   policy.Engine
	Asker    reasoning.Asker
	Approver Approver
	Mode     CostMode
	World    registry.World
	Events   chan<- Event

	history []provider.Message
	plan    Plan
}

const maxIterations = 40

const systemPromptNormal = `You are VelkroGo, a careful local AI agent.
Before non-trivial tasks, call set_plan to outline numbered steps, and update it as you progress.
When a request is ambiguous, or before consequential actions on inferred targets, call ask_user with 2-4 concrete options instead of guessing.
Use tools when they help; otherwise answer directly and concisely.`

// Saver mode: minimal prompt, fewer instructions, cheap and short.
const systemPromptSaver = `You are VelkroGo, a local AI agent. Be brief. Use tools only when needed. If ambiguous, call ask_user.`

func (e *Engine) emit(ev Event) {
	if e.Events != nil {
		e.Events <- ev
	}
}

func (e *Engine) systemPrompt() string {
	if e.Mode == ModeSaver {
		return systemPromptSaver
	}
	return systemPromptNormal
}

// Run executes one user turn to completion (final text or error).
func (e *Engine) Run(ctx context.Context, userInput string) error {
	e.history = append(e.history, provider.Message{Role: "user", Content: userInput})
	tools := e.toolSpecs()

	for i := 0; i < maxIterations; i++ {
		resp, err := e.Provider.Chat(ctx, provider.CompletionRequest{
			Model:    e.Model,
			System:   e.systemPrompt(),
			Messages: e.history,
			Tools:    tools,
		})
		if err != nil {
			e.emit(Event{Kind: "error", Text: err.Error()})
			return err
		}
		e.emit(Event{Kind: "usage", InToks: resp.InputToks, OutToks: resp.OutputToks})

		e.history = append(e.history, provider.Message{Role: "assistant", Content: resp.Text, ToolCalls: resp.ToolCalls})
		if resp.Text != "" {
			e.emit(Event{Kind: "text", Text: resp.Text})
		}
		if len(resp.ToolCalls) == 0 {
			return nil // model finished its turn
		}
		for _, tc := range resp.ToolCalls {
			result := e.dispatch(ctx, tc)
			e.history = append(e.history, provider.Message{Role: "tool", ToolResult: &result})
		}
	}
	e.emit(Event{Kind: "error", Text: "iteration limit reached"})
	return fmt.Errorf("orchestrator: iteration limit reached")
}

func (e *Engine) dispatch(ctx context.Context, tc provider.ToolCall) provider.ToolResult {
	fail := func(msg string) provider.ToolResult {
		return provider.ToolResult{CallID: tc.ID, Content: msg, IsError: true}
	}

	// Built-ins: plan management and the question box.
	switch tc.Name {
	case "set_plan":
		return e.setPlan(tc)
	case "ask_user":
		return e.askUser(ctx, tc)
	}

	tool, ok := e.Registry.Get(tc.Name)
	if !ok {
		return fail("unknown tool: " + tc.Name)
	}

	e.emit(Event{Kind: "tool_start", Tool: tc.Name, Text: string(tc.Args)})

	target := targetOf(tc.Args)
	preview := fmt.Sprintf("%s %s", tc.Name, tc.Args)
	switch e.Policy.Evaluate(policy.Request{Tool: tool, Target: target, Preview: preview, SaverMode: e.Mode == ModeSaver}, nil) {
	case policy.Deny:
		return fail("denied by policy")
	case policy.Ask:
		if e.Approver == nil {
			return fail("approval required but no approver attached")
		}
		ok, grant, err := e.Approver.Approve(ctx, tool, preview)
		if err != nil {
			return fail("approval failed: " + err.Error())
		}
		if !ok {
			return fail("user declined this action")
		}
		if grant != nil {
			e.Policy.AddGrant(*grant)
		}
	}

	res, err := tool.Execute(ctx, tc.Args)
	if err != nil {
		e.emit(Event{Kind: "tool_done", Tool: tc.Name, Text: "error: " + err.Error()})
		return fail(err.Error())
	}
	e.emit(Event{Kind: "tool_done", Tool: tc.Name, Text: summarize(res.Content)})
	return provider.ToolResult{CallID: tc.ID, Content: res.Content, IsError: res.IsError}
}

func (e *Engine) setPlan(tc provider.ToolCall) provider.ToolResult {
	var in struct {
		Steps []struct {
			Title  string `json:"title"`
			Status string `json:"status"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(tc.Args, &in); err != nil {
		return provider.ToolResult{CallID: tc.ID, Content: err.Error(), IsError: true}
	}
	e.plan = Plan{}
	for i, s := range in.Steps {
		st := StepStatus(s.Status)
		if st == "" {
			st = StepPending
		}
		e.plan.Steps = append(e.plan.Steps, Step{ID: fmt.Sprint(i + 1), Title: s.Title, Status: st})
	}
	p := e.plan
	e.emit(Event{Kind: "plan", Plan: &p})
	return provider.ToolResult{CallID: tc.ID, Content: "plan updated"}
}

func (e *Engine) askUser(ctx context.Context, tc provider.ToolCall) provider.ToolResult {
	fail := func(msg string) provider.ToolResult {
		return provider.ToolResult{CallID: tc.ID, Content: msg, IsError: true}
	}
	var in struct {
		Question string   `json:"question"`
		Options  []string `json:"options"`
	}
	if err := json.Unmarshal(tc.Args, &in); err != nil {
		return fail(err.Error())
	}
	if e.Asker == nil {
		return fail("no asker attached")
	}
	q := reasoning.Question{Prompt: in.Question}
	for _, o := range in.Options {
		q.Options = append(q.Options, reasoning.Option{Label: o})
	}
	answers, err := e.Asker.Ask(ctx, []reasoning.Question{q})
	if err != nil {
		return fail(err.Error())
	}
	a := answers[0]
	reply := strings.Join(a.Selected, ", ")
	if a.OtherText != "" {
		reply = a.OtherText
	}
	return provider.ToolResult{CallID: tc.ID, Content: "user answered: " + reply}
}

func (e *Engine) toolSpecs() []provider.ToolSpec {
	specs := []provider.ToolSpec{
		{
			Name:        "set_plan",
			Description: "Set or update the visible step-by-step plan for the current task.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"steps":{"type":"array","items":{"type":"object","properties":{"title":{"type":"string"},"status":{"type":"string","enum":["pending","active","done","blocked"]}},"required":["title"]}}},"required":["steps"]}`),
		},
		{
			Name:        "ask_user",
			Description: "Ask the user a clarifying question with 2-4 options before proceeding. Use when the request is ambiguous or before consequential actions on inferred targets.",
			Schema:      json.RawMessage(`{"type":"object","properties":{"question":{"type":"string"},"options":{"type":"array","items":{"type":"string"}}},"required":["question","options"]}`),
		},
	}
	for _, t := range e.Registry.Enabled(e.World, e.Mode == ModeSaver) {
		specs = append(specs, provider.ToolSpec{Name: t.Name(), Description: t.Description(), Schema: t.Schema()})
	}
	return specs
}

// targetOf pulls a path/url-ish field out of tool args for grant scope matching.
func targetOf(args json.RawMessage) string {
	var m map[string]any
	_ = json.Unmarshal(args, &m)
	for _, k := range []string{"path", "url", "target", "host"} {
		if v, ok := m[k].(string); ok {
			return v
		}
	}
	return ""
}

func summarize(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}
