package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestCompactionThresholdTokensCapsTriggerAtConfiguredBudgetThreshold(t *testing.T) {
	triggerTokens, targetTokens := compactionThresholdTokens(128000, SessionConfig{})
	if triggerTokens != 102400 {
		t.Fatalf("trigger tokens = %d, want 102400", triggerTokens)
	}
	if targetTokens != 76800 {
		t.Fatalf("target tokens = %d, want 76800", targetTokens)
	}
}

func TestCompactionThresholdTokensUsesConfiguredThresholdsDirectly(t *testing.T) {
	triggerTokens, targetTokens := compactionThresholdTokens(2048, SessionConfig{
		CompactionThreshold:       0.75,
		CompactionTargetThreshold: 0.50,
	})
	if triggerTokens != 1536 {
		t.Fatalf("trigger tokens = %d, want 1536", triggerTokens)
	}
	if targetTokens != 1024 {
		t.Fatalf("target tokens = %d, want 1024", targetTokens)
	}
}

func TestRequestInputTriggerExceededPressureUsesConfiguredTrigger(t *testing.T) {
	models := &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:             "gpt-5",
				ContextSize:    10000,
				MaxInputTokens: 10000,
			}},
		},
	}
	request := provider.Request{
		Model: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: strings.Repeat("x", 3400),
		}},
	}

	pressure := requestInputTriggerExceededPressure(request, models, SessionConfig{
		CompactionThreshold:       0.10,
		CompactionTargetThreshold: 0.25,
	})
	if pressure == nil {
		t.Fatal("pressure = nil, want trigger pressure")
	}
	if pressure.Payload.Reason != "input_limit_pressure" {
		t.Fatalf("reason = %q, want input_limit_pressure", pressure.Payload.Reason)
	}
	if pressure.Payload.TriggerTokens != 1000 || pressure.Payload.InputLimitTokens != 10000 {
		t.Fatalf("payload budget = %#v, want trigger=1000 limit=10000", pressure.Payload)
	}
	if pressure.Payload.EstimatedRequestTokens <= pressure.Payload.TriggerTokens ||
		pressure.Payload.EstimatedRequestTokens > pressure.Payload.InputLimitTokens {
		t.Fatalf("estimated request tokens = %d, want within trigger pressure band", pressure.Payload.EstimatedRequestTokens)
	}
}

func TestRequestInputBudgetDecisionUsesExactProviderCountNearTarget(t *testing.T) {
	models := &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:             "gpt-5",
				ContextSize:    1000,
				MaxInputTokens: 1000,
			}},
		},
	}
	request := provider.Request{
		Model: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: strings.Repeat("x", 3200),
		}},
	}
	client := &fakeProvider{
		counts:       []int{790},
		countSources: []provider.TokenCountSource{provider.TokenCountSourceExact},
	}

	decision := requestInputBudgetDecisionForProviderRequest(context.Background(), client, request, models, SessionConfig{
		CompactionThreshold:       0.80,
		CompactionTargetThreshold: 0.50,
	})
	if decision.LimitFailure != nil || decision.Pressure != nil {
		t.Fatalf("decision = %#v, want no pressure from exact count below trigger", decision)
	}
	if decision.TokenSource != provider.TokenCountSourceExact {
		t.Fatalf("token source = %q, want exact", decision.TokenSource)
	}
	if len(client.countRequests) != 1 {
		t.Fatalf("count requests = %d, want 1", len(client.countRequests))
	}
}

func TestRequestInputBudgetDecisionAppliesSafetyMarginToEstimatedCounts(t *testing.T) {
	models := &fakeModelCatalog{
		modelsByID: map[string][]provider.CatalogModel{
			"openai": {{
				ID:             "tiny-test-model",
				ContextSize:    2400,
				MaxInputTokens: 2400,
			}},
		},
	}
	request := provider.Request{
		Model: provider.ModelRef{ProviderID: "openai", ModelID: "tiny-test-model"},
		Inputs: []provider.Input{{
			Kind:    provider.InputKindUserMessage,
			Content: strings.Repeat("x", 5200),
		}},
	}

	decision := requestInputBudgetDecisionForProviderRequest(context.Background(), nil, request, models, SessionConfig{
		CompactionThreshold:       0.85,
		CompactionTargetThreshold: 0.50,
	})
	if decision.Pressure == nil {
		triggerTokens, targetTokens := compactionThresholdTokens(2400, SessionConfig{
			CompactionThreshold:       0.85,
			CompactionTargetThreshold: 0.50,
		})
		t.Fatalf("pressure = nil, want estimated safety margin to cross trigger; raw=%d shaped=%d trigger=%d target=%d source=%q",
			provider.EstimateRequestTokens(request),
			estimateRequestTokensWithInputs(request, request.Inputs),
			triggerTokens,
			targetTokens,
			decision.TokenSource,
		)
	}
	if decision.LimitFailure != nil {
		t.Fatalf("limit failure = %#v, want pressure only", decision.LimitFailure)
	}
	if decision.TokenSource != provider.TokenCountSourceEstimated {
		t.Fatalf("token source = %q, want estimated", decision.TokenSource)
	}
	if got, want := decision.Pressure.Payload.EstimatedRequestTokens, applyEstimatedInputBudgetSafetyMargin(estimateRequestTokensWithInputs(request, request.Inputs)); got != want {
		t.Fatalf("estimated request tokens = %d, want safety-margined %d", got, want)
	}
}
