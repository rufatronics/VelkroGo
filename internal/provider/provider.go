// Package provider defines the vendor-agnostic LLM interface. Built-in adapters
// (Anthropic, OpenAI / OpenAI-compatible incl. custom endpoints) all implement
// Provider, so the agent core never depends on a specific vendor. See
// ARCHITECTURE.md §5.5.
package provider

import (
	"context"
	"encoding/json"
)

// Role identifies a logical model slot. Cost mode and model routing map roles to
// concrete models (e.g. RoleSmall -> a cheap model in saver mode).
type Role string

const (
	RoleMain   Role = "main"   // everyday reasoning
	RoleStrong Role = "strong" // hard problems / deep reasoning
	RoleSmall  Role = "small"  // planning, summaries, saver mode
	RoleVision Role = "vision" // screenshots / computer use
)

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

// ToolResult is the observation returned for a prior ToolCall.
type ToolResult struct {
	CallID  string
	Content string
	IsError bool
}

// Message is one turn of conversation in vendor-neutral form.
type Message struct {
	Role       string // "system" | "user" | "assistant" | "tool"
	Content    string
	ToolCalls  []ToolCall  // set on assistant messages that invoked tools
	ToolResult *ToolResult // set on role "tool" messages
}

// ToolSpec is the schema advertised to the model for one tool.
type ToolSpec struct {
	Name        string
	Description string
	Schema      json.RawMessage
}

// CompletionRequest is a vendor-neutral request.
type CompletionRequest struct {
	Model       string
	System      string
	Messages    []Message
	Tools       []ToolSpec
	MaxTokens   int
	Temperature float32
}

// CompletionResponse is the result of one model turn.
type CompletionResponse struct {
	Text       string
	ToolCalls  []ToolCall // non-empty if the model requested tools
	StopReason string
	InputToks  int
	OutputToks int
}

// Delta is a streamed chunk (text or tool-call fragment).
type Delta struct {
	Text string
	Done bool
}

// Caps describes optional features an adapter supports.
type Caps struct {
	Tools     bool
	Vision    bool
	Streaming bool
	JSONMode  bool
}

// Provider is the swappable brain. Custom vendors are added either via the
// OpenAI-compatible adapter (custom base URL) or by implementing this interface.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	Capabilities() Caps
}
