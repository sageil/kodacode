package events

import (
	"slices"
	"strings"
)

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
	case AgentHandoffPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		if _, ok := turn.Handoffs[payload.HandoffID]; !ok {
			turn.HandoffOrder = append(turn.HandoffOrder, payload.HandoffID)
		}
		turn.Handoffs[payload.HandoffID] = &AgentHandoffState{
			HandoffID:          payload.HandoffID,
			ToolCallID:         payload.ToolCallID,
			ParentSessionID:    payload.ParentSessionID,
			ParentTurnID:       payload.ParentTurnID,
			ParentAgentID:      payload.ParentAgentID,
			ChildSessionID:     payload.ChildSessionID,
			ChildTurnID:        payload.ChildTurnID,
			ChildAgentID:       payload.ChildAgentID,
			Task:               payload.Task,
			ContextSummary:     payload.ContextSummary,
			SourceHandoffIDs:   append([]string(nil), payload.SourceHandoffIDs...),
			ProvidedKinds:      append([]string(nil), payload.ProvidedKinds...),
			ExplorationEntries: append([]AgentHandoffExplorationEntry(nil), payload.ExplorationEntries...),
			Model:              payload.Model,
			AllowedTools:       slices.Clone(payload.AllowedTools),
		}
		if strings.TrimSpace(event.SessionID) == strings.TrimSpace(payload.ParentSessionID) &&
			strings.TrimSpace(event.TurnID) == strings.TrimSpace(payload.ParentTurnID) &&
			strings.TrimSpace(payload.ToolCallID) != "" {
			call := p.ensureCall(event.TurnID, payload.ToolCallID, "delegate")
			call.HandoffID = payload.HandoffID
		}
		return true, nil
	case AgentHandoffPreviewPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		handoff, ok := turn.Handoffs[payload.HandoffID]
		if !ok {
			turn.HandoffOrder = append(turn.HandoffOrder, payload.HandoffID)
			handoff = &AgentHandoffState{HandoffID: payload.HandoffID}
			turn.Handoffs[payload.HandoffID] = handoff
		}
		handoff.ChildSessionID = payload.ChildSessionID
		handoff.ChildTurnID = payload.ChildTurnID
		handoff.PreviewActive = payload.Active
		handoff.PreviewToolName = payload.ToolName
		handoff.PreviewAction = payload.Action
		handoff.PreviewAssistantText = payload.AssistantText
		return true, nil
	case AgentResultPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		handoff, ok := turn.Handoffs[payload.HandoffID]
		if !ok {
			turn.HandoffOrder = append(turn.HandoffOrder, payload.HandoffID)
			handoff = &AgentHandoffState{HandoffID: payload.HandoffID}
			turn.Handoffs[payload.HandoffID] = handoff
		}
		handoff.ChildSessionID = payload.ChildSessionID
		handoff.ChildTurnID = payload.ChildTurnID
		handoff.PreviewActive = false
		handoff.PreviewToolName = ""
		handoff.PreviewAction = ""
		handoff.PreviewAssistantText = ""
		handoff.Status = payload.Status
		handoff.AssistantText = payload.AssistantText
		handoff.Error = payload.Error
		handoff.PermissionRequestID = payload.PermissionRequestID
		handoff.PermissionKind = payload.PermissionKind
		handoff.PermissionToolName = payload.PermissionToolName
		handoff.PermissionAccess = payload.PermissionAccess
		handoff.PermissionPath = payload.PermissionPath
		handoff.PermissionDir = payload.PermissionDir
		handoff.PermissionCommand = payload.PermissionCommand
		handoff.PermissionReason = payload.PermissionReason
		handoff.ExecutionApproval = cloneExecutionApprovalState(payload.ExecutionApproval)
		handoff.QuestionRequestID = payload.QuestionRequestID
		handoff.QuestionToolName = payload.QuestionToolName
		handoff.QuestionText = payload.QuestionText
		handoff.QuestionOptions = append([]string(nil), payload.QuestionOptions...)
		return true, nil
	case AgentResultReusedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		handoff, ok := turn.Handoffs[payload.HandoffID]
		if !ok {
			turn.HandoffOrder = append(turn.HandoffOrder, payload.HandoffID)
			handoff = &AgentHandoffState{HandoffID: payload.HandoffID}
			turn.Handoffs[payload.HandoffID] = handoff
		}
		handoff.ChildSessionID = payload.ChildSessionID
		handoff.ChildTurnID = payload.ChildTurnID
		handoff.PreviewActive = false
		handoff.PreviewToolName = ""
		handoff.PreviewAction = ""
		handoff.PreviewAssistantText = ""
		handoff.Reused = true
		handoff.ReusedContent = payload.Content
		return true, nil
	default:
		return false, nil
	}
}
