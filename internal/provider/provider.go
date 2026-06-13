// Package provider defines the vendor-agnostic LLM interface. Built-in adapters
// (Anthropic, OpenAI / OpenAI-compatible, Gemini) and a config-driven custom
// adapter all implement Provider, so the agent core never depends on a specific
// vendor. See ARCHITECTURE.md §5.5.
package provider

import "context"

// Role identifies a logical model slot. Cost mode and model routing map roles to
// concrete models (e.g. RoleSmall -> claude-haiku-4-5 in saver mode).
type Role string

const (
	RoleMain    Role = "main"    // everyday reasoning
	RoleStrong  Role = "strong"  // hard problems / deep reasoning
	RoleSmall   Role = "small"   // planning, summaries, saver mode
	RoleVision  Role = "vision"  // screenshots / computer use
)

// Message is one turn of conversation.
type Message struct {
	Role    string // "system" | "user" | "assistant" | "tool"
	Content string
	// ToolCalls / ToolResults carried in adapter-specific encodings are mapped
	// in/out by each adapter; kept minimal here for the contract.
}

// ToolSpec is the schema advertised to the model for one tool.
type ToolSpec struct {
	Name        string
	Description string
	Schema      []byte // JSON Schema
}

// CompletionRequest is a vendor-neutral request.
type CompletionRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolSpec
	MaxTokens   int
	Temperature float32
}

// Delta is a streamed chunk (text or tool-call fragment).
type Delta struct {
	Text     string
	ToolName string
	ToolArgs string
	Done     bool
}

// CompletionResponse is the non-streaming result.
type CompletionResponse struct {
	Text       string
	ToolName   string // set if the model requested a tool call
	ToolArgs   []byte
	StopReason string
	InputToks  int
	OutputToks int
}

// Caps describes optional features an adapter supports.
type Caps struct {
	Tools     bool
	Vision    bool
	Streaming bool
	JSONMode  bool
}

// Provider is the swappable brain. Custom vendors are added either via the
// config-driven adapter in package custom or by implementing this interface.
type Provider interface {
	Name() string
	Chat(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	Stream(ctx context.Context, req CompletionRequest) (<-chan Delta, error)
	Capabilities() Caps
}
