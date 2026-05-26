package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

const (
	currentTurnInputLimitSourceCatalog = "catalog_primary"
	currentTurnInputLimitSourceRoute   = "route_candidate_min"
	currentTurnInputLimitSourceDefault = "uncatalogued_default"
)

func estimateRequestTokensWithInputs(request provider.Request, inputs []provider.Input) int {
	shaped := request
	shaped.Inputs = append([]provider.Input(nil), inputs...)
	shaped = provider.PreparePromptRequest(shaped)
	return provider.EstimateRequestTokens(shaped)
}

type requestInputLimitExceeded struct {
	Payload events.ContextCompactionFailedPayload
}

func (e *requestInputLimitExceeded) Error() string {
	return e.Payload.Detail
}

type requestInputTriggerExceeded struct {
	Payload events.ContextCompactionFailedPayload
}

func (e *requestInputTriggerExceeded) Error() string {
	return e.Payload.Detail
}

type requestInputBudgetDecision struct {
	LimitFailure *requestInputLimitExceeded
	Pressure     *requestInputTriggerExceeded
	TokenSource  provider.TokenCountSource
}

func requestInputTriggerExceededPressure(request provider.Request, models modelCatalog, config SessionConfig) *requestInputTriggerExceeded {
	budget, ok := resolveModelInputBudgetForRequest(request, models)
	if !ok || budget.InputLimitTokens <= 0 {
		return nil
	}
	triggerTokens, targetTokens := compactionThresholdTokens(budget.InputLimitTokens, config)
	if triggerTokens <= 0 {
		return nil
	}
	requestTokens := estimateRequestTokensWithInputs(request, request.Inputs)
	if requestTokens <= triggerTokens {
		return nil
	}
	if requestTokens > budget.InputLimitTokens {
		return nil
	}
	if targetTokens <= 0 {
		targetTokens = triggerTokens
	}
	return &requestInputTriggerExceeded{
		Payload: events.ContextCompactionFailedPayload{
			Scope:                  events.CompactionScopeHistory,
			Reason:                 "input_limit_pressure",
			Detail:                 fmt.Sprintf("request reached compaction trigger after history compaction (%d > %d)", requestTokens, triggerTokens),
			InputLimitTokens:       budget.InputLimitTokens,
			TriggerTokens:          triggerTokens,
			TargetTokens:           targetTokens,
			EstimatedRequestTokens: requestTokens,
		},
	}
}

func requestInputBudgetDecisionForProviderRequest(ctx context.Context, client provider.Client, request provider.Request, models modelCatalog, config SessionConfig) requestInputBudgetDecision {
	budget, ok := resolveModelInputBudgetForRequest(request, models)
	if !ok || budget.InputLimitTokens <= 0 {
		return requestInputBudgetDecision{}
	}
	triggerTokens, targetTokens := compactionThresholdTokens(budget.InputLimitTokens, config)
	if triggerTokens <= 0 {
		triggerTokens = budget.InputLimitTokens
	}
	if targetTokens <= 0 {
		targetTokens = triggerTokens
	}

	requestTokens := estimateRequestTokensWithInputs(request, request.Inputs)
	source := provider.TokenCountSourceEstimated
	if shouldCountProviderRequestTokens(requestTokens, triggerTokens, targetTokens, budget.InputLimitTokens) {
		if counted, countedSource, err := provider.CountRequestTokens(ctx, client, request); err == nil || counted > 0 {
			source = countedSource
			if source == "" {
				source = provider.TokenCountSourceExact
			}
			if source == provider.TokenCountSourceExact {
				requestTokens = max(counted, 0)
			} else {
				requestTokens = max(requestTokens, counted)
			}
		}
	}
	measuredRequestTokens := requestTokens
	if source != provider.TokenCountSourceExact {
		requestTokens = applyEstimatedInputBudgetSafetyMargin(requestTokens)
	}

	decision := requestInputBudgetDecision{TokenSource: source}
	if requestTokens > budget.InputLimitTokens && (source == provider.TokenCountSourceExact || measuredRequestTokens > budget.InputLimitTokens) {
		decision.LimitFailure = &requestInputLimitExceeded{
			Payload: inputBudgetFailurePayload(
				"input_limit_unreachable",
				fmt.Sprintf("request exceeds input_limit_tokens after history compaction (%d > %d)", requestTokens, budget.InputLimitTokens),
				budget.InputLimitTokens,
				triggerTokens,
				targetTokens,
				requestTokens,
			),
		}
		return decision
	}
	if requestTokens > triggerTokens {
		decision.Pressure = &requestInputTriggerExceeded{
			Payload: inputBudgetFailurePayload(
				"input_limit_pressure",
				fmt.Sprintf("request reached compaction trigger after history compaction (%d > %d)", requestTokens, triggerTokens),
				budget.InputLimitTokens,
				triggerTokens,
				targetTokens,
				requestTokens,
			),
		}
	}
	return decision
}

func providerInputLimitExceededFailure(request provider.Request, models modelCatalog, config SessionConfig) *requestInputLimitExceeded {
	budget, ok := resolveModelInputBudgetForRequest(request, models)
	if !ok || budget.InputLimitTokens <= 0 {
		return nil
	}
	triggerTokens, targetTokens := compactionThresholdTokens(budget.InputLimitTokens, config)
	if triggerTokens <= 0 {
		triggerTokens = budget.InputLimitTokens
	}
	if targetTokens <= 0 {
		targetTokens = triggerTokens
	}
	requestTokens := estimateRequestTokensWithInputs(request, request.Inputs)
	return &requestInputLimitExceeded{
		Payload: inputBudgetFailurePayload(
			"input_limit_unreachable",
			fmt.Sprintf("provider rejected request as over input_limit_tokens after history compaction (estimated %d, limit %d)", requestTokens, budget.InputLimitTokens),
			budget.InputLimitTokens,
			triggerTokens,
			targetTokens,
			requestTokens,
		),
	}
}

func shouldCountProviderRequestTokens(estimatedTokens, triggerTokens, targetTokens, inputLimitTokens int) bool {
	if estimatedTokens <= 0 {
		return false
	}
	threshold := targetTokens
	if threshold <= 0 {
		threshold = triggerTokens
	}
	if threshold <= 0 {
		threshold = inputLimitTokens
	}
	return threshold <= 0 || estimatedTokens >= threshold
}

func applyEstimatedInputBudgetSafetyMargin(tokens int) int {
	if tokens <= 0 {
		return 0
	}
	return (tokens*115 + 99) / 100
}

func inputBudgetFailurePayload(reason, detail string, inputLimitTokens, triggerTokens, targetTokens, requestTokens int) events.ContextCompactionFailedPayload {
	return events.ContextCompactionFailedPayload{
		Scope:                  events.CompactionScopeHistory,
		Reason:                 reason,
		Detail:                 detail,
		InputLimitTokens:       inputLimitTokens,
		TriggerTokens:          triggerTokens,
		TargetTokens:           targetTokens,
		EstimatedRequestTokens: requestTokens,
	}
}

func catalogModelForRef(models modelCatalog, ref provider.ModelRef) (provider.CatalogModel, bool) {
	if models == nil {
		return provider.CatalogModel{}, false
	}
	providerID := strings.TrimSpace(ref.ProviderID)
	modelID := strings.TrimSpace(ref.ModelID)
	if providerID == "" || modelID == "" {
		return provider.CatalogModel{}, false
	}
	for _, model := range models.ModelsForProvider(providerID) {
		if strings.EqualFold(strings.TrimSpace(model.ID), modelID) {
			return provider.NormalizeCatalogModelCapabilities(providerID, model), true
		}
	}
	return provider.CatalogModel{}, false
}
