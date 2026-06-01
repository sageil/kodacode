package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

type extensionTestTool struct {
	definition tool.Definition
}

func (t extensionTestTool) Definition() tool.Definition {
	return t.definition
}

func (t extensionTestTool) Execute(context.Context, tool.ExecutionContext, json.RawMessage) (tool.Result, error) {
	return tool.Result{Output: "ok"}, nil
}

func TestRuntimeExtensionSurfaceCollectsToolsAndMetadata(t *testing.T) {
	hook := &recordingPrecomputeHook{calls: make(chan RuntimePrecomputeHint, 1)}
	middleware := provider.Middleware(func(next provider.ProviderHandler) provider.ProviderHandler {
		return next
	})

	surface, err := buildRuntimeExtensionSurface([]RuntimeExtensionRegistration{{
		ID: "metadata",
		Tools: []RuntimeExtensionToolRegistration{{
			Tool: extensionTestTool{definition: tool.Definition{
				Name:        "metadata_scan",
				Description: "Scan metadata",
				InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
			Effects: []tool.ExecutionEffect{tool.ExecutionEffectRead},
		}},
		PrecomputeHooks: []RuntimeExtensionPrecomputeHookRegistration{{
			ID:   "metadata_refresh",
			Hook: hook,
		}},
		ContextContributions: []RuntimeExtensionContextContribution{{
			ID:          "metadata_context",
			Description: "Metadata summaries",
		}},
		ProviderMiddleware: []RuntimeExtensionProviderMiddlewareRegistration{{
			ID:         "metadata_audit",
			Middleware: middleware,
		}},
	}})
	if err != nil {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v", err)
	}
	if len(surface.Tools) != 1 || surface.Tools[0].Definition().Name != "metadata_scan" {
		t.Fatalf("surface tools = %#v", surface.Tools)
	}
	if got := surface.ToolEffects["metadata_scan"]; len(got) != 1 || got[0] != tool.ExecutionEffectRead {
		t.Fatalf("surface tool effects = %#v", surface.ToolEffects)
	}
	if len(surface.PrecomputeHooks) != 1 || surface.PrecomputeHooks[0] != hook {
		t.Fatalf("surface precompute hooks = %#v", surface.PrecomputeHooks)
	}
	if len(surface.ContextContributions) != 1 || surface.ContextContributions[0].ID != "metadata_context" {
		t.Fatalf("surface context contributions = %#v", surface.ContextContributions)
	}
	if len(surface.ProviderMiddleware) != 1 || surface.ProviderMiddleware[0] == nil {
		t.Fatalf("surface provider middleware = %#v", surface.ProviderMiddleware)
	}
}

func TestRegisterRuntimeExtensionAddsRegisteredSurface(t *testing.T) {
	resetRuntimeExtensionsForTest(t)
	effects := []tool.ExecutionEffect{tool.ExecutionEffectRead}

	RegisterRuntimeExtension(RuntimeExtensionRegistration{
		ID: "metadata",
		Tools: []RuntimeExtensionToolRegistration{{
			Tool: extensionTestTool{definition: tool.Definition{
				Name:        "metadata_scan",
				Description: "Scan metadata",
				InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
			Effects: effects,
		}},
	})
	effects[0] = tool.ExecutionEffectWrite

	surface, err := buildRuntimeExtensionSurface(registeredRuntimeExtensions())
	if err != nil {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v", err)
	}
	if len(surface.Tools) != 1 || surface.Tools[0].Definition().Name != "metadata_scan" {
		t.Fatalf("surface tools = %#v", surface.Tools)
	}
	if got := surface.ToolEffects["metadata_scan"]; len(got) != 1 || got[0] != tool.ExecutionEffectRead {
		t.Fatalf("surface tool effects = %#v, want cloned read effect", surface.ToolEffects)
	}
}

func TestBuildRuntimeToolsIncludesExtensionTools(t *testing.T) {
	extensionTool := extensionTestTool{definition: tool.Definition{
		Name:        "metadata_scan",
		Description: "Scan metadata",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}

	tools, err := buildRuntimeTools(nil, []tool.Tool{extensionTool})
	if err != nil {
		t.Fatalf("buildRuntimeTools() error = %v", err)
	}
	if !runtimeToolNamesInclude(tools, "metadata_scan") {
		t.Fatalf("runtime tools missing extension tool: %v", runtimeToolNames(tools))
	}
}

func TestBuildRuntimeToolsRejectsExtensionToolOverride(t *testing.T) {
	extensionTool := extensionTestTool{definition: tool.Definition{
		Name:        tool.ReadToolName,
		Description: "Override read",
		InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
	}}

	_, err := buildRuntimeTools(nil, []tool.Tool{extensionTool})
	if !errors.Is(err, ErrRuntimeExtensionToolDuplicate) {
		t.Fatalf("buildRuntimeTools() error = %v, want ErrRuntimeExtensionToolDuplicate", err)
	}
}

func TestRuntimeExtensionToolRequiresDeclaredEffects(t *testing.T) {
	_, err := buildRuntimeExtensionSurface([]RuntimeExtensionRegistration{{
		ID: "metadata",
		Tools: []RuntimeExtensionToolRegistration{{
			Tool: extensionTestTool{definition: tool.Definition{
				Name:        "metadata_scan",
				Description: "Scan metadata",
				InputSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
			}},
		}},
	}})
	if !errors.Is(err, ErrRuntimeExtensionToolEffectMissing) {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v, want ErrRuntimeExtensionToolEffectMissing", err)
	}
}

func TestRuntimeExtensionProviderMiddlewareRequiresIDAndMiddleware(t *testing.T) {
	_, err := buildRuntimeExtensionSurface([]RuntimeExtensionRegistration{{
		ID: "metadata",
		ProviderMiddleware: []RuntimeExtensionProviderMiddlewareRegistration{{
			Middleware: func(next provider.ProviderHandler) provider.ProviderHandler { return next },
		}},
	}})
	if !errors.Is(err, ErrRuntimeExtensionProviderMiddlewareIDRequired) {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v, want ErrRuntimeExtensionProviderMiddlewareIDRequired", err)
	}

	_, err = buildRuntimeExtensionSurface([]RuntimeExtensionRegistration{{
		ID: "metadata",
		ProviderMiddleware: []RuntimeExtensionProviderMiddlewareRegistration{{
			ID: "metadata_audit",
		}},
	}})
	if !errors.Is(err, ErrRuntimeExtensionProviderMiddlewareRequired) {
		t.Fatalf("buildRuntimeExtensionSurface() error = %v, want ErrRuntimeExtensionProviderMiddlewareRequired", err)
	}
}

func TestRuntimeUsesExtensionProviderMiddleware(t *testing.T) {
	resetRuntimeExtensionsForTest(t)
	calls := make(chan provider.Request, 2)
	RegisterRuntimeExtension(RuntimeExtensionRegistration{
		ID: "provider_intercept",
		ProviderMiddleware: []RuntimeExtensionProviderMiddlewareRegistration{{
			ID: "provider_intercept_stream",
			Middleware: func(provider.ProviderHandler) provider.ProviderHandler {
				return runtimeExtensionProviderHandler{
					streamFunc: func(_ context.Context, req provider.Request) (provider.Stream, error) {
						calls <- req
						return provider.NewSliceStream([]provider.Event{{
							Kind:           provider.EventKindAssistantDelta,
							AssistantDelta: "intercepted",
						}}), nil
					},
					countFunc: func(context.Context, provider.Request) (int, provider.TokenCountSource, error) {
						return 1, provider.TokenCountSourceEstimated, nil
					},
				}
			},
		}},
	})

	runtime, err := NewRuntime(Config{
		ModelRoute: provider.ModelRoute{
			Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		},
		OpenAI: OpenAIProviderConfig{
			APIKey:  "test-key",
			BaseURL: "http://example.invalid/v1/responses",
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if closeErr := runtime.Close(); closeErr != nil {
			t.Fatalf("runtime.Close() error = %v", closeErr)
		}
	})

	req := provider.Request{
		SessionID:    "session-1",
		TurnID:       "turn-1",
		AgentID:      "engineer",
		Model:        provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Instructions: "respond",
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: "hello",
		}},
	}
	stream, err := runtime.Provider.Stream(context.Background(), req)
	if err != nil {
		t.Fatalf("Provider.Stream() error = %v", err)
	}
	event, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv() error = %v", err)
	}
	if event.AssistantDelta != "intercepted" {
		t.Fatalf("assistant delta = %q, want intercepted", event.AssistantDelta)
	}
	select {
	case req := <-calls:
		if req.Model.ProviderID != "openai" || req.Model.ModelID != "gpt-5" {
			t.Fatalf("middleware request model = %s, want openai/gpt-5", req.Model.String())
		}
	default:
		t.Fatal("extension provider middleware was not called")
	}
}

func TestRuntimePrecomputeHooksIncludesExtensionHooks(t *testing.T) {
	hook := &recordingPrecomputeHook{calls: make(chan RuntimePrecomputeHint, 1)}
	runtime := &Runtime{extensionPrecomputeHooks: []RuntimePrecomputeHook{hook}}

	hooks := runtime.runtimePrecomputeHooks()
	if len(hooks) != 1 || hooks[0] != hook {
		t.Fatalf("runtimePrecomputeHooks() = %#v, want extension hook", hooks)
	}
}

type runtimeExtensionProviderHandler struct {
	streamFunc func(context.Context, provider.Request) (provider.Stream, error)
	countFunc  func(context.Context, provider.Request) (int, provider.TokenCountSource, error)
}

func (h runtimeExtensionProviderHandler) Stream(ctx context.Context, req provider.Request) (provider.Stream, error) {
	return h.streamFunc(ctx, req)
}

func (h runtimeExtensionProviderHandler) CountTokens(ctx context.Context, req provider.Request) (int, provider.TokenCountSource, error) {
	return h.countFunc(ctx, req)
}

func runtimeToolNamesInclude(tools []tool.Tool, target string) bool {
	for _, tl := range tools {
		if tl.Definition().Name == target {
			return true
		}
	}
	return false
}

func runtimeToolNames(tools []tool.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tl := range tools {
		names = append(names, tl.Definition().Name)
	}
	return names
}

func resetRuntimeExtensionsForTest(t *testing.T) {
	t.Helper()
	runtimeExtensionsMu.Lock()
	previous := runtimeExtensions
	runtimeExtensions = map[string]RuntimeExtensionRegistration{}
	runtimeExtensionsMu.Unlock()
	t.Cleanup(func() {
		runtimeExtensionsMu.Lock()
		runtimeExtensions = previous
		runtimeExtensionsMu.Unlock()
	})
}
