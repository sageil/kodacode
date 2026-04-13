package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	openaisdk "github.com/openai/openai-go/v2"
	"github.com/openai/openai-go/v2/option"
	"github.com/openai/openai-go/v2/packages/param"
	"github.com/openai/openai-go/v2/packages/ssestream"
	"github.com/openai/openai-go/v2/responses"
	"github.com/openai/openai-go/v2/shared"
	"github.com/sageil/kodacode/v1/internal/provider"
)

const (
	codexBaseURL = "https://chatgpt.com/backend-api/codex"
)

// debugStream gates high-frequency per-delta log lines that can produce
// hundreds of thousands of entries and slow down the stream consumer.
// Set via KODACODE_DEBUG_STREAM=1 environment variable.
var debugStream = os.Getenv("KODACODE_DEBUG_STREAM") == "1"

// NewWithOAuth creates an OpenAI provider that authenticates via OAuth Bearer
// tokens for ChatGPT Pro/Plus subscription access. It returns a unified Client
// configured to use the Responses API at the codex endpoint.
func NewWithOAuth(auth *provider.AuthEntry, store *provider.AuthStore) *Client {
	mu := &sync.Mutex{}

	middleware := func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		mu.Lock()
		current := store.Get("openai")
		if current == nil {
			current = auth
		}
		if current.Access == "" || current.Expires < time.Now().UnixMilli()+30_000 {
			refreshed, err := provider.OpenAIOAuthRefresh(current.Refresh)
			if err != nil {
				mu.Unlock()
				return nil, fmt.Errorf("openai oauth: token refresh failed: %w", err)
			}
			if refreshed.AccountID == "" {
				refreshed.AccountID = current.AccountID
			}
			_ = store.Set("openai", *refreshed)
			current = refreshed
		}
		token := current.Access
		accountID := current.AccountID
		mu.Unlock()

		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("originator", "kodacode")
		req.Header.Set("User-Agent", fmt.Sprintf("kodacode/1.0 (%s %s)", runtime.GOOS, runtime.GOARCH))
		if accountID != "" {
			req.Header.Set("ChatGPT-Account-Id", accountID)
		}

		resp, err := next(req)
		if err != nil {
			return resp, err
		}

		if resp != nil && resp.StatusCode >= 400 {
			respBody, readErr := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if readErr == nil {
				log.Printf("[openai-oauth] %s %s → HTTP %d\nResponse: %s",
					req.Method, req.URL.String(), resp.StatusCode, string(respBody))
				resp.Body = io.NopCloser(strings.NewReader(string(respBody)))
			}
		}

		return resp, nil
	}

	client := openaisdk.NewClient(
		option.WithAPIKey("openai-oauth-placeholder"),
		option.WithBaseURL(codexBaseURL),
		option.WithMiddleware(middleware),
	)

	return &Client{
		id:                          "openai",
		name:                        "OpenAI",
		sdkClient:                   client,
		useResponsesAPI:             true,
		skipResponseMaxOutputTokens: true,
	}
}

// buildResponseParams converts kodacode's neutral message types to Responses API params.
func buildResponseParams(model string, messages []provider.Message, opts provider.ChatOptions, skipToolChoice bool, reasoningSummary string, includeMaxOutputTokens bool) responses.ResponseNewParams {
	params := responses.ResponseNewParams{
		Model:   shared.ResponsesModel(model),
		Store:   param.NewOpt(false),
		Include: []responses.ResponseIncludable{responses.ResponseIncludableReasoningEncryptedContent},
	}

	// System prompt → instructions.
	if len(opts.SystemParts) > 0 {
		var sb strings.Builder
		for i, part := range opts.SystemParts {
			if part == "" {
				continue
			}
			if i > 0 && sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(part)
		}
		if sb.Len() > 0 {
			params.Instructions = param.NewOpt(sb.String())
		}
	}

	if includeMaxOutputTokens && opts.MaxTokens > 0 {
		params.MaxOutputTokens = param.NewOpt(int64(opts.MaxTokens))
	}

	// Always set reasoning params so the model knows to use the structured
	// reasoning API instead of dumping chain-of-thought as visible text.
	effort := shared.ReasoningEffort(opts.ReasoningEffort)
	if effort == "" {
		effort = shared.ReasoningEffortMedium
	}
	summary := reasoningSummary
	if summary == "" {
		summary = "auto"
	}
	log.Printf("[variant-openai-codex] setting reasoning effort=%q summary=%q", effort, summary)
	params.Reasoning = shared.ReasoningParam{
		Effort:  effort,
		Summary: shared.ReasoningSummary(summary),
	}

	var input []responses.ResponseInputItemUnionParam
	for _, m := range messages {
		items := convertMessageToResponseInput(m)
		input = append(input, items...)
	}
	if len(input) > 0 {
		params.Input = responses.ResponseNewParamsInputUnion{
			OfInputItemList: input,
		}
	}

	if len(opts.Tools) > 0 {
		var tools []responses.ToolUnionParam
		for _, t := range opts.Tools {
			tools = append(tools, convertToolToResponseTool(t))
		}
		params.Tools = tools
		if !skipToolChoice {
			params.ToolChoice = responses.ResponseNewParamsToolChoiceUnion{
				OfToolChoiceMode: param.NewOpt(responses.ToolChoiceOptionsAuto),
			}
		}
	}

	return params
}

// convertMessageToResponseInput converts a provider.Message to Responses API input items.
func convertMessageToResponseInput(m provider.Message) []responses.ResponseInputItemUnionParam {
	var items []responses.ResponseInputItemUnionParam
	switch m.Role {
	case "user":
		var content responses.ResponseInputMessageContentListParam
		for _, part := range m.Parts {
			switch p := part.(type) {
			case provider.ToolResultPart:
				output := p.Output
				if p.Error != nil {
					output = *p.Error
				}
				items = append(items, responses.ResponseInputItemParamOfFunctionCallOutput(p.ToolCallID, output))
			default:
				if inputPart, ok := openAIResponseContentPart(p); ok {
					content = append(content, inputPart)
				}
			}
		}
		if len(content) > 0 {
			items = append(items, responses.ResponseInputItemParamOfMessage(content, responses.EasyInputMessageRoleUser))
		}
	case "assistant":
		for _, raw := range m.Parts {
			switch p := raw.(type) {
			case provider.ToolCallPart:
				items = append(items, responses.ResponseInputItemParamOfFunctionCall(
					p.Arguments, p.ID, p.Name))
			}
		}
		// Responses API rejects assistant message content parts encoded as
		// input_text/input_image/input_file. Replay assistant text as a plain
		// string instead, while preserving tool calls as separate items.
		if text := provider.TextFromParts(m.Parts); text != "" {
			items = append(items, responses.ResponseInputItemParamOfMessage(text, responses.EasyInputMessageRoleAssistant))
		}
	}
	return items
}

// convertToolToResponseTool converts a provider.Tool to a Responses API FunctionTool.
func convertToolToResponseTool(t provider.Tool) responses.ToolUnionParam {
	var schema map[string]any
	if len(t.Parameters) > 0 {
		if err := json.Unmarshal(t.Parameters, &schema); err != nil {
			log.Printf("openai: invalid tool schema for %q: %v", t.Name, err)
		}
	}
	if schema == nil {
		schema = map[string]any{"type": "object", "properties": map[string]any{}}
	}
	return responses.ToolUnionParam{
		OfFunction: &responses.FunctionToolParam{
			Name:        t.Name,
			Description: param.NewOpt(t.Description),
			Parameters:  schema,
		},
	}
}

// consumeResponseStream reads events from a Responses API stream and converts
// them to provider.StreamChunk values on ch. The channel is always closed.
func consumeResponseStream(ctx context.Context, stream *ssestream.Stream[responses.ResponseStreamEventUnion], ch chan<- provider.StreamChunk) {
	defer close(ch)

	var finishReason string
	var hasToolCalls bool
	var inReasoningPart bool // true when current content part is reasoning, not output

	for stream.Next() {
		evt := stream.Current()
		log.Printf("[1-provider] event type=%q delta=%d chars", evt.Type, len(evt.Delta))
		switch evt.Type {
		case "response.content_part.added":
			partType := evt.Part.Type
			if debugStream {
				log.Printf("[openai-stream] content_part.added: type=%q", partType)
			}
			inReasoningPart = partType == "reasoning_text"

		case "response.content_part.done":
			inReasoningPart = false

		case "response.output_text.delta":
			if evt.Delta != "" {
				if inReasoningPart {
					log.Printf("[1-provider] output_text.delta AS ReasoningDelta: %d chars", len(evt.Delta))
					ch <- provider.StreamChunk{ReasoningDelta: evt.Delta}
				} else {
					ch <- provider.StreamChunk{Delta: evt.Delta}
				}
			}

		case "response.output_item.done":
			log.Printf("[openai-stream] output_item.done: item.Type=%q item.Name=%q item.CallID=%q args=%d chars",
				evt.Item.Type, evt.Item.Name, evt.Item.CallID, len(evt.Item.Arguments))
			// Completed function call.
			if evt.Item.Type == "function_call" {
				ch <- provider.StreamChunk{
					ToolCalls: []provider.ToolCall{{
						ID:        evt.Item.CallID,
						Name:      evt.Item.Name,
						Arguments: evt.Item.Arguments,
					}},
				}
				hasToolCalls = true
			}

		case "response.completed":
			log.Printf("[openai-stream] response.completed: status=%q hasToolCalls=%v usage in=%d out=%d",
				evt.Response.Status, hasToolCalls, evt.Response.Usage.InputTokens, evt.Response.Usage.OutputTokens)
			if evt.Response.Status == "completed" {
				if hasToolCalls {
					finishReason = "tool_calls"
				} else {
					finishReason = "stop"
				}
			}
			if evt.Response.Usage.OutputTokens > 0 || evt.Response.Usage.InputTokens > 0 {
				ch <- provider.StreamChunk{
					Usage: &provider.Usage{
						InputTokens:  int(evt.Response.Usage.InputTokens),
						OutputTokens: int(evt.Response.Usage.OutputTokens),
					},
				}
			}

		case "response.reasoning_text.delta":
			log.Printf("[1-provider] reasoning_text.delta: %d chars (empty=%v)", len(evt.Delta), evt.Delta == "")
			if evt.Delta != "" {
				ch <- provider.StreamChunk{ReasoningDelta: evt.Delta}
			}

		case "response.reasoning_summary_text.delta":
			log.Printf("[1-provider] reasoning_summary_text.delta: %d chars", len(evt.Delta))
			if evt.Delta != "" {
				ch <- provider.StreamChunk{ReasoningDelta: evt.Delta}
			}

		case "response.function_call_arguments.delta":
			// Streamed tool call arguments are handled via output_item.done.
			// when complete.
			if debugStream {
				log.Printf("[openai-stream] function_call_arguments delta: %d chars", len(evt.Delta))
			}

		case "error":
			ch <- provider.StreamChunk{Err: fmt.Errorf("openai responses: %s", evt.Message)}
			return

		case "response.reasoning_summary_part.added",
			"response.reasoning_summary_part.done",
			"response.reasoning_summary_text.done":
			// Summary lifecycle events are separate. Text content arrives via the .delta events above.
		}
	}

	log.Printf("[openai-stream] stream loop ended, streamErr=%v finishReason=%q", stream.Err(), finishReason)

	if err := stream.Err(); err != nil {
		select {
		case <-ctx.Done():
		default:
			ch <- provider.StreamChunk{Err: fmt.Errorf("openai responses: stream: %w", err)}
		}
		return
	}

	if finishReason == "" {
		finishReason = "stop"
	}
	ch <- provider.StreamChunk{FinishReason: finishReason}
}
