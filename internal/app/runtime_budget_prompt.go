package app

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
)

func (r *Runtime) runtimeBudgetPromptFragment(ctx context.Context, sessionID string, workflowBudget workflowTurnBudget) (prompt.Fragment, bool) {
	status, ok := r.runtimeBudgetPromptStatus(ctx, sessionID, workflowBudget)
	if !ok {
		return prompt.Fragment{}, false
	}
	content, ok := runtimeBudgetPromptContent(status, workflowBudget)
	if !ok {
		return prompt.Fragment{}, false
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Layer:     "budget-context",
		Key:       "budget-context",
		Label:     "budget-context",
		Content:   content,
	}, true
}

func (r *Runtime) runtimeBudgetPromptStatus(ctx context.Context, sessionID string, workflowBudget workflowTurnBudget) (BudgetStatus, bool) {
	if r == nil || r.Sessions == nil {
		return BudgetStatus{}, false
	}
	status, err := r.Sessions.BudgetStatus(ctx, sessionID, r.Config.Sessions)
	if err != nil {
		return BudgetStatus{}, false
	}
	if !workflowBudget.enabled() {
		return status, true
	}
	budgetSessionID := strings.TrimSpace(workflowBudget.SessionID)
	if budgetSessionID == "" {
		budgetSessionID = sessionID
	}
	state, err := r.Sessions.Snapshot(ctx, budgetSessionID)
	if err != nil {
		return status, true
	}
	summary, err := workflowBudgetSummaryFromState(ctx, r.Sessions, state, workflowBudget.WorkflowID)
	if err != nil {
		return status, true
	}
	status.WorkflowID = strings.TrimSpace(workflowBudget.WorkflowID)
	status.WorkflowCost = summary.Cost
	status.WorkflowBudget = workflowBudget.MaxCost
	status.WorkflowWarnThreshold = workflowBudget.WarnThreshold
	status.WorkflowMissingPricingTurns = summary.MissingPricingTurns
	status.WorkflowWarn = status.WorkflowWarnThreshold > 0 && status.WorkflowCost >= status.WorkflowBudget*status.WorkflowWarnThreshold
	status.WorkflowExceeded = status.WorkflowCost >= status.WorkflowBudget
	return status, true
}

func runtimeBudgetPromptContent(status BudgetStatus, workflowBudget workflowTurnBudget) (string, bool) {
	lines := []string{}
	if status.HasSessionBudget() {
		lines = append(lines, runtimeBudgetPromptScopeLine("Session budget", status.SessionCost, status.SessionBudget, status.SessionWarnThreshold, status.SessionMissingPricingTurns))
	}
	if status.HasWorkflowBudget() {
		label := "Workflow budget"
		if workflowID := strings.TrimSpace(status.WorkflowID); workflowID != "" {
			label += " (" + workflowID + ")"
		}
		lines = append(lines, runtimeBudgetPromptScopeLine(label, status.WorkflowCost, status.WorkflowBudget, status.WorkflowWarnThreshold, status.WorkflowMissingPricingTurns))
	}
	if status.HasTotalBudget() {
		lines = append(lines, runtimeBudgetPromptScopeLine("Cross-session budget", status.TotalCost, status.TotalBudget, status.TotalWarnThreshold, status.TotalMissingPricingTurns))
	}
	if workflowBudget.MaxProviderRequestsPerTurn > 0 {
		lines = append(lines, fmt.Sprintf("Workflow provider request cap for this turn: %d", workflowBudget.MaxProviderRequestsPerTurn))
	}
	if len(lines) == 0 {
		return "", false
	}
	lines = append([]string{
		"Budget context:",
		"- Runtime enforces hard budget limits before provider calls; treat this as planning context, not as a reason to claim incomplete work is done.",
	}, prefixBudgetPromptLines(lines)...)
	lines = append(lines, "- Under budget pressure, prefer focused evidence, bounded scope, and explicit user checkpoints over speculative exploration.")
	return strings.Join(lines, "\n"), true
}

func runtimeBudgetPromptScopeLine(label string, cost, budget, warnThreshold float64, missingPricingTurns int) string {
	line := fmt.Sprintf("%s: %s of %s used", strings.TrimSpace(label), formatBudgetCurrency(cost), formatBudgetCurrency(budget))
	if percent, ok := budgetPercent(cost, budget); ok {
		line += fmt.Sprintf(" (%d%%)", percent)
	}
	if warnThreshold > 0 {
		line += fmt.Sprintf("; warn at %d%%", int(math.Round(warnThreshold*100)))
	}
	if missingPricingTurns > 0 {
		line += fmt.Sprintf("; pricing missing for %s", budgetPluralize(missingPricingTurns, "turn"))
	}
	return line
}

func prefixBudgetPromptLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, "- "+line)
		}
	}
	return out
}
