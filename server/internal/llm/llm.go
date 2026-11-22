// Package llm is a thin client for OpenAI-compatible /chat/completions with
// tools. Mistral, Ollama and other compatible services differ only in their
// configuration. The Client limits parallel calls, retries transient errors,
// falls back to a second provider and computes the cost of every call.
package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/dgloeckner/mbot-agent-lab/server/internal/config"
)

// Roles of chat messages.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is one chat message in the OpenAI format.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"` // tool name on tool results (required by Mistral)
}

// ToolCall is a function call requested by the model.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall holds the name and the JSON-encoded arguments.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool describes a function the model may call.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request is one chat completion request.
type Request struct {
	Messages []Message
	Tools    []Tool
}

// Usage counts tokens of one call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

// Response is the model's answer.
type Response struct {
	Message Message
	Usage   Usage
}

// Provider is one model endpoint.
type Provider interface {
	Chat(ctx context.Context, req Request) (Response, error)
}

// Call describes a finished call for accounting.
type Call struct {
	Provider     string
	Model        string
	Usage        Usage
	Duration     time.Duration
	CostMicroEUR int64
}

// StatusError is an HTTP error from a provider.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string { return fmt.Sprintf("llm: HTTP %d: %s", e.Status, e.Body) }

// retryable reports whether trying again may help.
func retryable(err error) bool {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Status == 429 || se.Status >= 500
	}
	return !errors.Is(err, context.Canceled)
}

type entry struct {
	name     string
	model    string
	provider Provider
	priceIn  float64
	priceOut float64
}

// Client chooses providers and accounts for calls. Safe for concurrent use.
type Client struct {
	providers map[string]entry
	def       string
	fallback  string
	sem       chan struct{}
	log       *slog.Logger

	// Retry pauses; tests shorten them.
	Backoff []time.Duration
}

// New builds a client from the configuration.
func New(cfg config.LLM, log *slog.Logger) *Client {
	c := &Client{
		providers: map[string]entry{},
		def:       cfg.Default,
		fallback:  cfg.Fallback,
		sem:       make(chan struct{}, max(1, cfg.MaxParallelCalls)),
		log:       log,
		Backoff:   []time.Duration{time.Second, 3 * time.Second},
	}
	for name, p := range cfg.Providers {
		var prov Provider
		model := p.Model
		switch p.Kind {
		case config.ProviderFake:
			prov = Fake{}
			if model == "" {
				model = "testmodell"
			}
		default:
			prov = NewOpenAI(p.BaseURL, p.APIKey, p.Model)
		}
		c.providers[name] = entry{name: name, model: model, provider: prov, priceIn: p.PriceIn, priceOut: p.PriceOut}
	}
	return c
}

// Use registers or replaces a provider (for tests).
func (c *Client) Use(name, model string, p Provider) {
	c.providers[name] = entry{name: name, model: model, provider: p}
}

// Available reports whether any provider is configured.
func (c *Client) Available() bool { return len(c.providers) > 0 }

// DefaultModel names the model new runs use.
func (c *Client) DefaultModel() string { return c.providers[c.def].model }

// Chat runs a request on the default provider, with retries and fallback.
// At most MaxParallelCalls requests run at once; every team has one agent
// with at most one call in flight, so waiting callers are served in turn.
func (c *Client) Chat(ctx context.Context, req Request) (Response, Call, error) {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return Response{}, Call{}, ctx.Err()
	}
	defer func() { <-c.sem }()

	names := []string{c.def}
	if c.fallback != "" && c.fallback != c.def {
		names = append(names, c.fallback)
	}
	var lastErr error
	for _, name := range names {
		e, ok := c.providers[name]
		if !ok {
			continue
		}
		for attempt := 0; ; attempt++ {
			start := time.Now()
			resp, err := e.provider.Chat(ctx, req)
			if err == nil {
				call := Call{Provider: name, Model: e.model, Usage: resp.Usage, Duration: time.Since(start)}
				call.CostMicroEUR = int64(math.Round(float64(resp.Usage.PromptTokens)*e.priceIn + float64(resp.Usage.CompletionTokens)*e.priceOut))
				return resp, call, nil
			}
			lastErr = err
			if ctx.Err() != nil {
				return Response{}, Call{}, ctx.Err()
			}
			if !retryable(err) || attempt >= len(c.Backoff) {
				break
			}
			c.log.Warn("llm call failed, retrying", "provider", name, "attempt", attempt+1, "err", err)
			select {
			case <-time.After(c.Backoff[attempt]):
			case <-ctx.Done():
				return Response{}, Call{}, ctx.Err()
			}
		}
		c.log.Error("llm provider failed", "provider", name, "err", lastErr)
	}
	if lastErr == nil {
		lastErr = errors.New("llm: no provider configured")
	}
	return Response{}, Call{}, lastErr
}
