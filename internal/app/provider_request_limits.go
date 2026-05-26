package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
)

type outputBudgetKind string

const (
	outputBudgetAgentTurn         outputBudgetKind = "agent_turn"
	outputBudgetReview            outputBudgetKind = "review"
	outputBudgetSessionTitle      outputBudgetKind = "session_title"
	outputBudgetUtilityText       outputBudgetKind = "utility_text"
	outputBudgetWorkspaceCompress outputBudgetKind = "workspace_compress"
	outputBudgetSessionCompaction outputBudgetKind = "session_compaction"
)

func outputBudgetKindForAgent(agentID string) outputBudgetKind {
	if strings.TrimSpace(agentID) == reviewerAgentID {
		return outputBudgetReview
	}
	return outputBudgetAgentTurn
}

func requestMaxOutputTokensForRoute(models modelCatalog, overrides []ModelOverrideConfig, budgets OutputBudgetsConfig, route provider.ModelRoute, kind outputBudgetKind, thinking bool) int {
	return requestMaxOutputTokensForModel(models, overrides, budgets, route.Primary, kind, thinking)
}

func requestMaxOutputTokensForModel(models modelCatalog, overrides []ModelOverrideConfig, budgets OutputBudgetsConfig, ref provider.ModelRef, kind outputBudgetKind, thinking bool) int {
	ceiling := modelMaxOutputTokenCeilingForModel(models, ref)
	budget := outputBudgetForRequest(overrides, budgets, ref, kind, thinking)
	return clampOutputTokenBudget(budget, ceiling)
}

func modelMaxOutputTokenCeilingForModel(models modelCatalog, ref provider.ModelRef) int {
	model, ok := catalogModelForRef(models, ref)
	if !ok {
		return max(provider.SuggestedMaxOutputTokens(ref), 0)
	}
	return max(model.MaxOutputTokens, 0)
}

func outputBudgetForRequest(overrides []ModelOverrideConfig, budgets OutputBudgetsConfig, ref provider.ModelRef, kind outputBudgetKind, thinking bool) int {
	effective := budgets.Effective()
	switch kind {
	case outputBudgetSessionTitle:
		return effective.SessionTitle
	case outputBudgetUtilityText:
		return effective.UtilityText
	case outputBudgetReview:
		return effective.Review
	case outputBudgetWorkspaceCompress:
		return effective.WorkspaceCompress
	case outputBudgetSessionCompaction:
		return effective.SessionCompaction
	case outputBudgetAgentTurn:
		if override, ok := modelDefaultOutputTokensOverride(overrides, ref); ok {
			return override
		}
		if thinking {
			return effective.AgentTurnThinking
		}
		return effective.AgentTurn
	default:
		if override, ok := modelDefaultOutputTokensOverride(overrides, ref); ok {
			return override
		}
		if thinking {
			return effective.AgentTurnThinking
		}
		return effective.AgentTurn
	}
}

func modelDefaultOutputTokensOverride(overrides []ModelOverrideConfig, ref provider.ModelRef) (int, bool) {
	providerID := strings.TrimSpace(ref.ProviderID)
	modelID := strings.TrimSpace(ref.ModelID)
	if providerID == "" || modelID == "" {
		return 0, false
	}
	for _, override := range overrides {
		if override.DefaultOutputTokens == nil {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(override.Ref.ProviderID), providerID) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(override.Ref.ModelID), modelID) {
			continue
		}
		return max(*override.DefaultOutputTokens, 0), true
	}
	return 0, false
}

func clampOutputTokenBudget(budget int, ceiling int) int {
	if budget <= 0 {
		return max(ceiling, 0)
	}
	if ceiling > 0 && budget > ceiling {
		return ceiling
	}
	return budget
}
