// Package openaicompat implements provider.Provider against any
// OpenAI-compatible chat-completions endpoint. This single adapter covers
// OpenAI itself plus Ollama, LM Studio, OpenRouter, vLLM, and arbitrary custom
// providers via a configurable base URL.
package openaicompat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rufatronics/velkrogo/internal/provider"
)

type Client struct {
	name    string
	apiKey  string
	baseURL string // e.g. https://api.openai.com/v1 or http://localhost:11434/v1
	http    *http.Client
}

func New(name, apiKey, baseURL string) *Client {
	if name == "" {
		name = "openai-compatible"
	}
	return &Client{name: name, apiKey: apiKey, baseURL: baseURL, http: &http.Client{Timeout: 120 * time.Second}}
}

func (c *Client) Name() string { return c.name }

func (c *Client) Capabilities() provider.Caps {
	return provider.Caps{Tools: true, Streaming: true, JSONMode: true}
}

type apiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type apiMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ToolCalls  []apiToolCall `json:"tool_calls,omitempty"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type apiRequest struct {
	Model       string       `json:"model"`
	Messages    []apiMessage `json:"messages"`
	Tools       []any        `json:"tools,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty"`
	Temperature float32      `json:"temperature,omitempty"`
}

type apiResponse struct {
	Choices []struct {
		Message      apiMessage `json:"message"`
		FinishReason string     `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Chat(ctx context.Context, req provider.CompletionRequest) (provider.CompletionResponse, error) {
	api := apiRequest{Model: req.Model, MaxTokens: req.MaxTokens, Temperature: req.Temperature}
	if req.System != "" {
		api.Messages = append(api.Messages, apiMessage{Role: "system", Content: req.System})
	}
	for _, t := range req.Tools {
		api.Tools = append(api.Tools, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        t.Name,
				"description": t.Description,
				"parameters":  json.RawMessage(t.Schema),
			},
		})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			api.Messages = append(api.Messages, apiMessage{Role: "tool", Content: m.ToolResult.Content, ToolCallID: m.ToolResult.CallID})
		case "assistant":
			am := apiMessage{Role: "assistant", Content: m.Content}
			for _, tc := range m.ToolCalls {
				atc := apiToolCall{ID: tc.ID, Type: "function"}
				atc.Function.Name = tc.Name
				atc.Function.Arguments = string(tc.Args)
				am.ToolCalls = append(am.ToolCalls, atc)
			}
			api.Messages = append(api.Messages, am)
		default:
			api.Messages = append(api.Messages, apiMessage{Role: m.Role, Content: m.Content})
		}
	}

	body, err := json.Marshal(api)
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	httpReq.Header.Set("content-type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return provider.CompletionResponse{}, fmt.Errorf("%s: bad response (%d): %.500s", c.name, resp.StatusCode, raw)
	}
	if out.Error != nil {
		return provider.CompletionResponse{}, fmt.Errorf("%s: %s", c.name, out.Error.Message)
	}
	if len(out.Choices) == 0 {
		return provider.CompletionResponse{}, fmt.Errorf("%s: empty choices (HTTP %d)", c.name, resp.StatusCode)
	}

	ch := out.Choices[0]
	res := provider.CompletionResponse{
		Text:       ch.Message.Content,
		StopReason: ch.FinishReason,
		InputToks:  out.Usage.PromptTokens,
		OutputToks: out.Usage.CompletionTokens,
	}
	for _, tc := range ch.Message.ToolCalls {
		res.ToolCalls = append(res.ToolCalls, provider.ToolCall{ID: tc.ID, Name: tc.Function.Name, Args: json.RawMessage(tc.Function.Arguments)})
	}
	return res, nil
}
