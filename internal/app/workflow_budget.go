package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

type workflowTurnBudget struct {
	WorkflowID                 string
	SessionID                  string
	MaxCost                    float64
	WarnThreshold              float64
	MaxProviderRequestsPerTurn int
}

func workflowTurnBudgetFromDefinition(workflowID string, definition workflowpkg.Definition) workflowTurnBudget {
	return workflowTurnBudget{
		WorkflowID:                 strings.TrimSpace(workflowID),
		MaxCost:                    max(definition.Budgets.MaxCost, 0),
		WarnThreshold:              max(definition.Budgets.WarnThreshold, 0),
		MaxProviderRequestsPerTurn: max(definition.Budgets.MaxProviderRequestsPerTurn, 0),
	}
}

func (b workflowTurnBudget) enabled() bool {
	return strings.TrimSpace(b.WorkflowID) != "" && b.MaxCost > 0
}

type workflowBudgetSummary struct {
	Cost                float64
	MissingPricingTurns int
}

func workflowBudgetSummaryFromState(ctx context.Context, sessions *SessionService, state events.SessionState, workflowID string) (workflowBudgetSummary, error) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return workflowBudgetSummary{}, nil
	}
	startedAtSeq := int64(-1)
	if state.Workflow != nil && strings.TrimSpace(state.Workflow.WorkflowID) == workflowID {
		startedAtSeq = state.Workflow.StartedAtSeq
	}
	summary := workflowBudgetSummary{}
	visitedChildSessions := map[string]struct{}{}
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil || turn.Config == nil || strings.TrimSpace(turn.Config.WorkflowID) != workflowID {
			continue
		}
		if startedAtSeq >= 0 && turn.LastUpdatedAtSeq > 0 && turn.LastUpdatedAtSeq < startedAtSeq {
			continue
		}
		if turn.ProviderUsage != nil {
			cost := max(turn.ProviderUsage.EstimatedInputCost, 0) + max(turn.ProviderUsage.EstimatedOutputCost, 0)
			summary.Cost += cost
			if cost <= 0 && max(turn.ProviderUsage.RequestTokens, 0)+max(turn.ProviderUsage.CompletionTokens, 0) > 0 {
				summary.MissingPricingTurns++
			}
		}
		childSummary, err := workflowChildHandoffBudgetSummary(ctx, sessions, turn, visitedChildSessions)
		if err != nil {
			return workflowBudgetSummary{}, err
		}
		summary.Cost += childSummary.Cost
		summary.MissingPricingTurns += childSummary.MissingPricingTurns
	}
	return summary, nil
}

func workflowChildHandoffBudgetSummary(ctx context.Context, sessions *SessionService, turn *events.TurnState, visited map[string]struct{}) (workflowBudgetSummary, error) {
	if sessions == nil || turn == nil {
		return workflowBudgetSummary{}, nil
	}
	summary := workflowBudgetSummary{}
	for _, handoffID := range turn.HandoffOrder {
		handoff := turn.Handoffs[handoffID]
		if handoff == nil {
			continue
		}
		childSessionID := strings.TrimSpace(handoff.ChildSessionID)
		if childSessionID == "" {
			continue
		}
		if _, ok := visited[childSessionID]; ok {
			continue
		}
		visited[childSessionID] = struct{}{}
		usage, err := sessions.UsageSummary(ctx, childSessionID)
		if err != nil {
			return workflowBudgetSummary{}, err
		}
		summary.Cost += usage.EstimatedCost
		summary.MissingPricingTurns += usage.MissingPricingTurns
	}
	return summary, nil
}

func (r *TurnRunner) enforceWorkflowBudgetLimit(ctx context.Context, sessionID string, budget workflowTurnBudget) error {
	if r == nil || r.sessions == nil || !budget.enabled() {
		return nil
	}
	budgetSessionID := strings.TrimSpace(budget.SessionID)
	if budgetSessionID == "" {
		budgetSessionID = sessionID
	}
	state, err := r.sessions.Snapshot(ctx, budgetSessionID)
	if err != nil {
		return err
	}
	summary, err := workflowBudgetSummaryFromState(ctx, r.sessions, state, budget.WorkflowID)
	if err != nil {
		return err
	}
	if summary.Cost < budget.MaxCost {
		return nil
	}
	return BudgetExceededError{
		Scope:               BudgetScopeWorkflow,
		Cost:                summary.Cost,
		Budget:              budget.MaxCost,
		MissingPricingTurns: summary.MissingPricingTurns,
	}
}
