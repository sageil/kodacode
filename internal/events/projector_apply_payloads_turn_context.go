package events

import (
	"slices"
	"strings"
)

func (p *Projector) applyTurnContextPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case UserMessagePayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnRetryState(turn)
		turn.UserText = payload.Content
		turn.UserAttachments = cloneUserAttachmentPayloads(payload.Attachments)
		turn.Transcript = append(turn.Transcript, TranscriptEntryState{
			Kind:     TranscriptEntryUser,
			Sequence: event.Sequence,
			Text:     renderUserTranscriptText(payload),
		})
		return true, nil
	case TurnConfiguredPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.Config = &TurnConfigState{
			AgentID:                   payload.AgentID,
			WorkflowID:                payload.WorkflowID,
			WorkflowPhaseID:           payload.WorkflowPhaseID,
			SkillIDs:                  append([]string(nil), payload.SkillIDs...),
			SelectedSkillIDs:          slices.Clone(payload.SelectedSkillIDs),
			Model:                     payload.Model,
			PreserveSessionModel:      payload.PreserveSessionModel,
			ThinkingEnabled:           payload.ThinkingEnabled,
			ThinkingMode:              payload.ThinkingMode,
			ResponseStyle:             payload.ResponseStyle,
			AllowedTools:              slices.Clone(payload.AllowedTools),
			SupportsReasoningVariants: payload.SupportsReasoningVariants,
			SupportsThinkingOutput:    payload.SupportsThinkingOutput,
			HideAssistantPreview:      payload.HideAssistantPreview,
		}
		return true, nil
	case TurnContinuationStartedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.ContinuationStart = &TurnContinuationState{
			PreviousTurnID: payload.PreviousTurnID,
			Reason:         payload.Reason,
			Summary: TurnWorkStateSummaryState{
				Objective:     payload.Summary.Objective,
				Decisions:     append([]string(nil), payload.Summary.Decisions...),
				TouchedPaths:  append([]string(nil), payload.Summary.TouchedPaths...),
				CompletedWork: append([]string(nil), payload.Summary.CompletedWork...),
				Verification:  append([]string(nil), payload.Summary.Verification...),
				Failures:      append([]string(nil), payload.Summary.Failures...),
				OpenItems:     append([]string(nil), payload.Summary.OpenItems...),
			},
		}
		turn.Transcript = append(turn.Transcript, TranscriptEntryState{
			Kind:     TranscriptEntryWorklog,
			Sequence: event.Sequence,
			Text:     renderTurnContinuationTranscriptText(payload),
		})
		return true, nil
	case PromptCompiledPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		fragments := make([]PromptFragmentState, len(payload.Fragments))
		for i, fragment := range payload.Fragments {
			fragments[i] = PromptFragmentState(fragment)
		}
		layerPayloads := payload.Layers
		if len(layerPayloads) == 0 {
			layerPayloads = PromptLayersFromFragments(payload.Fragments)
		}
		layers := make([]PromptLayerState, len(layerPayloads))
		for i, layer := range layerPayloads {
			layers[i] = PromptLayerState(layer)
		}
		turn.Prompt = &PromptState{
			Shape:            payload.Shape,
			BaseInstructions: payload.BaseInstructions,
			Instructions:     payload.Instructions,
			CacheablePrefix:  payload.CacheablePrefix,
			DynamicSuffix:    payload.DynamicSuffix,
			Layers:           layers,
			Fragments:        fragments,
		}
		return true, nil
	case ContextPrunedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.Pruning = &PruningState{
			PriorTurns:          payload.PriorTurns,
			PriorInputBytes:     payload.PriorInputBytes,
			RawPriorTurns:       payload.RawPriorTurns,
			RawInputBytes:       payload.RawInputBytes,
			CompactedPriorTurns: payload.CompactedPriorTurns,
			CompactedInputBytes: payload.CompactedInputBytes,
			OmittedPriorTurns:   payload.OmittedPriorTurns,
			OmittedInputBytes:   payload.OmittedInputBytes,
		}
		return true, nil
	case ContextCompactionStartedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.CompactionAttempt = cloneCompactionAttemptState(&payload)
		turn.CompactionFailure = nil
		beginTurnHistoryCompactionUI(turn, payload.Scope, event.Sequence, event.TurnID)
		return true, nil
	case ContextCompactionFailedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.CompactionAttempt = nil
		turn.CompactionFailure = cloneCompactionFailureState(&payload)
		turn.HistoryCompactionUI = nil
		return true, nil
	case SessionHistoryContinuationUpdatedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.CompactionAttempt = nil
		turn.CompactionFailure = nil
		turn.Continuation = cloneHistoryContinuationState(&payload)
		markTurnHistoryCompactionSummaryReady(turn, event.Sequence)
		if summary := strings.TrimSpace(payload.RenderedSummary); summary != "" {
			turn.Transcript = append(turn.Transcript, TranscriptEntryState{
				Kind:     TranscriptEntryCompaction,
				Sequence: event.Sequence,
				Text:     summary,
			})
		}
		if usage, ok := turnContextUsageStateFromContinuation(turn.Continuation); ok {
			turn.ContextUsage = &usage
		}
		return true, nil
	default:
		return false, nil
	}
}

func renderTurnContinuationTranscriptText(payload TurnContinuationStartedPayload) string {
	switch strings.TrimSpace(payload.Reason) {
	case TurnContinuationReasonContextLimit:
		return "Continuing automatically after the previous turn reached the model input limit."
	case TurnContinuationReasonQuestionAnswer:
		return "Continuing in a new turn after the user answered a pending question."
	default:
		return "Continuing automatically from the previous turn."
	}
}
