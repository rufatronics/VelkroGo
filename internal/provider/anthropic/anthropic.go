// Package anthropic implements the provider.Provider interface against the
// Anthropic Messages API, including tool use.
package anthropic

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

const defaultBaseURL = "https://api.anthropic.com"

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func New(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{apiKey: apiKey, baseURL: baseURL, http: &http.Client{Timeout: 120 * time.Second}}
}

func (c *Client) Name() string { return "anthropic" }

func (c *Client) Capabilities() provider.Caps {
	return provider.Caps{Tools: true, Vision: true, Streaming: true}
}

type contentBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Input     json.RawMessage `json:"input,omitempty"`
	ToolUseID string          `json:"tool_use_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	IsError   bool            `json:"is_error,omitempty"`
}

type apiMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

type apiTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type apiRequest struct {
	Model     string       `json:"model"`
	System    string       `json:"system,omitempty"`
	Messages  []apiMessage `json:"messages"`
	Tools     []apiTool    `json:"tools,omitempty"`
	MaxTokens int          `json:"max_tokens"`
}

type apiResponse struct {
	Content    []contentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	Error *struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Chat(ctx context.Context, req provider.CompletionRequest) (provider.CompletionResponse, error) {
	api := apiRequest{Model: req.Model, System: req.System, MaxTokens: req.MaxTokens}
	if api.MaxTokens == 0 {
		api.MaxTokens = 4096
	}
	for _, t := range req.Tools {
		api.Tools = append(api.Tools, apiTool{Name: t.Name, Description: t.Description, InputSchema: t.Schema})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			// Anthropic encodes tool results as user-role tool_result blocks.
			api.Messages = append(api.Messages, apiMessage{Role: "user", Content: []contentBlock{{
				Type: "tool_result", ToolUseID: m.ToolResult.CallID, Content: m.ToolResult.Content, IsError: m.ToolResult.IsError,
			}}})
		case "assistant":
			blocks := []contentBlock{}
			if m.Content != "" {
				blocks = append(blocks, contentBlock{Type: "text", Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				blocks = append(blocks, contentBlock{Type: "tool_use", ID: tc.ID, Name: tc.Name, Input: tc.Args})
			}
			api.Messages = append(api.Messages, apiMessage{Role: "assistant", Content: blocks})
		default:
			api.Messages = append(api.Messages, apiMessage{Role: "user", Content: []contentBlock{{Type: "text", Text: m.Content}}})
		}
	}

	body, err := json.Marshal(api)
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return provider.CompletionResponse{}, fmt.Errorf("anthropic: bad response (%d): %s", resp.StatusCode, truncate(raw))
	}
	if out.Error != nil {
		return provider.CompletionResponse{}, fmt.Errorf("anthropic: %s: %s", out.Error.Type, out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return provider.CompletionResponse{}, fmt.Errorf("anthropic: HTTP %d: %s", resp.StatusCode, truncate(raw))
	}

	res := provider.CompletionResponse{
		StopReason: out.StopReason,
		InputToks:  out.Usage.InputTokens,
		OutputToks: out.Usage.OutputTokens,
	}
	for _, b := range out.Content {
		switch b.Type {
		case "text":
			res.Text += b.Text
		case "tool_use":
			res.ToolCalls = append(res.ToolCalls, provider.ToolCall{ID: b.ID, Name: b.Name, Args: b.Input})
		}
	}
	return res, nil
}

func truncate(b []byte) string {
	if len(b) > 500 {
		return string(b[:500]) + "…"
	}
	return string(b)
}
