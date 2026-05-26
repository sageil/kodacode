package provider

import (
	"encoding/json"
	"testing"

	"google.golang.org/genai"
)

func TestProviderToolRequestsExposeParallelToolUseWhereSupported(t *testing.T) {
	t.Run("openai responses", func(t *testing.T) {
		payload, err := buildOpenAIRequest(providerParallelValidationRequest("openai", "gpt-5"))
		if err != nil {
			t.Fatalf("buildOpenAIRequest() error = %v", err)
		}
		if !payload.ParallelToolCalls {
			t.Fatal("ParallelToolCalls = false, want true")
		}
	})

	t.Run("openai compatible chat completions", func(t *testing.T) {
		payload, err := buildOpenAIChatCompletionsRequest(providerParallelValidationRequest("openai-compatible", "custom-model"), true)
		if err != nil {
			t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
		}
		if !payload.ParallelToolCalls {
			t.Fatal("ParallelToolCalls = false, want true")
		}
	})

	t.Run("github copilot responses gpt", func(t *testing.T) {
		payload, err := buildOpenAIRequest(providerParallelValidationRequest("github-copilot", "gpt-5-mini"))
		if err != nil {
			t.Fatalf("buildOpenAIRequest() error = %v", err)
		}
		if !payload.ParallelToolCalls {
			t.Fatal("ParallelToolCalls = false, want true")
		}
	})

	t.Run("github copilot chat completions gpt", func(t *testing.T) {
		payload, err := buildOpenAIChatCompletionsRequest(providerParallelValidationRequest("github-copilot", "gpt-5.2"), true)
		if err != nil {
			t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
		}
		if !payload.ParallelToolCalls {
			t.Fatal("ParallelToolCalls = false, want true")
		}
	})

	t.Run("github copilot chat completions gemini omits unsupported flag", func(t *testing.T) {
		payload, err := buildOpenAIChatCompletionsRequest(providerParallelValidationRequest("github-copilot", "gemini-2.5-pro"), true)
		if err != nil {
			t.Fatalf("buildOpenAIChatCompletionsRequest() error = %v", err)
		}
		if payload.ParallelToolCalls {
			t.Fatal("ParallelToolCalls = true, want false for Copilot Gemini chat completions")
		}
	})

	t.Run("anthropic messages", func(t *testing.T) {
		params, err := buildAnthropicParams(providerParallelValidationRequest("anthropic", "claude-sonnet-4-5-20250929"))
		if err != nil {
			t.Fatalf("buildAnthropicParams() error = %v", err)
		}
		if params.ToolChoice.OfAuto == nil {
			t.Fatalf("ToolChoice = %#v, want auto", params.ToolChoice)
		}
		disabled := params.ToolChoice.OfAuto.DisableParallelToolUse
		if !disabled.Valid() || disabled.Value {
			t.Fatalf("DisableParallelToolUse = %#v, want explicit false", disabled)
		}
	})

	t.Run("google generate content", func(t *testing.T) {
		config, err := buildGoogleGenerateContentConfig(providerParallelValidationRequest("google", "gemini-2.5-pro"))
		if err != nil {
			t.Fatalf("buildGoogleGenerateContentConfig() error = %v", err)
		}
		if len(config.Tools) != 1 || len(config.Tools[0].FunctionDeclarations) != 1 {
			t.Fatalf("Tools = %#v, want one function declaration", config.Tools)
		}
		if config.ToolConfig == nil || config.ToolConfig.FunctionCallingConfig == nil {
			t.Fatalf("ToolConfig = %#v, want function calling config", config.ToolConfig)
		}
		if config.ToolConfig.FunctionCallingConfig.Mode != genai.FunctionCallingConfigModeAuto {
			t.Fatalf("FunctionCallingConfig.Mode = %q, want AUTO", config.ToolConfig.FunctionCallingConfig.Mode)
		}
	})
}

func TestGitHubCopilotProviderRoutePrefersParallelCapableEndpointForGPT(t *testing.T) {
	apis := gitHubCopilotAPIs(defaultGitHubCopilotBaseURL, "gpt-5-mini")
	if len(apis) == 0 {
		t.Fatal("gitHubCopilotAPIs returned no endpoints")
	}
	if apis[0].Mode != openAICompatibleModeResponses {
		t.Fatalf("first Copilot GPT endpoint = %q, want responses", apis[0].Mode)
	}
	trace := openAICompatibleRequestTrace(providerParallelValidationRequest("github-copilot", "gpt-5-mini"), apis[0])
	if trace.APIMode != string(openAICompatibleModeResponses) || !trace.ParallelToolCalls {
		t.Fatalf("trace = %#v, want responses with parallel tool calls", trace)
	}
}

func providerParallelValidationRequest(providerID, modelID string) Request {
	schema := json.RawMessage(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	return Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Model: ModelRef{
			ProviderID: providerID,
			ModelID:    modelID,
		},
		Instructions: "Use tools when needed.",
		Inputs: []Input{{
			Kind:    InputKindUserMessage,
			Content: "Read a file.",
		}},
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file.",
			InputSchema: string(schema),
		}},
	}
}
