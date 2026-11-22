package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAI talks to any OpenAI-compatible /chat/completions endpoint.
type OpenAI struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// NewOpenAI creates a provider with a generous timeout: models need seconds.
func NewOpenAI(baseURL, apiKey, model string) *OpenAI {
	return &OpenAI{BaseURL: strings.TrimRight(baseURL, "/"), APIKey: apiKey, Model: model, HTTP: &http.Client{Timeout: 90 * time.Second}}
}

type wireTool struct {
	Type     string `json:"type"`
	Function Tool   `json:"function"`
}

type wireRequest struct {
	Model      string     `json:"model"`
	Messages   []Message  `json:"messages"`
	Tools      []wireTool `json:"tools,omitempty"`
	ToolChoice string     `json:"tool_choice,omitempty"`
}

type wireResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   *string    `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

// Chat sends one request.
func (o *OpenAI) Chat(ctx context.Context, req Request) (Response, error) {
	body := wireRequest{Model: o.Model, Messages: req.Messages}
	for _, t := range req.Tools {
		body.Tools = append(body.Tools, wireTool{Type: "function", Function: t})
	}
	if len(body.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return Response{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if o.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+o.APIKey)
	}
	resp, err := o.HTTP.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode != http.StatusOK {
		msg := string(data)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return Response{}, &StatusError{Status: resp.StatusCode, Body: msg}
	}
	var wr wireResponse
	if err := json.Unmarshal(data, &wr); err != nil {
		return Response{}, fmt.Errorf("llm: bad response: %w", err)
	}
	if len(wr.Choices) == 0 {
		return Response{}, fmt.Errorf("llm: response without choices")
	}
	m := wr.Choices[0].Message
	out := Message{Role: RoleAssistant, ToolCalls: m.ToolCalls}
	if m.Content != nil {
		out.Content = *m.Content
	}
	for i := range out.ToolCalls {
		if out.ToolCalls[i].Type == "" {
			out.ToolCalls[i].Type = "function"
		}
	}
	return Response{Message: out, Usage: wr.Usage}, nil
}
