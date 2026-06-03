package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var ErrSessionListingUnsupported = errors.New("session listing is not supported by the configured event store")

type BudgetScope string

const (
	BudgetScopeSession  BudgetScope = "session"
	BudgetScopeWorkflow BudgetScope = "workflow"
	BudgetScopeTotal    BudgetScope = "cross-session"
)

type BudgetExceededError struct {
	Scope               BudgetScope
	Cost                float64
	Budget              float64
	MissingPricingTurns int
}

func (e BudgetExceededError) Error() string {
	label := "Session budget reached"
	switch e.Scope {
	case BudgetScopeWorkflow:
		label = "Workflow budget reached"
	case BudgetScopeTotal:
		label = "Cross-session budget reached"
	}
	return formatBudgetStatusMessage(label, e.Cost, e.Budget, e.MissingPricingTurns)
}

type BudgetStatus struct {
	WorkflowID                  string
	WorkflowCost                float64
	WorkflowBudget              float64
	WorkflowWarnThreshold       float64
	WorkflowWarn                bool
	WorkflowExceeded            bool
	WorkflowMissingPricingTurns int

	SessionCost                float64
	SessionBudget              float64
	SessionWarnThreshold       float64
	SessionWarn                bool
	SessionExceeded            bool
	SessionMissingPricingTurns int

	TotalCost                float64
	TotalBudget              float64
	TotalWarnThreshold       float64
	TotalWarn                bool
	TotalExceeded            bool
	TotalMissingPricingTurns int
}

type sessionLister interface {
	ListSessions(ctx context.Context) ([]events.SessionIndexEntry, error)
}

type budgetSessionSummary struct {
	Turns map[string]budgetTurnSummary
}

type budgetTurnSummary struct {
	RequestTokens       int
	CompletionTokens    int
	EstimatedInputCost  float64
	EstimatedOutputCost float64
}

func (s BudgetStatus) HasSessionBudget() bool {
	return s.SessionBudget > 0
}

func (s BudgetStatus) HasWorkflowBudget() bool {
	return s.WorkflowBudget > 0
}

func (s BudgetStatus) HasTotalBudget() bool {
	return s.TotalBudget > 0
}

func (s BudgetStatus) AnyWarn() bool {
	return s.WorkflowWarn || s.SessionWarn || s.TotalWarn
}

func (s BudgetStatus) AnyExceeded() bool {
	return s.WorkflowExceeded || s.SessionExceeded || s.TotalExceeded
}

func (s BudgetStatus) WorkflowPercent() (int, bool) {
	return budgetPercent(s.WorkflowCost, s.WorkflowBudget)
}

func (s BudgetStatus) SessionPercent() (int, bool) {
	return budgetPercent(s.SessionCost, s.SessionBudget)
}

func (s BudgetStatus) TotalPercent() (int, bool) {
	return budgetPercent(s.TotalCost, s.TotalBudget)
}

func (s BudgetStatus) WarningMessage() string {
	switch {
	case s.TotalWarn:
		return formatBudgetStatusMessage("Cross-session budget warning", s.TotalCost, s.TotalBudget, s.TotalMissingPricingTurns)
	case s.WorkflowWarn:
		return formatBudgetStatusMessage("Workflow budget warning", s.WorkflowCost, s.WorkflowBudget, s.WorkflowMissingPricingTurns)
	case s.SessionWarn:
		return formatBudgetStatusMessage("Session budget warning", s.SessionCost, s.SessionBudget, s.SessionMissingPricingTurns)
	default:
		return ""
	}
}

func (s BudgetStatus) ExceededError() error {
	switch {
	case s.TotalExceeded:
		return BudgetExceededError{
			Scope:               BudgetScopeTotal,
			Cost:                s.TotalCost,
			Budget:              s.TotalBudget,
			MissingPricingTurns: s.TotalMissingPricingTurns,
		}
	case s.WorkflowExceeded:
		return BudgetExceededError{
			Scope:               BudgetScopeWorkflow,
			Cost:                s.WorkflowCost,
			Budget:              s.WorkflowBudget,
			MissingPricingTurns: s.WorkflowMissingPricingTurns,
		}
	case s.SessionExceeded:
		return BudgetExceededError{
			Scope:               BudgetScopeSession,
			Cost:                s.SessionCost,
			Budget:              s.SessionBudget,
			MissingPricingTurns: s.SessionMissingPricingTurns,
		}
	default:
		return nil
	}
}

func (s *SessionService) BudgetStatus(ctx context.Context, sessionID string, config SessionConfig) (BudgetStatus, error) {
	status := BudgetStatus{
		SessionBudget:        max(config.Budget, 0),
		SessionWarnThreshold: config.BudgetWarn,
		TotalBudget:          max(config.TotalBudget, 0),
		TotalWarnThreshold:   config.TotalBudgetWarn,
	}
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return status, nil
	}

	usageSummary, err := s.UsageSummary(ctx, sessionID)
	if err != nil {
		return BudgetStatus{}, err
	}
	status.SessionCost = usageSummary.EstimatedCost
	status.SessionMissingPricingTurns = usageSummary.MissingPricingTurns
	if status.HasSessionBudget() {
		status.SessionWarn = status.SessionWarnThreshold > 0 && status.SessionCost >= status.SessionBudget*status.SessionWarnThreshold
		status.SessionExceeded = status.SessionCost >= status.SessionBudget
	}

	if !status.HasTotalBudget() {
		return status, nil
	}
	status.TotalCost, status.TotalMissingPricingTurns, err = s.totalEstimatedCost(ctx)
	if err != nil {
		return BudgetStatus{}, err
	}
	status.TotalWarn = status.TotalWarnThreshold > 0 && status.TotalCost >= status.TotalBudget*status.TotalWarnThreshold
	status.TotalExceeded = status.TotalCost >= status.TotalBudget
	return status, nil
}

func (s *SessionService) totalEstimatedCost(ctx context.Context) (float64, int, error) {
	if s == nil {
		return 0, 0, nil
	}
	s.budgetMu.Lock()
	if s.budgetTotalsWarm {
		totalCost := s.budgetTotalsCost
		totalMissing := s.budgetTotalsMiss
		s.budgetMu.Unlock()
		return totalCost, totalMissing, nil
	}
	s.budgetMu.Unlock()

	lister, ok := s.store.(sessionLister)
	if !ok {
		return 0, 0, ErrSessionListingUnsupported
	}
	entries, err := lister.ListSessions(ctx)
	if err != nil {
		return 0, 0, err
	}
	seen := make(map[string]struct{}, len(entries))
	totalCost := 0.0
	totalMissing := 0
	for _, entry := range entries {
		sessionID := strings.TrimSpace(entry.SessionID)
		if sessionID == "" {
			continue
		}
		if _, ok := seen[sessionID]; ok {
			continue
		}
		seen[sessionID] = struct{}{}
		summary, err := s.budgetSummary(ctx, sessionID)
		if err != nil {
			return 0, 0, fmt.Errorf("budget summary for %s: %w", sessionID, err)
		}
		cost, missing := summary.totals()
		totalCost += cost
		totalMissing += missing
	}

	s.budgetMu.Lock()
	s.budgetTotalsCost = totalCost
	s.budgetTotalsMiss = totalMissing
	s.budgetTotalsWarm = true
	s.budgetMu.Unlock()
	return totalCost, totalMissing, nil
}

func (r *Runtime) BudgetStatus(ctx context.Context, sessionID string) (BudgetStatus, error) {
	if r == nil || r.Sessions == nil {
		return BudgetStatus{}, nil
	}
	status, err := r.Sessions.BudgetStatus(ctx, sessionID, r.Config.Sessions)
	if err != nil {
		return BudgetStatus{}, err
	}
	state, definition, workflow, err := r.activeWorkflowState(ctx, sessionID)
	if err != nil {
		if errors.Is(err, ErrWorkflowStateMissing) || errors.Is(err, ErrWorkflowSessionNotConfigured) {
			return status, nil
		}
		return BudgetStatus{}, err
	}
	workflowBudget := workflowTurnBudgetFromDefinition(workflow.WorkflowID, definition)
	if !workflowBudget.enabled() {
		return status, nil
	}
	summary, err := workflowBudgetSummaryFromState(ctx, r.Sessions, state, workflowBudget.WorkflowID)
	if err != nil {
		return BudgetStatus{}, err
	}
	status.WorkflowID = workflowBudget.WorkflowID
	status.WorkflowCost = summary.Cost
	status.WorkflowBudget = workflowBudget.MaxCost
	status.WorkflowWarnThreshold = definition.Budgets.WarnThreshold
	status.WorkflowMissingPricingTurns = summary.MissingPricingTurns
	status.WorkflowWarn = status.WorkflowWarnThreshold > 0 && status.WorkflowCost >= status.WorkflowBudget*status.WorkflowWarnThreshold
	status.WorkflowExceeded = status.WorkflowCost >= status.WorkflowBudget
	return status, nil
}

func (s *SessionService) budgetSummary(ctx context.Context, sessionID string) (budgetSessionSummary, error) {
	if s == nil || strings.TrimSpace(sessionID) == "" {
		return budgetSessionSummary{}, nil
	}

	runtime := s.runtimeForSession(sessionID)
	runtime.mu.Lock()
	if runtime.budgetWarm {
		summary := runtime.budget
		runtime.mu.Unlock()
		return summary, nil
	}
	if runtime.projector != nil {
		s.ensureBudgetSummaryLocked(runtime)
		summary := runtime.budget
		runtime.mu.Unlock()
		return summary, nil
	}
	runtime.mu.Unlock()

	summary, err := s.replayBudgetSummary(ctx, sessionID)
	if err != nil {
		return budgetSessionSummary{}, err
	}

	runtime.mu.Lock()
	if runtime.budgetWarm {
		summary = runtime.budget
	} else if runtime.projector != nil {
		summary = budgetSessionSummaryFromState(runtime.projector.CurrentState())
		runtime.budget = summary
		runtime.budgetWarm = true
	} else {
		runtime.budget = summary
		runtime.budgetWarm = true
	}
	runtime.mu.Unlock()
	return summary, nil
}

func (s *SessionService) replayBudgetSummary(ctx context.Context, sessionID string) (budgetSessionSummary, error) {
	projector, _, _, err := s.loadProjector(ctx, sessionID)
	if err != nil {
		return budgetSessionSummary{}, err
	}
	return budgetSessionSummaryFromState(projector.CurrentState()), nil
}

func (s *SessionService) updateBudgetSummaryLocked(runtime *sessionRuntime) {
	if runtime == nil {
		return
	}
	before := runtime.budget
	beforeCost, beforeMissing := before.totals()
	after := budgetSessionSummaryFromState(runtime.projector.CurrentState())
	runtime.budget = after
	runtime.budgetWarm = true

	s.budgetMu.Lock()
	defer s.budgetMu.Unlock()
	if !s.budgetTotalsWarm {
		return
	}
	afterCost, afterMissing := after.totals()
	s.budgetTotalsCost += afterCost - beforeCost
	s.budgetTotalsMiss += afterMissing - beforeMissing
	if s.budgetTotalsMiss < 0 {
		s.budgetTotalsMiss = 0
	}
}

func (s *SessionService) clearBudgetSummaryLocked(runtime *sessionRuntime) {
	if runtime == nil || !runtime.budgetWarm {
		return
	}
	summary := runtime.budget

	s.budgetMu.Lock()
	defer s.budgetMu.Unlock()
	if !s.budgetTotalsWarm {
		return
	}
	cost, missing := summary.totals()
	s.budgetTotalsCost -= cost
	s.budgetTotalsMiss -= missing
	if s.budgetTotalsMiss < 0 {
		s.budgetTotalsMiss = 0
	}
	if s.budgetTotalsCost < 0 {
		s.budgetTotalsCost = 0
	}
}

func budgetSessionSummaryFromState(state events.SessionState) budgetSessionSummary {
	summary := budgetSessionSummary{Turns: make(map[string]budgetTurnSummary, len(state.Turns))}
	for turnID, turn := range state.Turns {
		if turn == nil || turn.ProviderUsage == nil {
			continue
		}
		turnSummary := budgetTurnSummary{
			RequestTokens:       max(turn.ProviderUsage.RequestTokens, 0),
			CompletionTokens:    max(turn.ProviderUsage.CompletionTokens, 0),
			EstimatedInputCost:  max(turn.ProviderUsage.EstimatedInputCost, 0),
			EstimatedOutputCost: max(turn.ProviderUsage.EstimatedOutputCost, 0),
		}
		summary.Turns[turnID] = turnSummary
	}
	return summary
}

func (s budgetSessionSummary) totals() (float64, int) {
	totalCost := 0.0
	missingPricingTurns := 0
	for _, turn := range s.Turns {
		cost := turn.EstimatedInputCost + turn.EstimatedOutputCost
		totalCost += cost
		if cost <= 0 && turn.RequestTokens+turn.CompletionTokens > 0 {
			missingPricingTurns++
		}
	}
	return totalCost, missingPricingTurns
}

func budgetPercent(cost, budget float64) (int, bool) {
	if budget <= 0 {
		return 0, false
	}
	return int(math.Round((cost / budget) * 100)), true
}

func formatBudgetStatusMessage(label string, cost, budget float64, missingPricingTurns int) string {
	message := fmt.Sprintf("%s: %s of %s used", label, formatBudgetCurrency(cost), formatBudgetCurrency(budget))
	if missingPricingTurns > 0 {
		message += fmt.Sprintf(" (pricing missing for %s)", budgetPluralize(missingPricingTurns, "turn"))
	}
	return message
}

func formatBudgetCurrency(cost float64) string {
	switch {
	case cost >= 10:
		return fmt.Sprintf("$%.2f", cost)
	case cost >= 1:
		return fmt.Sprintf("$%.3f", cost)
	case cost >= 0.1:
		return fmt.Sprintf("$%.4f", cost)
	default:
		return fmt.Sprintf("$%.5f", cost)
	}
}

func budgetPluralize(count int, singular string) string {
	if count == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %ss", count, singular)
}
