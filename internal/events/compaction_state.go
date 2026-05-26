package events

import "strings"

type CompactionAttemptState = ContextCompactionStartedPayload

type CompactionFailureState = ContextCompactionFailedPayload

type HistoryContinuationState = SessionHistoryContinuationUpdatedPayload

type HistoryCompactionUIState struct {
	Scope             CompactionScope
	StartedAtSeq      int64
	SummaryReadyAtSeq int64
	ResumedAtSeq      int64
	SourceTurnID      string
}

func cloneCompactionAttemptState(state *CompactionAttemptState) *CompactionAttemptState {
	if state == nil {
		return nil
	}
	out := *state
	return &out
}

func cloneCompactionFailureState(state *CompactionFailureState) *CompactionFailureState {
	if state == nil {
		return nil
	}
	out := *state
	return &out
}

func cloneHistoryContinuationState(state *HistoryContinuationState) *HistoryContinuationState {
	if state == nil {
		return nil
	}
	out := state.normalized()
	return &out
}

func cloneHistoryCompactionUIState(state *HistoryCompactionUIState) *HistoryCompactionUIState {
	if state == nil {
		return nil
	}
	out := *state
	return &out
}

func beginTurnHistoryCompactionUI(turn *TurnState, scope CompactionScope, sequence int64, sourceTurnID string) {
	if turn == nil || scope != CompactionScopeHistory {
		return
	}
	turn.HistoryCompactionUI = &HistoryCompactionUIState{
		Scope:        scope,
		StartedAtSeq: sequence,
		SourceTurnID: strings.TrimSpace(sourceTurnID),
	}
}

func markTurnHistoryCompactionSummaryReady(turn *TurnState, sequence int64) {
	if turn == nil || turn.HistoryCompactionUI == nil {
		return
	}
	if turn.HistoryCompactionUI.Scope != CompactionScopeHistory {
		return
	}
	if turn.HistoryCompactionUI.StartedAtSeq <= 0 {
		return
	}
	if sequence < turn.HistoryCompactionUI.StartedAtSeq {
		sequence = turn.HistoryCompactionUI.StartedAtSeq
	}
	turn.HistoryCompactionUI.SummaryReadyAtSeq = sequence
}

func resumeTurnHistoryCompactionUI(turn *TurnState, sequence int64) {
	if turn == nil || turn.HistoryCompactionUI == nil {
		return
	}
	if turn.HistoryCompactionUI.Scope != CompactionScopeHistory {
		return
	}
	if turn.HistoryCompactionUI.StartedAtSeq <= 0 {
		return
	}
	if turn.HistoryCompactionUI.SummaryReadyAtSeq <= 0 {
		return
	}
	if turn.HistoryCompactionUI.ResumedAtSeq > 0 {
		return
	}
	if sequence <= turn.HistoryCompactionUI.SummaryReadyAtSeq {
		return
	}
	turn.HistoryCompactionUI.ResumedAtSeq = sequence
}
