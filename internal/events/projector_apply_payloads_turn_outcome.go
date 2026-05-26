package events

func (p *Projector) applyTurnOutcomePayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case TurnDonePayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.StreamingText = ""
		clearTurnRetryState(turn)
		clearTurnCompactionAttemptState(turn)
		turn.HistoryCompactionUI = nil
		turn.Status = TurnStatusCompleted
		turn.CompletedAtSeq = event.Sequence
		return true, nil
	case TurnRetryScheduledPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.Status = TurnStatusRunning
		turn.StreamingText = ""
		turn.Retry = &TurnRetryState{
			Message:     payload.Message,
			Attempt:     payload.Attempt,
			MaxAttempts: payload.MaxAttempts,
			RetryAt:     payload.RetryAt,
		}
		clearUndeclaredToolCalls(turn)
		return true, nil
	case ReviewRecordedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.Review = reviewStateFromPayload(payload)
		p.applyReviewRecorded(payload)
		turn.Transcript = append(turn.Transcript, TranscriptEntryState{
			Kind:     TranscriptEntryReview,
			Sequence: event.Sequence,
		})
		return true, nil
	case TurnCanceledPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.StreamingText = ""
		clearTurnRetryState(turn)
		clearTurnCompactionAttemptState(turn)
		turn.HistoryCompactionUI = nil
		turn.Status = TurnStatusCanceled
		turn.Error = ""
		turn.ErrorCode = TurnFailureCodeUnknown
		turn.ErrorRetryable = false
		turn.Transcript = filterCanceledTurnErrors(turn.Transcript, payload.Message)
		turn.CompletedAtSeq = event.Sequence
		p.clearPendingInteractionsForTurn(event.TurnID, payload.Message)
		return true, nil
	case TurnErrorPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.StreamingText = ""
		clearTurnRetryState(turn)
		clearTurnCompactionAttemptState(turn)
		turn.HistoryCompactionUI = nil
		turn.Status = TurnStatusFailed
		turn.Error = payload.Message
		turn.ErrorCode = payload.Code
		turn.ErrorRetryable = payload.Retryable
		turn.Transcript = append(turn.Transcript, TranscriptEntryState{
			Kind:     TranscriptEntryError,
			Sequence: event.Sequence,
			Text:     payload.Message,
		})
		turn.CompletedAtSeq = event.Sequence
		return true, nil
	default:
		return false, nil
	}
}

func (p *Projector) clearPendingInteractionsForTurn(turnID, message string) {
	for requestID, request := range p.state.PendingExecutions {
		if request == nil || request.TurnID != turnID {
			continue
		}
		delete(p.state.PendingExecutions, requestID)
		p.state.PendingExecutionOrder = filterPendingRequestOrder(p.state.PendingExecutionOrder, requestID)
		if request.ExecutionID != "" {
			delete(p.state.ApprovedExecutions, request.ExecutionID)
		}
	}
	for requestID, request := range p.state.PendingPermissions {
		if request == nil || request.TurnID != turnID {
			continue
		}
		delete(p.state.PendingPermissions, requestID)
		p.state.PendingPermissionOrder = filterPendingRequestOrder(p.state.PendingPermissionOrder, requestID)
	}
	for questionID, request := range p.state.PendingQuestions {
		if request == nil || request.TurnID != turnID {
			continue
		}
		delete(p.state.PendingQuestions, questionID)
		p.state.PendingQuestionOrder = filterPendingRequestOrder(p.state.PendingQuestionOrder, questionID)
		if request.ToolCallID != "" {
			call := p.ensureCall(request.TurnID, request.ToolCallID, request.ToolName)
			call.ToolName = request.ToolName
			call.Executing = false
			call.Completed = true
			call.Error = message
		}
	}
	turn := p.state.Turns[turnID]
	if turn == nil {
		return
	}
	for _, handoffID := range turn.HandoffOrder {
		handoff := turn.Handoffs[handoffID]
		if handoff == nil {
			continue
		}
		switch handoff.Status {
		case AgentResultStatusPendingPermission, AgentResultStatusPendingQuestion:
			handoff.Status = AgentResultStatusFailed
			handoff.Error = message
			handoff.PermissionRequestID = ""
			handoff.QuestionRequestID = ""
		}
	}
}
