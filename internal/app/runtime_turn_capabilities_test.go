package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestRuntimeResolveSessionTurnCapabilitiesPreservesAnthropicVariantsWhenToolsAreAllowed(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"},
	}
	runtime.Config.Anthropic.APIKey = "test-key"

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v", err)
	}
	if len(capabilities.AllowedTools) == 0 {
		t.Fatalf("allowed tools = %#v, want tool-enabled builder surface", capabilities.AllowedTools)
	}
	if !capabilities.SupportsReasoningVariants() {
		t.Fatalf("SupportsReasoningVariants() = false, want true for anthropic tool-enabled turn")
	}
	if !capabilities.SupportsThinkingOutput() {
		t.Fatalf("SupportsThinkingOutput() = false, want true for anthropic tool-enabled turn")
	}
}

func TestRuntimeResolveSessionTurnCapabilitiesPreservesOpenAICodexVariants(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai-codex", ModelID: "gpt-5.3-codex"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v", err)
	}
	if len(capabilities.AllowedTools) == 0 {
		t.Fatalf("allowed tools = %#v, want tool-enabled builder surface", capabilities.AllowedTools)
	}
	if !capabilities.SupportsReasoningVariants() {
		t.Fatalf("SupportsReasoningVariants() = false, want true for openai-codex")
	}
	if !capabilities.SupportsThinkingOutput() {
		t.Fatalf("SupportsThinkingOutput() = false, want true for openai-codex")
	}
	if got := capabilities.EffectiveReasoningVariant(provider.ReasoningVariantMedium); got != provider.ReasoningVariantMedium {
		t.Fatalf("EffectiveReasoningVariant(medium) = %q, want medium", got)
	}
	if got := capabilities.EffectiveReasoningVariant(""); got != provider.ReasoningVariantMedium {
		t.Fatalf("EffectiveReasoningVariant(empty) = %q, want medium default for openai-codex", got)
	}
}

func TestRuntimeResolveSessionTurnCapabilitiesSupportsCompatibleOpenAIModels(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.CompatibleProviders = map[string]OpenAICompatibleProviderConfig{
		"openrouter": {ProviderID: "openrouter", APIKey: "test-key", BaseURL: "https://openrouter.ai/api/v1"},
	}
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openrouter", ModelID: "openai/gpt-5.3-codex"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v", err)
	}
	if !capabilities.SupportsReasoningVariants() {
		t.Fatalf("SupportsReasoningVariants() = false, want true for compatible OpenAI model")
	}
	if !capabilities.SupportsThinkingOutput() {
		t.Fatalf("SupportsThinkingOutput() = false, want true for compatible OpenAI model")
	}
	if got := capabilities.EffectiveReasoningVariant(provider.ReasoningVariantMedium); got != provider.ReasoningVariantMedium {
		t.Fatalf("EffectiveReasoningVariant(medium) = %q, want medium", got)
	}
	if got := capabilities.EffectiveReasoningVariant(""); got != provider.ReasoningVariantMedium {
		t.Fatalf("EffectiveReasoningVariant(empty) = %q, want medium default for compatible OpenAI codex model", got)
	}
}

func TestRuntimeResolveSessionTurnCapabilitiesAllowsAnthropicReasoningForNoToolAgent(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reasoner.md"), []byte(`---
description: anthropic reasoning only
model: anthropic/claude-opus-4-5
AllowTools: []
---

You are the reasoning agent.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.Anthropic.APIKey = "test-key"
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "reasoner",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v", err)
	}
	if len(capabilities.AllowedTools) != 0 {
		t.Fatalf("allowed tools = %#v, want none", capabilities.AllowedTools)
	}
	if !capabilities.SupportsReasoningVariants() {
		t.Fatal("SupportsReasoningVariants() = false, want true for no-tool anthropic turn")
	}
}

func TestRuntimeResolveSessionTurnCapabilitiesAppliesDisallowedToolsWithoutAllowList(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reviewer.md"), []byte(`---
description: reviewer
DisallowedTools:
  - write
  - apply_patch
---

Review only.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	runtime := newRuntimeWithClient(t, &fakeProvider{})
	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "reviewer",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v", err)
	}
	if len(capabilities.AllowedTools) == 0 {
		t.Fatalf("allowed tools = %#v, want non-empty runtime surface", capabilities.AllowedTools)
	}
	for _, name := range capabilities.AllowedTools {
		if name == "write" || name == "apply_patch" {
			t.Fatalf("allowed tools = %#v, want disallowed tools removed", capabilities.AllowedTools)
		}
	}
}

func TestRuntimeResolveSessionTurnCapabilitiesDoesNotRequireProviderAuth(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "google", ModelID: "gemini-2.5-flash"},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v, want auth-free capability resolution", err)
	}
	if capabilities.ModelRoute.Primary.ProviderID != "google" || capabilities.ModelRoute.Primary.ModelID != "gemini-2.5-flash" {
		t.Fatalf("capabilities model route = %#v", capabilities.ModelRoute)
	}
}

func TestRuntimeResolveSessionTurnCapabilitiesUsesPersistedSessionModelRoute(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}
	runtime.Config.OpenAI.APIKey = "test-key"
	runtime.Config.DeepSeek.APIKey = "deepseek-key"

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if err := runtime.SetSessionModelRoute(context.Background(), sessionID, provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-chat"},
	}); err != nil {
		t.Fatalf("SetSessionModelRoute() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v", err)
	}
	if got := capabilities.ModelRoute.Primary.String(); got != "deepseek/deepseek-chat" {
		t.Fatalf("capabilities model route = %q, want deepseek/deepseek-chat", got)
	}
}

func TestRuntimeResolveSessionTurnCapabilitiesSupportsDeepSeekThinkingOutput(t *testing.T) {
	runtime := newRuntimeWithClient(t, &fakeProvider{})
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
	}
	runtime.Config.DeepSeek.APIKey = "deepseek-key"
	runtime.ModelCatalog = &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"deepseek": {{
				ID:        "deepseek-v4-pro",
				Name:      "DeepSeek V4 Pro",
				Reasoning: true,
			}},
		},
	}

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	capabilities, err := runtime.ResolveSessionTurnCapabilities(context.Background(), ResolveSessionTurnCapabilitiesInput{
		SessionID: sessionID,
		AgentID:   "builder",
	})
	if err != nil {
		t.Fatalf("ResolveSessionTurnCapabilities() error = %v", err)
	}
	if !capabilities.SupportsThinkingOutput() {
		t.Fatal("SupportsThinkingOutput() = false, want true for deepseek-v4-pro")
	}
	if got := capabilities.EffectiveReasoningVariant(provider.ReasoningVariantXHigh); got != provider.ReasoningVariantXHigh {
		t.Fatalf("EffectiveReasoningVariant(xhigh) = %q, want xhigh for deepseek-v4-pro", got)
	}
	if !capabilities.SupportsReasoningVariants() {
		t.Fatal("SupportsReasoningVariants() = false, want true for deepseek-v4-pro")
	}
	if capabilities.EffectiveThinkingEnabled(true) != true {
		t.Fatal("EffectiveThinkingEnabled(true) = false, want true for deepseek-v4-pro")
	}
}

func TestRuntimeRunExistingSessionTurnPreservesAnthropicVariantWhenToolsAreAllowed(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
	}
	runtime.Config.Anthropic.APIKey = "test-key"

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:    sessionID,
		TurnID:       "turn-1",
		UserText:     "build it",
		AgentID:      "builder",
		ThinkingMode: provider.ReasoningVariantHigh,
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].ThinkingMode; got != provider.ReasoningVariantHigh {
		t.Fatalf("request thinking mode = %q, want high", got)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn config missing: %#v", turn)
	}
	if got := turn.Config.ThinkingMode; got != provider.ReasoningVariantHigh {
		t.Fatalf("stored thinking mode = %q, want high", got)
	}
}

func TestRuntimeRunExistingSessionTurnDropsUnsupportedVariant(t *testing.T) {
	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "nvidia", ModelID: "stepfun-ai/step-3.5-flash"},
	}
	runtime.Config.NVIDIA.APIKey = "test-key"

	sessionID, err := runtime.CreateSession(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:    sessionID,
		TurnID:       "turn-1",
		UserText:     "build it",
		AgentID:      "builder",
		ThinkingMode: provider.ReasoningVariantHigh,
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].ThinkingMode; got != "" {
		t.Fatalf("request thinking mode = %q, want empty", got)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	turn := state.Turns["turn-1"]
	if turn == nil || turn.Config == nil {
		t.Fatalf("turn config missing: %#v", turn)
	}
	if got := turn.Config.ThinkingMode; got != "" {
		t.Fatalf("stored thinking mode = %q, want empty", got)
	}
}

func TestRuntimeRunExistingSessionTurnPreservesAnthropicThinkingModeForNoToolAgent(t *testing.T) {
	root := t.TempDir()
	agentDir := filepath.Join(root, ".kodacode", "agents")
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(agentDir, "reasoner.md"), []byte(`---
description: anthropic reasoning only
model: anthropic/claude-opus-4-5
---

You are the reasoning agent.
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{provider.NewSliceStream([]provider.Event{
			{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
		})},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Config.Anthropic.APIKey = "test-key"

	sessionID, err := runtime.CreateSession(context.Background(), root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID:    sessionID,
		TurnID:       "turn-1",
		UserText:     "think through it",
		AgentID:      "reasoner",
		ThinkingMode: provider.ReasoningVariantHigh,
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(client.requests) != 1 {
		t.Fatalf("provider requests = %d, want 1", len(client.requests))
	}
	if got := client.requests[0].ThinkingMode; got != provider.ReasoningVariantHigh {
		t.Fatalf("request thinking mode = %q, want high", got)
	}
}
