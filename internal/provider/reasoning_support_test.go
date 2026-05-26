package provider

import "testing"

func TestSupportsReasoningVariantsForTurn(t *testing.T) {
	tests := []struct {
		name         string
		model        ModelRef
		allowedTools []string
		want         bool
	}{
		{
			name:  "openai gpt5",
			model: ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			want:  true,
		},
		{
			name:  "openai codex alias",
			model: ModelRef{ProviderID: "openai-codex", ModelID: "gpt-5.3-codex"},
			want:  true,
		},
		{
			name:  "openai gpt41",
			model: ModelRef{ProviderID: "openai", ModelID: "gpt-4.1"},
			want:  false,
		},
		{
			name:  "nvidia gpt oss",
			model: ModelRef{ProviderID: "nvidia", ModelID: "openai/gpt-oss-20b"},
			want:  true,
		},
		{
			name:  "openrouter openai gpt5",
			model: ModelRef{ProviderID: "openrouter", ModelID: "openai/gpt-5"},
			want:  true,
		},
		{
			name:  "google gemini 2.5",
			model: ModelRef{ProviderID: "google", ModelID: "gemini-2.5-pro"},
			want:  true,
		},
		{
			name:  "google gemini 2.5 flash lite",
			model: ModelRef{ProviderID: "google", ModelID: "gemini-2.5-flash-lite"},
			want:  true,
		},
		{
			name:  "google gemini 3 flash",
			model: ModelRef{ProviderID: "google", ModelID: "gemini-3-flash"},
			want:  true,
		},
		{
			name:  "anthropic opus 4.5",
			model: ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
			want:  true,
		},
		{
			name:         "anthropic opus 4.5 with tools still supports effort",
			model:        ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
			allowedTools: []string{"read"},
			want:         true,
		},
		{
			name:  "anthropic claude 3.5 haiku",
			model: ModelRef{ProviderID: "anthropic", ModelID: "claude-3-5-haiku-latest"},
			want:  false,
		},
		{
			name:  "mistral medium 3.5",
			model: ModelRef{ProviderID: "mistral", ModelID: "mistral-medium-2604"},
			want:  true,
		},
		{
			name:  "openrouter qwen3",
			model: ModelRef{ProviderID: "openrouter", ModelID: "qwen/qwen3.5-32b"},
			want:  true,
		},
		{
			name:  "qwencloud qwen3",
			model: ModelRef{ProviderID: "qwencloud", ModelID: "qwen3.6-plus"},
			want:  true,
		},
		{
			name:  "compatible unnamespaced openai model hidden",
			model: ModelRef{ProviderID: "openrouter", ModelID: "gpt-5"},
			want:  false,
		},
		{
			name:  "compatible openai gpt41 hidden",
			model: ModelRef{ProviderID: "openrouter", ModelID: "openai/gpt-4.1"},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SupportsReasoningVariantsForTurn(tt.model, tt.allowedTools); got != tt.want {
				t.Fatalf("SupportsReasoningVariantsForTurn(%+v, %#v) = %v, want %v", tt.model, tt.allowedTools, got, tt.want)
			}
		})
	}
}

func TestSupportsThinkingOutputForTurn(t *testing.T) {
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "openai", ModelID: "gpt-5"}, []string{"read"}) {
		t.Fatal("openai gpt-5 should support surfaced thinking output")
	}
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "openai-codex", ModelID: "gpt-5.3-codex"}, []string{"read"}) {
		t.Fatal("openai-codex gpt-5.3-codex should support surfaced thinking output")
	}
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "openrouter", ModelID: "openai/gpt-5.3-codex"}, []string{"read"}) {
		t.Fatal("openrouter openai/gpt-5.3-codex should support surfaced thinking output")
	}
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}, []string{"read"}) {
		t.Fatal("anthropic tool-enabled turns should support surfaced thinking output")
	}
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "anthropic", ModelID: "claude-sonnet-4-6"}, nil) {
		t.Fatal("anthropic no-tool turn should support surfaced thinking output")
	}
	if SupportsThinkingOutputForTurn(ModelRef{ProviderID: "mistral", ModelID: "mistral-medium-2604"}, nil) {
		t.Fatal("mistral adjustable reasoning should not expose a separate thinking toggle")
	}
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"}, []string{"read"}) {
		t.Fatal("deepseek-v4-pro should support surfaced thinking output")
	}
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "openrouter", ModelID: "qwen/qwen3.5-32b"}, []string{"read"}) {
		t.Fatal("compatible qwen/qwen3.5-32b should support surfaced thinking output")
	}
	if !SupportsThinkingOutputForTurn(ModelRef{ProviderID: "qwencloud", ModelID: "qwen3.6-plus"}, []string{"read"}) {
		t.Fatal("qwencloud qwen3.6-plus should support surfaced thinking output")
	}
	if SupportsThinkingOutputForTurn(ModelRef{ProviderID: "deepseek", ModelID: "deepseek-chat"}, []string{"read"}) {
		t.Fatal("deepseek-chat should not expose surfaced thinking output")
	}
	if SupportsThinkingOutputFromCatalog(ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"}, true) {
		t.Fatal("deepseek should no longer depend on catalog fallback for thinking support")
	}
}

func TestEffectiveReasoningVariantForTurn(t *testing.T) {
	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "anthropic", ModelID: "claude-opus-4-5"},
		[]string{"read"},
		ReasoningVariantHigh,
	); got != ReasoningVariantHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want high", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "nvidia", ModelID: "stepfun-ai/step-3.5-flash"},
		[]string{"read"},
		ReasoningVariantHigh,
	); got != "" {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want empty for unsupported model", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "openai", ModelID: "gpt-5.2"},
		[]string{"read"},
		ReasoningVariantXHigh,
	); got != ReasoningVariantXHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want xhigh", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "openai-codex", ModelID: "gpt-5.3-codex"},
		[]string{"read"},
		ReasoningVariantMedium,
	); got != ReasoningVariantMedium {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want medium for openai-codex alias", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "openrouter", ModelID: "openai/gpt-5.3-codex"},
		[]string{"read"},
		ReasoningVariantMedium,
	); got != ReasoningVariantMedium {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want medium for openrouter openai model", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		[]string{"read"},
		"",
	); got != "" {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want empty", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "google", ModelID: "gemini-2.5-flash"},
		[]string{"read"},
		"1024",
	); got != "1024" {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want 1024", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "google", ModelID: "gemini-2.5-flash"},
		[]string{"read"},
		"0",
	); got != "0" {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want 0", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "google", ModelID: "gemini-3-pro"},
		[]string{"read"},
		ReasoningVariantMedium,
	); got != ReasoningVariantMedium {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want medium", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "google", ModelID: "gemini-2.5-pro"},
		[]string{"read"},
		"0",
	); got != "" {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want empty", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
		[]string{"read"},
		ReasoningVariantXHigh,
	); got != ReasoningVariantXHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want xhigh", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "openai", ModelID: "gpt-5.1-codex-max"},
		[]string{"read"},
		ReasoningVariantLow,
	); got != "" {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want empty", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "mistral", ModelID: "mistral-medium-2604"},
		nil,
		ReasoningVariantHigh,
	); got != ReasoningVariantHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want high for mistral-medium-2604", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
		nil,
		ReasoningVariantMedium,
	); got != ReasoningVariantHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want high for deepseek medium compatibility", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "deepseek", ModelID: "deepseek-v4-pro"},
		nil,
		ReasoningVariantXHigh,
	); got != ReasoningVariantXHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want xhigh for deepseek-v4-pro", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "openrouter", ModelID: "qwen/qwen3.5-32b"},
		nil,
		ReasoningVariantHigh,
	); got != ReasoningVariantHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want high for compatible qwen model", got)
	}

	if got := EffectiveReasoningVariantForTurn(
		ModelRef{ProviderID: "qwencloud", ModelID: "qwen3.6-plus"},
		nil,
		ReasoningVariantHigh,
	); got != ReasoningVariantHigh {
		t.Fatalf("EffectiveReasoningVariantForTurn() = %q, want high for qwencloud qwen model", got)
	}
}

func TestNormalizeCatalogModelCapabilitiesMarksNativeMistralReasoningModels(t *testing.T) {
	model := NormalizeCatalogModelCapabilities("mistral", CatalogModel{
		ID: "magistral-medium-2509",
	})
	if !model.Reasoning {
		t.Fatal("Reasoning = false, want true for magistral-medium-2509")
	}
	if len(model.SupportedReasoningVariants) != 0 {
		t.Fatalf("SupportedReasoningVariants = %#v, want none for magistral-medium-2509", model.SupportedReasoningVariants)
	}
}
