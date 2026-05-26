package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type sessionCompactionPlan struct {
	UpdateReason               string
	FrontierTurnID             string
	ConsolidatedTurnCount      int
	NewlyConsolidatedTurnCount int
	NewTurnIDs                 []string
}

func buildNextCompaction(
	existing *events.SessionHistoryContinuationUpdatedPayload,
	completedOrder []string,
	turns map[string]*replayedSessionTurn,
	request provider.Request,
	currentTurnInputs []provider.Input,
	budget sessionHistoryBudget,
) *events.SessionHistoryContinuationUpdatedPayload {
	plan := buildSessionCompactionPlan(existing, completedOrder, request, currentTurnInputs, budget, turns)
	return buildSessionCompactionProjectionPayload(existing, plan, turns, budget.SummaryBudgetBytes)
}

func buildSessionCompactionPlan(
	existing *events.SessionHistoryContinuationUpdatedPayload,
	completedOrder []string,
	request provider.Request,
	currentTurnInputs []provider.Input,
	budget sessionHistoryBudget,
	turns map[string]*replayedSessionTurn,
) *sessionCompactionPlan {
	if !budget.resolved() {
		return nil
	}
	if !sessionHistoryRequestUsable(request, currentTurnInputs) {
		return nil
	}
	if sessionCompactionPrefixCount(completedOrder, existing) >= len(completedOrder) {
		return nil
	}
	rawInputs, _ := buildSessionConversationInputs(sessionHistoryRawOrder(completedOrder, existing), turns, existing, budget.SummaryBudgetBytes)
	estimatedRawTokens := estimateSessionRequestTokens(request, rawInputs, currentTurnInputs)
	if estimatedRawTokens <= budget.TriggerTokens {
		return nil
	}

	semanticPlan := buildSemanticSessionCompactionPlan(existing, completedOrder)
	if semanticPlan != nil {
		semanticPayload := buildSessionCompactionProjectionPayload(existing, semanticPlan, turns, budget.SummaryBudgetBytes)
		if semanticPayload != nil {
			compactedInputs, _ := buildSessionConversationInputs(sessionHistoryRawOrder(completedOrder, semanticPayload), turns, semanticPayload, budget.SummaryBudgetBytes)
			estimatedSemanticTokens := estimateSessionRequestTokens(request, compactedInputs, currentTurnInputs)
			if estimatedSemanticTokens <= budget.TargetTokens {
				return semanticPlan
			}
		}
	}

	return buildTokenPressureSessionCompactionPlan(existing, completedOrder)
}

func buildSessionCompactionPayload(existing *events.SessionHistoryContinuationUpdatedPayload, compactedOrder []string, turns map[string]*replayedSessionTurn, summaryBudgetBytes int) *events.SessionHistoryContinuationUpdatedPayload {
	plan := buildSessionCompactionPlanForOrder(existing, compactedOrder)
	return buildSessionCompactionProjectionPayload(existing, plan, turns, summaryBudgetBytes)
}

func buildSessionCompactionPlanForOrder(existing *events.SessionHistoryContinuationUpdatedPayload, compactedOrder []string) *sessionCompactionPlan {
	compactedOrder = sanitizeCompactionTurnOrder(compactedOrder)
	if len(compactedOrder) == 0 {
		return nil
	}
	newTurnIDs := compactionNewTurnIDs(compactedOrder, existing)
	return &sessionCompactionPlan{
		UpdateReason:               sessionHistoryCompactionReason,
		FrontierTurnID:             compactedOrder[len(compactedOrder)-1],
		ConsolidatedTurnCount:      len(compactedOrder),
		NewlyConsolidatedTurnCount: len(newTurnIDs),
		NewTurnIDs:                 newTurnIDs,
	}
}

func buildSemanticSessionCompactionPlan(existing *events.SessionHistoryContinuationUpdatedPayload, completedOrder []string) *sessionCompactionPlan {
	completedOrder = sanitizeCompactionTurnOrder(completedOrder)
	if len(completedOrder) <= sessionHistorySemanticFrontier {
		return nil
	}
	targetPrefixCount := len(completedOrder) - sessionHistorySemanticFrontier
	if targetPrefixCount <= 0 {
		return nil
	}
	if sessionCompactionPrefixCount(completedOrder, existing) >= targetPrefixCount {
		return nil
	}
	plan := buildSessionCompactionPlanForOrder(existing, completedOrder[:targetPrefixCount])
	if plan == nil {
		return nil
	}
	plan.UpdateReason = events.HistoryContinuationUpdateReasonSemanticClosure
	return plan
}

func buildTokenPressureSessionCompactionPlan(existing *events.SessionHistoryContinuationUpdatedPayload, completedOrder []string) *sessionCompactionPlan {
	plan := buildSessionCompactionPlanForOrder(existing, completedOrder)
	if plan == nil {
		return nil
	}
	plan.UpdateReason = sessionHistoryCompactionReason
	return plan
}

func buildSessionCompactionProjectionPayload(
	existing *events.SessionHistoryContinuationUpdatedPayload,
	plan *sessionCompactionPlan,
	turns map[string]*replayedSessionTurn,
	summaryBudgetBytes int,
) *events.SessionHistoryContinuationUpdatedPayload {
	if plan == nil {
		return nil
	}
	artifact := buildSessionCompactionArtifact(existing, plan.NewTurnIDs, turns)
	payload := buildSessionCompactionPayloadFromArtifact(plan, artifact, summaryBudgetBytes)
	if payload == nil {
		return nil
	}
	return payload
}

func buildSessionCompactionPayloadFromArtifact(
	plan *sessionCompactionPlan,
	artifact events.HistoryContinuationArtifact,
	summaryBudgetBytes int,
) *events.SessionHistoryContinuationUpdatedPayload {
	if plan == nil {
		return nil
	}
	payload := plan.scopePayload()
	payload.Artifact = artifact
	payload.RenderedSummary = renderSessionCompactionArtifactSummary(artifact, summaryBudgetBytes)
	if strings.TrimSpace(payload.RenderedSummary) == "" {
		return nil
	}
	return applySessionCompactionActivityText(payload)
}

func (p *sessionCompactionPlan) scopePayload() *events.SessionHistoryContinuationUpdatedPayload {
	if p == nil {
		return nil
	}
	updateReason := strings.TrimSpace(p.UpdateReason)
	if updateReason == "" {
		updateReason = sessionHistoryCompactionReason
	}
	return &events.SessionHistoryContinuationUpdatedPayload{
		UpdateReason:               updateReason,
		FrontierTurnID:             strings.TrimSpace(p.FrontierTurnID),
		ConsolidatedTurnCount:      p.ConsolidatedTurnCount,
		NewlyConsolidatedTurnCount: p.NewlyConsolidatedTurnCount,
	}
}

func sessionCompactionPlanFromPayload(
	existing *events.SessionHistoryContinuationUpdatedPayload,
	payload *events.SessionHistoryContinuationUpdatedPayload,
	completedOrder []string,
) *sessionCompactionPlan {
	if payload == nil {
		return nil
	}
	compactedOrder := compactedPrefixTurnOrder(completedOrder, payload)
	if len(compactedOrder) == 0 {
		return nil
	}
	return &sessionCompactionPlan{
		UpdateReason:               strings.TrimSpace(payload.UpdateReason),
		FrontierTurnID:             strings.TrimSpace(payload.FrontierTurnID),
		ConsolidatedTurnCount:      payload.ConsolidatedTurnCount,
		NewlyConsolidatedTurnCount: payload.NewlyConsolidatedTurnCount,
		NewTurnIDs:                 compactionNewTurnIDs(compactedOrder, existing),
	}
}

func cloneSessionCompactionPlan(plan *sessionCompactionPlan) *sessionCompactionPlan {
	if plan == nil {
		return nil
	}
	copyPlan := *plan
	copyPlan.NewTurnIDs = append([]string(nil), plan.NewTurnIDs...)
	return &copyPlan
}

func sessionCompactionPrefixCount(order []string, payload *events.SessionHistoryContinuationUpdatedPayload) int {
	if payload == nil || len(order) == 0 {
		return 0
	}
	frontierTurnID := strings.TrimSpace(payload.FrontierTurnID)
	if payload.ConsolidatedTurnCount > 0 && payload.ConsolidatedTurnCount <= len(order) {
		if frontierTurnID == "" || order[payload.ConsolidatedTurnCount-1] == frontierTurnID {
			return payload.ConsolidatedTurnCount
		}
	}
	if frontierTurnID == "" {
		return 0
	}
	for index, turnID := range order {
		if turnID == frontierTurnID {
			return index + 1
		}
	}
	return 0
}

func compactedPrefixTurnOrder(order []string, payload *events.SessionHistoryContinuationUpdatedPayload) []string {
	prefixCount := sessionCompactionPrefixCount(order, payload)
	if prefixCount <= 0 {
		return nil
	}
	return append([]string(nil), order[:prefixCount]...)
}

func sessionHistoryRawOrder(order []string, payload *events.SessionHistoryContinuationUpdatedPayload) []string {
	prefixCount := sessionCompactionPrefixCount(order, payload)
	if prefixCount >= len(order) {
		return nil
	}
	return append([]string(nil), order[prefixCount:]...)
}

func sanitizeCompactionTurnOrder(turnIDs []string) []string {
	if len(turnIDs) == 0 {
		return nil
	}
	out := make([]string, 0, len(turnIDs))
	seen := make(map[string]struct{}, len(turnIDs))
	for _, turnID := range turnIDs {
		turnID = strings.TrimSpace(turnID)
		if turnID == "" {
			continue
		}
		if _, ok := seen[turnID]; ok {
			continue
		}
		seen[turnID] = struct{}{}
		out = append(out, turnID)
	}
	return out
}

func compactionNewTurnIDs(compactedOrder []string, existing *events.SessionHistoryContinuationUpdatedPayload) []string {
	start := 0
	if existing != nil {
		start = min(sessionCompactionPrefixCount(compactedOrder, existing), len(compactedOrder))
	}
	return append([]string(nil), compactedOrder[start:]...)
}

func normalizeSessionCompactionPayload(
	payload *events.SessionHistoryContinuationUpdatedPayload,
	budget sessionHistoryBudget,
	completedOrder []string,
) *events.SessionHistoryContinuationUpdatedPayload {
	if payload == nil {
		return nil
	}
	copyPayload := cloneCompactionPayload(payload)
	if strings.TrimSpace(copyPayload.UpdateReason) == "" {
		copyPayload.UpdateReason = sessionHistoryCompactionReason
	}
	if source := strings.TrimSpace(copyPayload.Attribution.InputLimitSource); source != "" {
		copyPayload.Attribution.InputLimitSource = source
	} else if source := strings.TrimSpace(budget.InputLimitSource); source != "" {
		copyPayload.Attribution.InputLimitSource = source
	}
	copyPayload.Attribution.MeasurementSource = strings.TrimSpace(copyPayload.Attribution.MeasurementSource)
	copyPayload.Attribution.SummarySource = strings.TrimSpace(copyPayload.Attribution.SummarySource)
	if copyPayload.InputBudget == nil {
		copyPayload.InputBudget = &events.HistoryInputBudgetPayload{}
	}
	if copyPayload.InputBudget.InputLimitTokens <= 0 && budget.InputLimitTokens > 0 {
		copyPayload.InputBudget.InputLimitTokens = budget.InputLimitTokens
	}
	if copyPayload.InputBudget.TriggerTokens <= 0 && budget.TriggerTokens > 0 {
		copyPayload.InputBudget.TriggerTokens = budget.TriggerTokens
	}
	if copyPayload.InputBudget.TargetTokens <= 0 && budget.TargetTokens > 0 {
		copyPayload.InputBudget.TargetTokens = budget.TargetTokens
	}
	prefixCount := sessionCompactionPrefixCount(completedOrder, copyPayload)
	if prefixCount > 0 {
		copyPayload.ConsolidatedTurnCount = prefixCount
		copyPayload.FrontierTurnID = completedOrder[prefixCount-1]
	}
	if copyPayload.NewlyConsolidatedTurnCount < 0 {
		copyPayload.NewlyConsolidatedTurnCount = 0
	}
	if copyPayload.NewlyConsolidatedTurnCount > copyPayload.ConsolidatedTurnCount {
		copyPayload.NewlyConsolidatedTurnCount = copyPayload.ConsolidatedTurnCount
	}
	copyPayload.Artifact = normalizeHistoryCompactionArtifact(copyPayload.Artifact)
	if budget.SummaryBudgetBytes > 0 {
		copyPayload.RenderedSummary = renderSessionCompactionArtifactSummary(copyPayload.Artifact, budget.SummaryBudgetBytes)
	}
	return applySessionCompactionActivityText(copyPayload)
}
