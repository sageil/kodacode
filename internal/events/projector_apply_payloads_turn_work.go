package events

func (p *Projector) applyTurnWorkPayload(event Event) (bool, error) {
	switch payload := event.Payload.(type) {
	case TurnWorkStateUpdatedPayload:
		turn := p.ensureTurn(event.TurnID)
		turn.LastUpdatedAtSeq = event.Sequence
		turn.WorkState = turnWorkStateFromPayload(payload)
		return true, nil
	default:
		return false, nil
	}
}

func turnWorkStateFromPayload(payload TurnWorkStateUpdatedPayload) *TurnWorkState {
	return &TurnWorkState{
		Summary: TurnWorkStateSummaryState{
			Objective:     payload.Summary.Objective,
			Decisions:     append([]string(nil), payload.Summary.Decisions...),
			TouchedPaths:  append([]string(nil), payload.Summary.TouchedPaths...),
			CompletedWork: append([]string(nil), payload.Summary.CompletedWork...),
			Verification:  append([]string(nil), payload.Summary.Verification...),
			Failures:      append([]string(nil), payload.Summary.Failures...),
			OpenItems:     append([]string(nil), payload.Summary.OpenItems...),
		},
		NativeContinuation: turnNativeContinuationFromPayload(payload.NativeContinuation),
	}
}

func turnNativeContinuationFromPayload(payload *TurnNativeContinuationPayload) *TurnNativeContinuationState {
	if payload == nil {
		return nil
	}
	return &TurnNativeContinuationState{
		Contract: payload.Contract,
		Slice:    cloneTurnWorkContinuationSlice(payload.Slice),
	}
}
