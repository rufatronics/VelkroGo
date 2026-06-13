// Package gemini implements provider.Provider against the Google Gemini
// GenerateContent API (v1beta). Uses native Gemini format rather than the
// OpenAI-compat shim to support vision and full function calling.
package gemini

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

const baseURL = "https://generativelanguage.googleapis.com/v1beta/models"

type Client struct {
	apiKey string
	http   *http.Client
}

func New(apiKey string) *Client {
	return &Client{apiKey: apiKey, http: &http.Client{Timeout: 120 * time.Second}}
}

func (c *Client) Name() string { return "gemini" }
func (c *Client) Capabilities() provider.Caps {
	return provider.Caps{Tools: true, Vision: true, Streaming: false}
}

type part struct {
	Text             string            `json:"text,omitempty"`
	FunctionCall     *functionCall     `json:"functionCall,omitempty"`
	FunctionResponse *functionResponse `json:"functionResponse,omitempty"`
}

type functionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type functionResponse struct {
	Name     string `json:"name"`
	Response struct {
		Content string `json:"content"`
	} `json:"response"`
}

type geminiContent struct {
	Role  string `json:"role"` // "user" | "model"
	Parts []part `json:"parts"`
}

type functionDecl struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type apiRequest struct {
	Contents         []geminiContent `json:"contents"`
	SystemInstruction *geminiContent `json:"systemInstruction,omitempty"`
	Tools            []struct {
		FunctionDeclarations []functionDecl `json:"functionDeclarations"`
	} `json:"tools,omitempty"`
	GenerationConfig struct {
		MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
	} `json:"generationConfig"`
}

type apiResponse struct {
	Candidates []struct {
		Content geminiContent `json:"content"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) Chat(ctx context.Context, req provider.CompletionRequest) (provider.CompletionResponse, error) {
	api := apiRequest{}
	if req.MaxTokens > 0 {
		api.GenerationConfig.MaxOutputTokens = req.MaxTokens
	}
	if req.System != "" {
		api.SystemInstruction = &geminiContent{Role: "user", Parts: []part{{Text: req.System}}}
	}
	if len(req.Tools) > 0 {
		decls := make([]functionDecl, len(req.Tools))
		for i, t := range req.Tools {
			decls[i] = functionDecl{Name: t.Name, Description: t.Description, Parameters: t.Schema}
		}
		api.Tools = append(api.Tools, struct {
			FunctionDeclarations []functionDecl `json:"functionDeclarations"`
		}{FunctionDeclarations: decls})
	}
	for _, m := range req.Messages {
		switch m.Role {
		case "tool":
			fr := &functionResponse{Name: m.ToolResult.CallID}
			fr.Response.Content = m.ToolResult.Content
			api.Contents = append(api.Contents, geminiContent{Role: "user", Parts: []part{{FunctionResponse: fr}}})
		case "assistant":
			var parts []part
			if m.Content != "" {
				parts = append(parts, part{Text: m.Content})
			}
			for _, tc := range m.ToolCalls {
				parts = append(parts, part{FunctionCall: &functionCall{Name: tc.Name, Args: tc.Args}})
			}
			api.Contents = append(api.Contents, geminiContent{Role: "model", Parts: parts})
		default:
			api.Contents = append(api.Contents, geminiContent{Role: "user", Parts: []part{{Text: m.Content}}})
		}
	}

	body, _ := json.Marshal(api)
	url := fmt.Sprintf("%s/%s:generateContent?key=%s", baseURL, req.Model, c.apiKey)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return provider.CompletionResponse{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	var out apiResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return provider.CompletionResponse{}, fmt.Errorf("gemini: bad response (%d): %.500s", resp.StatusCode, raw)
	}
	if out.Error != nil {
		return provider.CompletionResponse{}, fmt.Errorf("gemini: %s", out.Error.Message)
	}
	if len(out.Candidates) == 0 {
		return provider.CompletionResponse{}, fmt.Errorf("gemini: empty response")
	}

	res := provider.CompletionResponse{
		InputToks:  out.UsageMetadata.PromptTokenCount,
		OutputToks: out.UsageMetadata.CandidatesTokenCount,
		StopReason: "stop",
	}
	for _, p := range out.Candidates[0].Content.Parts {
		if p.Text != "" {
			res.Text += p.Text
		}
		if p.FunctionCall != nil {
			res.ToolCalls = append(res.ToolCalls, provider.ToolCall{
				ID:   p.FunctionCall.Name,
				Name: p.FunctionCall.Name,
				Args: p.FunctionCall.Args,
			})
		}
	}
	return res, nil
}
