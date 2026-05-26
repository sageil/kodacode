package events

import "strings"

func (p *Projector) applyStreamPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case AssistantPreviewDeltaPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		turn.StreamingText += payload.Content
		return true, nil
	case AssistantPreviewResetPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		turn.StreamingText = ""
		return true, nil
	case AssistantWorklogCommitPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		turn.StreamingText = ""
		turn.Transcript = append(turn.Transcript, TranscriptEntryState{
			Kind:     TranscriptEntryWorklog,
			Sequence: event.Sequence,
			Text:     payload.Content,
		})
		return true, nil
	case AssistantCommitPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		previous := turn.AssistantText
		turn.AssistantText = payload.Content
		turn.StreamingText = ""
		content := payload.Content
		if previous != "" && strings.HasPrefix(payload.Content, previous) {
			content = payload.Content[len(previous):]
		}
		if strings.TrimSpace(content) != "" {
			if n := len(turn.Transcript); n > 0 && turn.Transcript[n-1].Kind == TranscriptEntryAssistant {
				turn.Transcript[n-1].Sequence = event.Sequence
				turn.Transcript[n-1].Text += content
			} else {
				turn.Transcript = append(turn.Transcript, TranscriptEntryState{
					Kind:     TranscriptEntryAssistant,
					Sequence: event.Sequence,
					Text:     content,
				})
			}
		}
		if turn.Continuation == nil {
			if attempt := LatestAgentProviderAttempt(turn); attempt != nil {
				if usage, ok := turnContextUsageStateFromProviderAttempt(*attempt); ok {
					turn.ContextUsage = &usage
				}
			}
		}
		return true, nil
	case ReasoningDeltaPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		p.appendReasoning(event.TurnID, turn, event.Sequence, payload.Content, payload.SegmentID)
		return true, nil
	case AnthropicThinkingCommittedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		return true, nil
	case OpenAIReasoningCommittedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		return true, nil
	case ToolCallDeltaPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		call := p.ensureCall(event.TurnID, payload.CallID, payload.ToolName)
		call.Input += payload.InputDelta
		call.LastUpdatedSeq = event.Sequence
		return true, nil
	case ToolCallDeclaredPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		clearTurnPassTransientState(turn, event.Sequence)
		call := p.ensureCall(event.TurnID, payload.CallID, payload.ToolName)
		call.ToolName = payload.ToolName
		call.Input = payload.Input
		call.Declared = true
		call.LastUpdatedSeq = event.Sequence
		turn.Transcript = append(turn.Transcript, TranscriptEntryState{
			Kind:     TranscriptEntryTool,
			Sequence: event.Sequence,
			CallID:   payload.CallID,
		})
		return true, nil
	case ToolCallBatchPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.ToolCallBatches = append(turn.ToolCallBatches, ToolCallBatchState{
			CallIDs:  append([]string(nil), payload.CallIDs...),
			Sequence: event.Sequence,
		})
		return true, nil
	default:
		return false, nil
	}
}
