package orchestrator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rufatronics/velkrogo/internal/policy"
	"github.com/rufatronics/velkrogo/internal/provider"
	"github.com/rufatronics/velkrogo/internal/registry"
)

// scriptedProvider returns canned responses in order.
type scriptedProvider struct {
	turns []provider.CompletionResponse
	i     int
}

func (s *scriptedProvider) Name() string                    { return "scripted" }
func (s *scriptedProvider) Capabilities() provider.Caps     { return provider.Caps{Tools: true} }
func (s *scriptedProvider) Chat(_ context.Context, _ provider.CompletionRequest) (provider.CompletionResponse, error) {
	r := s.turns[s.i]
	s.i++
	return r, nil
}

type echoTool struct {
	tier registry.Tier
	ran  *bool
}

func (e echoTool) Name() string            { return "echo" }
func (e echoTool) Description() string     { return "echo" }
func (e echoTool) Tier() registry.Tier     { return e.tier }
func (e echoTool) World() registry.World   { return registry.WorldShared }
func (e echoTool) Schema() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (e echoTool) Execute(_ context.Context, args json.RawMessage) (registry.Result, error) {
	*e.ran = true
	return registry.Result{Content: string(args)}, nil
}

type autoApprover struct{ approve bool }

func (a autoApprover) Approve(context.Context, registry.Tool, string) (bool, *policy.Grant, error) {
	return a.approve, nil, nil
}

func newTestEngine(t *testing.T, prov provider.Provider, tool registry.Tool, app Approver) *Engine {
	t.Helper()
	reg := registry.NewMemory()
	if tool != nil {
		if err := reg.Register(tool); err != nil {
			t.Fatal(err)
		}
	}
	return &Engine{
		Provider: prov, Model: "test", Registry: reg,
		Policy: policy.NewBasic(), Approver: app, World: registry.WorldShared,
	}
}

func TestLoopExecutesT0ToolWithoutApproval(t *testing.T) {
	ran := false
	prov := &scriptedProvider{turns: []provider.CompletionResponse{
		{ToolCalls: []provider.ToolCall{{ID: "1", Name: "echo", Args: json.RawMessage(`{"x":1}`)}}},
		{Text: "done"},
	}}
	e := newTestEngine(t, prov, echoTool{registry.TierReadOnly, &ran}, autoApprover{false})
	if err := e.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("T0 tool should run without approval")
	}
}

func TestLoopBlocksDeclinedT1Tool(t *testing.T) {
	ran := false
	prov := &scriptedProvider{turns: []provider.CompletionResponse{
		{ToolCalls: []provider.ToolCall{{ID: "1", Name: "echo", Args: json.RawMessage(`{}`)}}},
		{Text: "ok, declined"},
	}}
	e := newTestEngine(t, prov, echoTool{registry.TierReversibleLocal, &ran}, autoApprover{false})
	if err := e.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if ran {
		t.Fatal("declined T1 tool must not execute")
	}
	// The decline must be reported back to the model as a tool error.
	last := e.history[len(e.history)-2]
	if last.Role != "tool" || !last.ToolResult.IsError {
		t.Fatalf("expected error tool result, got %+v", last)
	}
}

func TestSetPlanUpdatesVisiblePlan(t *testing.T) {
	prov := &scriptedProvider{turns: []provider.CompletionResponse{
		{ToolCalls: []provider.ToolCall{{ID: "1", Name: "set_plan",
			Args: json.RawMessage(`{"steps":[{"title":"a","status":"active"},{"title":"b"}]}`)}}},
		{Text: "planned"},
	}}
	e := newTestEngine(t, prov, nil, nil)
	if err := e.Run(context.Background(), "go"); err != nil {
		t.Fatal(err)
	}
	if len(e.plan.Steps) != 2 || e.plan.Steps[0].Status != StepActive || e.plan.Steps[1].Status != StepPending {
		t.Fatalf("unexpected plan: %+v", e.plan)
	}
}

func TestSaverModeHidesHighTierTools(t *testing.T) {
	ran := false
	e := newTestEngine(t, &scriptedProvider{}, echoTool{registry.TierDeviceControl, &ran}, nil)
	e.Mode = ModeSaver
	for _, s := range e.toolSpecs() {
		if s.Name == "echo" {
			t.Fatal("saver mode must not advertise T3 tools to the model")
		}
	}
}
