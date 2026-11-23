package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/anthropics/anthropic-sdk-go/v2/option"
)

// Anthropic talks to the Claude Messages API with the official Go SDK.
//
// The agent keeps its history in the OpenAI-style Message format; this
// provider converts it on every call. Claude answers with thinking blocks
// that must go back unchanged with the tool results, so each assistant
// message keeps the native content in Message.Native and is replayed from it.
// The agent trims old history and edits the system prompt (notes), which
// would invalidate replayed thinking blocks; with drop_block the API drops
// such blocks instead of rejecting the request.
type Anthropic struct {
	client anthropic.Client
	model  string
	effort string
}

// anthropicMaxTokens is generous: agent turns are short, and hitting the cap
// would cut a tool call in half.
const anthropicMaxTokens = 16000

// ErrRefused means the model declined the request. Retrying the same request
// does not help; the client moves on to the fallback provider.
var ErrRefused = errors.New("llm: the model declined the request")

// NewAnthropic creates a Claude provider. baseURL is empty for the Claude API;
// tests point it at a fake server. effort is low, medium, high, xhigh, max or
// empty for the model's default.
func NewAnthropic(baseURL, apiKey, model, effort string) *Anthropic {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithMaxRetries(0), // Client.Chat retries and falls back itself
		option.WithRequestTimeout(90 * time.Second),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	return &Anthropic{client: anthropic.NewClient(opts...), model: model, effort: effort}
}

// serverFallbackModels decline some requests through safety classifiers and
// support the server-side refusal fallback, which re-runs such a request on
// a suitable model inside the same call.
var serverFallbackModels = map[string]bool{
	"claude-opus-5-5": true, "claude-sonnet-5-5": true, "claude-opus-5": true,
	"claude-fable-5-1": true, "claude-fable-5-2": true,
}

// Chat sends one request.
func (a *Anthropic) Chat(ctx context.Context, req Request) (Response, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     a.model,
		MaxTokens: anthropicMaxTokens,
		Betas:     []anthropic.AnthropicBeta{anthropic.AnthropicBetaThinkingBindingControls2026_08_01},
		Thinking: anthropic.BetaThinkingConfigParamUnion{OfAdaptive: &anthropic.BetaThinkingConfigAdaptiveParam{
			BlockBinding: anthropic.BetaThinkingBlockBindingParam{
				PrefixMismatchBehavior: anthropic.BetaThinkingPrefixMismatchBehaviorDropBlock,
			},
		}},
		// Cache the growing prefix: within a wake-up every round only appends.
		CacheControl: anthropic.NewBetaCacheControlEphemeralParam(),
	}
	if a.effort != "" {
		params.OutputConfig = anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffort(a.effort)}
	}
	if serverFallbackModels[a.model] {
		params.Betas = append(params.Betas, anthropic.AnthropicBetaServerSideFallback2026_07_01)
		params.Fallbacks = anthropic.BetaFallbacksParamOfDefault()
	}
	system, messages, err := toAnthropicMessages(req.Messages)
	if err != nil {
		return Response{}, err
	}
	if system != "" {
		params.System = []anthropic.BetaTextBlockParam{{Text: system}}
	}
	params.Messages = messages
	for _, t := range req.Tools {
		tool, err := toAnthropicTool(t)
		if err != nil {
			return Response{}, err
		}
		params.Tools = append(params.Tools, anthropic.BetaToolUnionParam{OfTool: &tool})
	}

	resp, err := a.client.Beta.Messages.New(ctx, params)
	if err != nil {
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			msg := apiErr.Error()
			if len(msg) > 300 {
				msg = msg[:300]
			}
			return Response{}, &StatusError{Status: apiErr.StatusCode, Body: msg}
		}
		return Response{}, err
	}
	usage := Usage{
		PromptTokens:     int(resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.CacheCreationInputTokens),
		CompletionTokens: int(resp.Usage.OutputTokens),
		CacheReadTokens:  int(resp.Usage.CacheReadInputTokens),
		CacheWriteTokens: int(resp.Usage.CacheCreationInputTokens),
	}
	if resp.StopReason == anthropic.BetaStopReasonRefusal {
		return Response{Usage: usage}, fmt.Errorf("%w (%s)", ErrRefused, resp.StopDetails.Category)
	}
	out := Message{Role: RoleAssistant, Native: resp.ToParam()}
	var text []string
	for _, block := range resp.Content {
		switch b := block.AsAny().(type) {
		case anthropic.BetaTextBlock:
			if strings.TrimSpace(b.Text) != "" {
				text = append(text, b.Text)
			}
		case anthropic.BetaToolUseBlock:
			out.ToolCalls = append(out.ToolCalls, ToolCall{
				ID: b.ID, Type: "function",
				Function: FunctionCall{Name: b.Name, Arguments: b.JSON.Input.Raw()},
			})
		}
	}
	out.Content = strings.Join(text, "\n\n")
	return Response{Message: out, Usage: usage}, nil
}

// toAnthropicMessages converts the history. The leading system message becomes
// the system prompt; tool results and the user text after them share one user
// turn, tool results first, as the API requires.
func toAnthropicMessages(in []Message) (string, []anthropic.BetaMessageParam, error) {
	var system string
	var out []anthropic.BetaMessageParam
	userBlocks := func() *anthropic.BetaUserMessageParam {
		if n := len(out); n > 0 && out[n-1].OfUser != nil {
			return out[n-1].OfUser
		}
		out = append(out, anthropic.BetaMessageParam{OfUser: &anthropic.BetaUserMessageParam{}})
		return out[len(out)-1].OfUser
	}
	for i, m := range in {
		switch m.Role {
		case RoleSystem:
			if i != 0 {
				return "", nil, fmt.Errorf("llm: system message at position %d", i)
			}
			system = m.Content
		case RoleUser:
			if strings.TrimSpace(m.Content) == "" {
				continue
			}
			u := userBlocks()
			u.Content = append(u.Content, anthropic.NewBetaTextBlock(m.Content))
		case RoleTool:
			u := userBlocks()
			u.Content = append(u.Content, anthropic.NewBetaToolResultBlock(m.ToolCallID, m.Content, false))
		case RoleAssistant:
			if native, ok := m.Native.(anthropic.BetaMessageParam); ok && native.OfAssistant != nil {
				out = append(out, native)
				continue
			}
			// Written by another provider: rebuild from text and tool calls.
			var blocks []anthropic.BetaContentBlockParamUnion
			if strings.TrimSpace(m.Content) != "" {
				blocks = append(blocks, anthropic.NewBetaTextBlock(m.Content))
			}
			for _, tc := range m.ToolCalls {
				var input any = map[string]any{}
				if tc.Function.Arguments != "" {
					if err := json.Unmarshal([]byte(tc.Function.Arguments), &input); err != nil {
						input = map[string]any{}
					}
				}
				blocks = append(blocks, anthropic.NewBetaToolUseBlock(tc.ID, input, tc.Function.Name))
			}
			if len(blocks) == 0 {
				continue
			}
			out = append(out, anthropic.BetaMessageParam{OfAssistant: &anthropic.BetaAssistantMessageParam{Content: blocks}})
		default:
			return "", nil, fmt.Errorf("llm: unknown role %q", m.Role)
		}
	}
	return system, out, nil
}

// toAnthropicTool converts a JSON schema tool definition.
func toAnthropicTool(t Tool) (anthropic.BetaToolParam, error) {
	var schema map[string]any
	if len(t.Parameters) > 0 {
		if err := json.Unmarshal(t.Parameters, &schema); err != nil {
			return anthropic.BetaToolParam{}, fmt.Errorf("llm: tool %s: bad schema: %w", t.Name, err)
		}
	}
	in := anthropic.BetaToolInputSchemaParam{Properties: schema["properties"], ExtraFields: map[string]any{}}
	if in.Properties == nil {
		in.Properties = map[string]any{}
	}
	if req, ok := schema["required"].([]any); ok {
		for _, r := range req {
			if s, ok := r.(string); ok {
				in.Required = append(in.Required, s)
			}
		}
	}
	for k, v := range schema {
		if k != "properties" && k != "required" && k != "type" {
			in.ExtraFields[k] = v
		}
	}
	return anthropic.BetaToolParam{Name: t.Name, Description: anthropic.String(t.Description), InputSchema: in}, nil
}
