package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type SessionUsageEntry struct {
	SessionID                string
	Depth                    int
	AgentID                  string
	Title                    string
	UsageTurns               int
	ToolCalls                int
	CompletedToolCalls       int
	FailedToolCalls          int
	ContractViolationCalls   int
	RequestTokens            int
	CompletionTokens         int
	UnpricedRequestTokens    int
	UnpricedCompletionTokens int
	CacheReadInputTokens     int
	CacheWriteInputTokens    int
	ReasoningTokens          int
	Steps                    int
	Attempts                 int
	EstimatedCost            float64
	MissingPricingTurns      int
	Exact                    bool
}

type SessionUsageSummary struct {
	RootSessionID            string
	SessionCount             int
	ChildSessionCount        int
	UsageTurns               int
	ToolCalls                int
	CompletedToolCalls       int
	FailedToolCalls          int
	ContractViolationCalls   int
	RequestTokens            int
	CompletionTokens         int
	UnpricedRequestTokens    int
	UnpricedCompletionTokens int
	CacheReadInputTokens     int
	CacheWriteInputTokens    int
	ReasoningTokens          int
	Steps                    int
	Attempts                 int
	EstimatedCost            float64
	MissingPricingTurns      int
	Exact                    bool
	Local                    SessionUsageEntry
	Sessions                 []SessionUsageEntry
}

func (s SessionUsageSummary) ValidFor(sessionID string) bool {
	return strings.TrimSpace(s.RootSessionID) != "" &&
		strings.TrimSpace(s.RootSessionID) == strings.TrimSpace(sessionID)
}

func (s SessionUsageSummary) HasUsage() bool {
	return s.UsageTurns > 0 || s.RequestTokens > 0 || s.CompletionTokens > 0
}

func (s SessionUsageSummary) HasDelegatedSessions() bool {
	return s.ChildSessionCount > 0
}

func (s SessionUsageSummary) DelegatedUsageTurns() int {
	return max(s.UsageTurns-s.Local.UsageTurns, 0)
}

func (s SessionUsageSummary) DelegatedToolCalls() int {
	return max(s.ToolCalls-s.Local.ToolCalls, 0)
}

func (s SessionUsageSummary) DelegatedCompletedToolCalls() int {
	return max(s.CompletedToolCalls-s.Local.CompletedToolCalls, 0)
}

func (s SessionUsageSummary) DelegatedFailedToolCalls() int {
	return max(s.FailedToolCalls-s.Local.FailedToolCalls, 0)
}

func (s SessionUsageSummary) DelegatedContractViolationCalls() int {
	return max(s.ContractViolationCalls-s.Local.ContractViolationCalls, 0)
}

func (s SessionUsageSummary) DelegatedRequestTokens() int {
	return max(s.RequestTokens-s.Local.RequestTokens, 0)
}

func (s SessionUsageSummary) DelegatedCompletionTokens() int {
	return max(s.CompletionTokens-s.Local.CompletionTokens, 0)
}

func (s SessionUsageSummary) DelegatedUnpricedRequestTokens() int {
	return max(s.UnpricedRequestTokens-s.Local.UnpricedRequestTokens, 0)
}

func (s SessionUsageSummary) DelegatedUnpricedCompletionTokens() int {
	return max(s.UnpricedCompletionTokens-s.Local.UnpricedCompletionTokens, 0)
}

func (s SessionUsageSummary) DelegatedCacheReadInputTokens() int {
	return max(s.CacheReadInputTokens-s.Local.CacheReadInputTokens, 0)
}

func (s SessionUsageSummary) DelegatedCacheWriteInputTokens() int {
	return max(s.CacheWriteInputTokens-s.Local.CacheWriteInputTokens, 0)
}

func (s SessionUsageSummary) DelegatedReasoningTokens() int {
	return max(s.ReasoningTokens-s.Local.ReasoningTokens, 0)
}

func (s SessionUsageSummary) DelegatedSteps() int {
	return max(s.Steps-s.Local.Steps, 0)
}

func (s SessionUsageSummary) DelegatedAttempts() int {
	return max(s.Attempts-s.Local.Attempts, 0)
}

func (s SessionUsageSummary) DelegatedEstimatedCost() float64 {
	value := s.EstimatedCost - s.Local.EstimatedCost
	if value < 0 {
		return 0
	}
	return value
}

func (s SessionUsageSummary) DelegatedMissingPricingTurns() int {
	return max(s.MissingPricingTurns-s.Local.MissingPricingTurns, 0)
}

func (s *SessionService) UsageSummary(ctx context.Context, sessionID string) (SessionUsageSummary, error) {
	if strings.TrimSpace(sessionID) == "" {
		return SessionUsageSummary{}, ErrSessionIDRequired
	}
	summary := SessionUsageSummary{
		RootSessionID: strings.TrimSpace(sessionID),
		Exact:         true,
	}
	visited := make(map[string]struct{}, 4)
	if err := s.collectUsageSummary(ctx, sessionUsageVisit{
		SessionID: strings.TrimSpace(sessionID),
		Depth:     0,
	}, visited, &summary); err != nil {
		return SessionUsageSummary{}, err
	}
	summary.ChildSessionCount = max(summary.SessionCount-1, 0)
	return summary, nil
}

func (r *Runtime) SessionUsageSummary(ctx context.Context, sessionID string) (SessionUsageSummary, error) {
	if r == nil || r.Sessions == nil {
		return SessionUsageSummary{}, nil
	}
	return r.Sessions.UsageSummary(ctx, sessionID)
}

type sessionUsageVisit struct {
	SessionID string
	AgentID   string
	Depth     int
}

func (s *SessionService) collectUsageSummary(ctx context.Context, visit sessionUsageVisit, visited map[string]struct{}, summary *SessionUsageSummary) error {
	sessionID := strings.TrimSpace(visit.SessionID)
	if sessionID == "" {
		return nil
	}
	if _, ok := visited[sessionID]; ok {
		return nil
	}
	visited[sessionID] = struct{}{}

	state, err := s.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	entry := sessionUsageEntryFromState(state, visit)
	summary.SessionCount++
	summary.UsageTurns += entry.UsageTurns
	summary.ToolCalls += entry.ToolCalls
	summary.CompletedToolCalls += entry.CompletedToolCalls
	summary.FailedToolCalls += entry.FailedToolCalls
	summary.ContractViolationCalls += entry.ContractViolationCalls
	summary.RequestTokens += entry.RequestTokens
	summary.CompletionTokens += entry.CompletionTokens
	summary.UnpricedRequestTokens += entry.UnpricedRequestTokens
	summary.UnpricedCompletionTokens += entry.UnpricedCompletionTokens
	summary.CacheReadInputTokens += entry.CacheReadInputTokens
	summary.CacheWriteInputTokens += entry.CacheWriteInputTokens
	summary.ReasoningTokens += entry.ReasoningTokens
	summary.Steps += entry.Steps
	summary.Attempts += entry.Attempts
	summary.EstimatedCost += entry.EstimatedCost
	summary.MissingPricingTurns += entry.MissingPricingTurns
	if entry.UsageTurns > 0 && !entry.Exact {
		summary.Exact = false
	}
	if visit.Depth == 0 {
		summary.Local = entry
	}
	summary.Sessions = append(summary.Sessions, entry)

	for _, child := range childUsageVisits(state) {
		child.Depth = visit.Depth + 1
		if err := s.collectUsageSummary(ctx, child, visited, summary); err != nil {
			return err
		}
	}
	return nil
}

func sessionUsageEntryFromState(state events.SessionState, visit sessionUsageVisit) SessionUsageEntry {
	entry := SessionUsageEntry{
		SessionID: strings.TrimSpace(state.SessionID),
		Depth:     max(visit.Depth, 0),
		AgentID:   strings.TrimSpace(visit.AgentID),
		Title:     strings.TrimSpace(state.Title),
		Exact:     true,
	}
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		entry.ToolCalls += turnVisibleToolCallCount(turn)
		completedToolCalls, failedToolCalls, contractViolationCalls := turnToolOutcomeCounts(turn)
		entry.CompletedToolCalls += completedToolCalls
		entry.FailedToolCalls += failedToolCalls
		entry.ContractViolationCalls += contractViolationCalls
		if turn.ProviderUsage == nil {
			continue
		}
		entry.UsageTurns++
		entry.Steps += max(turn.ProviderUsage.Steps, 0)
		entry.Attempts += max(turn.ProviderUsage.Attempts, 0)
		requestTokens, completionTokens, cacheReadTokens, cacheWriteTokens, reasoningTokens, exact, ok := turnUsageTotals(turn)
		if ok {
			entry.RequestTokens += requestTokens
			entry.CompletionTokens += completionTokens
			entry.CacheReadInputTokens += cacheReadTokens
			entry.CacheWriteInputTokens += cacheWriteTokens
			entry.ReasoningTokens += reasoningTokens
			if !exact {
				entry.Exact = false
			}
		}
		cost := max(turn.ProviderUsage.EstimatedInputCost, 0) + max(turn.ProviderUsage.EstimatedOutputCost, 0)
		entry.EstimatedCost += cost
		if cost <= 0 && requestTokens+completionTokens > 0 {
			entry.MissingPricingTurns++
			entry.UnpricedRequestTokens += requestTokens
			entry.UnpricedCompletionTokens += completionTokens
		}
	}
	return entry
}

func turnVisibleToolCallCount(turn *events.TurnState) int {
	if turn == nil {
		return 0
	}
	count := 0
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil {
			continue
		}
		if call.Declared || call.Executing || call.Completed {
			count++
		}
	}
	return count
}

func turnToolOutcomeCounts(turn *events.TurnState) (completed, failed, contractViolations int) {
	if turn == nil {
		return 0, 0, 0
	}
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || !call.Completed {
			continue
		}
		completed++
		if call.Succeeded {
			continue
		}
		failed++
		if strings.TrimSpace(call.FailureClass) == toolFailureClassContract {
			contractViolations++
		}
	}
	return completed, failed, contractViolations
}

func turnUsageTotals(turn *events.TurnState) (requestTokens, completionTokens, cacheReadTokens, cacheWriteTokens, reasoningTokens int, exact bool, ok bool) {
	if turn == nil {
		return 0, 0, 0, 0, 0, true, false
	}
	if turn.ProviderUsage != nil {
		requestTokens = max(turn.ProviderUsage.RequestTokens, 0)
		completionTokens = max(turn.ProviderUsage.CompletionTokens, 0)
	}
	if len(turn.ProviderAttempts) > 0 {
		reportedAttempts := 0
		for _, attempt := range turn.ProviderAttempts {
			if !turnUsageAttemptHasReportedUsage(attempt) {
				continue
			}
			reportedAttempts++
			cacheReadTokens += max(attempt.ReportedCacheReadInputTokens, 0)
			cacheWriteTokens += max(attempt.ReportedCacheWriteInputTokens, 0)
			reasoningTokens += max(attempt.ReportedReasoningTokens, 0)
		}
		if reportedAttempts > 0 {
			return requestTokens,
				completionTokens,
				cacheReadTokens,
				cacheWriteTokens,
				reasoningTokens,
				reportedAttempts == len(turn.ProviderAttempts),
				requestTokens > 0 || completionTokens > 0 || cacheReadTokens > 0 || cacheWriteTokens > 0 || reasoningTokens > 0
		}
		if turn.ProviderReportedUsage != nil && turn.ProviderReportedUsage.Attempts > 0 {
			cacheReadTokens = max(turn.ProviderReportedUsage.CacheReadInputTokens, 0)
			cacheWriteTokens = max(turn.ProviderReportedUsage.CacheWriteInputTokens, 0)
			reasoningTokens = max(turn.ProviderReportedUsage.ReasoningTokens, 0)
			if requestTokens == 0 && completionTokens == 0 {
				requestTokens = max(turn.ProviderReportedUsage.InputTokens, 0)
				completionTokens = max(turn.ProviderReportedUsage.OutputTokens, 0)
			}
			return requestTokens,
				completionTokens,
				cacheReadTokens,
				cacheWriteTokens,
				reasoningTokens,
				max(turn.ProviderReportedUsage.Attempts, 0) == len(turn.ProviderAttempts),
				requestTokens > 0 || completionTokens > 0 || cacheReadTokens > 0 || cacheWriteTokens > 0 || reasoningTokens > 0
		}
		if requestTokens > 0 || completionTokens > 0 {
			return requestTokens, completionTokens, 0, 0, 0, false, true
		}
		return 0, 0, 0, 0, 0, true, false
	}
	if turn.ProviderReportedUsage != nil && turn.ProviderReportedUsage.Attempts > 0 {
		cacheReadTokens = max(turn.ProviderReportedUsage.CacheReadInputTokens, 0)
		cacheWriteTokens = max(turn.ProviderReportedUsage.CacheWriteInputTokens, 0)
		reasoningTokens = max(turn.ProviderReportedUsage.ReasoningTokens, 0)
		if requestTokens == 0 && completionTokens == 0 {
			requestTokens = max(turn.ProviderReportedUsage.InputTokens, 0)
			completionTokens = max(turn.ProviderReportedUsage.OutputTokens, 0)
		}
		return requestTokens,
			completionTokens,
			cacheReadTokens,
			cacheWriteTokens,
			reasoningTokens,
			true,
			requestTokens > 0 || completionTokens > 0 || cacheReadTokens > 0 || cacheWriteTokens > 0 || reasoningTokens > 0
	}
	if requestTokens > 0 || completionTokens > 0 {
		return requestTokens, completionTokens, 0, 0, 0, false, true
	}
	return 0, 0, 0, 0, 0, true, false
}

func turnUsageAttemptHasReportedUsage(attempt events.TurnProviderAttemptState) bool {
	return attempt.ReportedRequestID != "" ||
		attempt.ReportedModel != "" ||
		attempt.ReportedInputTokens > 0 ||
		attempt.ReportedCacheReadInputTokens > 0 ||
		attempt.ReportedCacheWriteInputTokens > 0 ||
		attempt.ReportedOutputTokens > 0 ||
		attempt.ReportedReasoningTokens > 0 ||
		attempt.ReportedTotalTokens > 0 ||
		attempt.CachePricingApplied ||
		attempt.CachePricingMissing
}

func childUsageVisits(state events.SessionState) []sessionUsageVisit {
	parentSessionID := strings.TrimSpace(state.SessionID)
	if parentSessionID == "" {
		return nil
	}
	visits := make([]sessionUsageVisit, 0, 4)
	seen := make(map[string]struct{}, 4)
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		for _, handoffID := range turn.HandoffOrder {
			handoff := turn.Handoffs[handoffID]
			if handoff == nil {
				continue
			}
			if strings.TrimSpace(handoff.ParentSessionID) != parentSessionID {
				continue
			}
			childSessionID := strings.TrimSpace(handoff.ChildSessionID)
			if childSessionID == "" || childSessionID == parentSessionID {
				continue
			}
			if _, ok := seen[childSessionID]; ok {
				continue
			}
			seen[childSessionID] = struct{}{}
			visits = append(visits, sessionUsageVisit{
				SessionID: childSessionID,
				AgentID:   strings.TrimSpace(handoff.ChildAgentID),
			})
		}
	}
	return visits
}
