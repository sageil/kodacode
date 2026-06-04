package events

import "strings"

func (p *Projector) applyProviderPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case TurnProviderUsageRecordedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		attempt := providerAttemptStateFromPayload(payload)
		turn.ProviderAttempts = append(turn.ProviderAttempts, attempt)
		if usage, ok := turnContextUsageStateFromProviderAttempt(attempt); ok {
			turn.ContextUsage = &usage
		}
		recomputeTurnProviderUsageState(turn)
		return true, nil
	case TurnProviderUsageReportedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.ProviderAttempts = mergeTurnProviderAttemptReportedUsage(turn.ProviderAttempts, payload)
		if attempt := LatestAgentProviderAttempt(turn); attempt != nil {
			if usage, ok := turnContextUsageStateFromProviderAttempt(*attempt); ok {
				turn.ContextUsage = &usage
			}
		}
		if turn.ProviderUsage == nil {
			turn.ProviderUsage = &TurnProviderUsageState{}
		}
		if strings.TrimSpace(payload.Model) != "" {
			turn.ProviderUsage.Model = payload.Model
		}
		recomputeTurnProviderUsageState(turn)
		if turn.ProviderReportedUsage == nil {
			turn.ProviderReportedUsage = &TurnProviderReportedUsageState{}
		}
		if strings.TrimSpace(payload.Model) != "" {
			turn.ProviderReportedUsage.Model = payload.Model
		}
		if strings.TrimSpace(payload.RequestID) != "" {
			turn.ProviderReportedUsage.RequestID = payload.RequestID
		}
		if payload.Step > turn.ProviderReportedUsage.Steps {
			turn.ProviderReportedUsage.Steps = payload.Step
		}
		turn.ProviderReportedUsage.Attempts++
		turn.ProviderReportedUsage.InputTokens += payload.InputTokens
		turn.ProviderReportedUsage.CacheReadInputTokens += payload.CacheReadInputTokens
		turn.ProviderReportedUsage.CacheWriteInputTokens += payload.CacheWriteInputTokens
		turn.ProviderReportedUsage.OutputTokens += payload.OutputTokens
		turn.ProviderReportedUsage.ReasoningTokens += payload.ReasoningTokens
		turn.ProviderReportedUsage.TotalTokens += payload.TotalTokens
		turn.ProviderReportedUsage.EstimatedCacheSavingsCost += payload.EstimatedCacheSavingsCost
		turn.ProviderReportedUsage.CachePricingApplied = turn.ProviderReportedUsage.CachePricingApplied || payload.CachePricingApplied
		turn.ProviderReportedUsage.CachePricingMissing = turn.ProviderReportedUsage.CachePricingMissing || payload.CachePricingMissing
		return true, nil
	default:
		return false, nil
	}
}
