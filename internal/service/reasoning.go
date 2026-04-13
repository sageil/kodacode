package service

import (
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
)

// resolveChatReasoning applies kodacode's request-level precedence for reasoning:
// per-model config provides the base budget, agent config can override it, and
// the user-selected variant overrides both. Provider-level defaults remain in
// the provider implementation when no request-level budget is resolved.
func resolveChatReasoning(req *pipeline.TurnRequest) provider.ChatOptions {
	budget := req.Model.ThinkingBudget
	if req.Agent.Reasoning.Budget != nil {
		budget = req.Agent.Reasoning.Budget
	}
	effort := req.Agent.Reasoning.Effort

	if variantBudget, ok := VariantBudget(req.Variant); ok {
		budget = &variantBudget
	}
	if variantEffort, ok := VariantEffort(req.Variant); ok {
		effort = variantEffort
	}

	budget = AutoReduceReasoningBudget(req.Step, budget)
	budget = ContextAwareReasoningBudget(budget, req.ContextUsage)

	return provider.ChatOptions{
		SupportedEndpoints: append([]string(nil), req.Model.SupportedEndpoints...),
		ReasoningBudget:    budget,
		ReasoningEffort:    effort,
		ReasoningSupported: req.Model.Reasoning || req.Model.ThinkingBudget != nil,
	}
}
