package events

import "github.com/sageil/kodacode/internal/textdiff"

func NewProjectorFromSnapshot(state SessionState) *Projector {
	cloned := cloneSessionState(state)
	normalizeSessionProviderUsageState(&cloned)
	return &Projector{
		state:     cloned,
		toolCalls: indexToolCalls(cloned),
		reasoning: make(map[string]*turnReasoningAccumulator),
	}
}

// NewProjectorFromOwnedSnapshot transfers ownership of state into the projector.
// Callers must not retain or mutate maps, slices, or nested pointers from state
// after passing it here.
func NewProjectorFromOwnedSnapshot(state SessionState) *Projector {
	normalizeSessionProviderUsageState(&state)
	return &Projector{
		state:     state,
		toolCalls: indexToolCalls(state),
		reasoning: make(map[string]*turnReasoningAccumulator),
	}
}

func NewProjectorFromSnapshotAt(state SessionState, lastSequence int64) *Projector {
	cloned := cloneSessionState(state)
	cloned.LastSequence = lastSequence
	normalizeSessionProviderUsageState(&cloned)
	return &Projector{
		state:     cloned,
		toolCalls: indexToolCalls(cloned),
		reasoning: make(map[string]*turnReasoningAccumulator),
	}
}

func normalizeSessionProviderUsageState(state *SessionState) {
	if state == nil {
		return
	}
	for _, turn := range state.Turns {
		recomputeTurnProviderUsageState(turn)
		normalizeTurnContextUsageState(turn)
	}
}

func (p *Projector) Snapshot() SessionState {
	return cloneSessionState(p.state)
}

// CurrentState returns the projector's current state for read-only inspection.
// Callers must not mutate the returned maps, slices, or nested pointers.
func (p *Projector) CurrentState() SessionState {
	return p.state
}

func (p *Projector) ToolCall(callID string) *ToolCallState {
	if p == nil || callID == "" {
		return nil
	}
	return p.toolCalls[callID]
}

func SnapshotSessionState(state SessionState) SessionState {
	out := cloneSessionState(state)
	for _, turn := range out.Turns {
		turn.StreamingText = ""

		keptOrder := turn.ToolCallOrder[:0]
		for _, callID := range turn.ToolCallOrder {
			call := turn.ToolCalls[callID]
			if call == nil {
				continue
			}
			if snapshotOmitsToolCall(call) {
				delete(turn.ToolCalls, callID)
				continue
			}
			if call.Executing && !call.Completed {
				call.Output = ""
				call.Error = ""
				call.OutputBlob = nil
				call.ErrorBlob = nil
				call.OutputTruncated = false
				call.ErrorTruncated = false
				if call.Execution != nil {
					call.Execution.Output = ""
					call.Execution.Error = ""
					call.Execution.OutputBlob = nil
					call.Execution.ErrorBlob = nil
					call.Execution.OutputTruncated = false
					call.Execution.ErrorTruncated = false
				}
			}
			if call.Completed && !SnapshotRetainsToolBody(call) {
				call.Output = ""
				call.Error = ""
				if call.Execution != nil {
					call.Execution.Output = ""
					call.Execution.Error = ""
				}
			}
			keptOrder = append(keptOrder, callID)
		}
		if len(keptOrder) == 0 {
			turn.ToolCallOrder = nil
		} else {
			turn.ToolCallOrder = keptOrder
		}
	}
	return out
}

func cloneSessionState(state SessionState) SessionState {
	out := SessionState{
		SessionID:                    state.SessionID,
		WorkspaceRoot:                state.WorkspaceRoot,
		AdditionalWorkspaceRoots:     append([]string(nil), state.AdditionalWorkspaceRoots...),
		PermissionMode:               state.PermissionMode,
		ProviderRequestLimitDisabled: state.ProviderRequestLimitDisabled,
		Branch:                       cloneSessionBranchState(state.Branch),
		Model:                        state.Model,
		Title:                        state.Title,
		SessionGrantDecisions:        append([]SessionGrantDecisionState(nil), state.SessionGrantDecisions...),
		WorkspaceGrants:              append([]WorkspaceGrantState(nil), state.WorkspaceGrants...),
		ExecutionGrants:              cloneExecutionGrants(state.ExecutionGrants),
		NetworkGrants:                append([]NetworkGrantState(nil), state.NetworkGrants...),
		TaskOrder:                    append([]string(nil), state.TaskOrder...),
		Tasks:                        cloneTaskStates(state.Tasks),
		ReviewOrder:                  append([]string(nil), state.ReviewOrder...),
		Reviews:                      cloneReviewStates(state.Reviews),
		PlanOrder:                    append([]string(nil), state.PlanOrder...),
		Plans:                        clonePlanStates(state.Plans),
		ApprovedExecutions:           make(map[string]*ApprovedExecutionState, len(state.ApprovedExecutions)),
		PendingExecutionOrder:        append([]string(nil), state.PendingExecutionOrder...),
		PendingExecutions:            make(map[string]*ExecutionApprovalState, len(state.PendingExecutions)),
		PendingPermissionOrder:       append([]string(nil), state.PendingPermissionOrder...),
		PendingPermissions:           make(map[string]*PermissionRequestState, len(state.PendingPermissions)),
		PendingQuestionOrder:         append([]string(nil), state.PendingQuestionOrder...),
		PendingQuestions:             make(map[string]*QuestionRequestState, len(state.PendingQuestions)),
		QuestionAnswers:              make(map[string]*QuestionAnswerState, len(state.QuestionAnswers)),
		LastSequence:                 state.LastSequence,
		TurnOrder:                    append([]string(nil), state.TurnOrder...),
		Turns:                        make(map[string]*TurnState, len(state.Turns)),
	}
	for id, pending := range state.PendingExecutions {
		out.PendingExecutions[id] = cloneExecutionApprovalState(pending)
	}
	for id, approved := range state.ApprovedExecutions {
		out.ApprovedExecutions[id] = cloneApprovedExecutionState(approved)
	}
	for id, pending := range state.PendingPermissions {
		copyPending := *pending
		out.PendingPermissions[id] = &copyPending
	}
	for id, pending := range state.PendingQuestions {
		copyPending := *pending
		copyPending.Options = append([]string(nil), pending.Options...)
		out.PendingQuestions[id] = &copyPending
	}
	for id, answer := range state.QuestionAnswers {
		copyAnswer := *answer
		out.QuestionAnswers[id] = &copyAnswer
	}
	for id, turn := range state.Turns {
		out.Turns[id] = turn.clone()
	}
	return out
}

func cloneSessionBranchState(state *SessionBranchState) *SessionBranchState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func cloneExecutionGrants(grants []ExecutionGrantState) []ExecutionGrantState {
	if len(grants) == 0 {
		return nil
	}
	out := make([]ExecutionGrantState, 0, len(grants))
	for _, grant := range grants {
		out = append(out, ExecutionGrantState{
			PrefixRule:     append([]string(nil), grant.PrefixRule...),
			SessionPaths:   append([]string(nil), grant.SessionPaths...),
			NetworkTargets: append([]string(nil), grant.NetworkTargets...),
		})
	}
	return out
}

func cloneUserAttachmentPayloads(attachments []UserAttachmentPayload) []UserAttachmentPayload {
	if len(attachments) == 0 {
		return nil
	}
	out := make([]UserAttachmentPayload, len(attachments))
	copy(out, attachments)
	return out
}

func (p *Projector) ensureTurn(turnID string) *TurnState {
	if turn, ok := p.state.Turns[turnID]; ok {
		return turn
	}

	turn := &TurnState{
		TurnID:    turnID,
		Status:    TurnStatusRunning,
		ToolCalls: make(map[string]*ToolCallState),
		Handoffs:  make(map[string]*AgentHandoffState),
	}
	p.state.Turns[turnID] = turn
	p.state.TurnOrder = append(p.state.TurnOrder, turnID)
	return turn
}

func (p *Projector) ensureCall(turnID, callID, toolName string) *ToolCallState {
	turn := p.ensureTurn(turnID)
	call := turn.ensureCall(callID, toolName)
	if callID != "" {
		if p.toolCalls == nil {
			p.toolCalls = make(map[string]*ToolCallState)
		}
		p.toolCalls[callID] = call
	}
	return call
}

func (t *TurnState) ensureCall(callID, toolName string) *ToolCallState {
	if call, ok := t.ToolCalls[callID]; ok {
		if toolName != "" {
			call.ToolName = toolName
		}
		return call
	}

	call := &ToolCallState{
		CallID:   callID,
		ToolName: toolName,
	}
	t.ToolCalls[callID] = call
	t.ToolCallOrder = append(t.ToolCallOrder, callID)
	return call
}

func (t *TurnState) clone() *TurnState {
	out := &TurnState{
		TurnID:                t.TurnID,
		Status:                t.Status,
		UserText:              t.UserText,
		UserAttachments:       cloneUserAttachmentPayloads(t.UserAttachments),
		Config:                cloneTurnConfigState(t.Config),
		ContinuationStart:     cloneTurnContinuationState(t.ContinuationStart),
		Prompt:                clonePromptState(t.Prompt),
		Pruning:               clonePruningState(t.Pruning),
		CompactionAttempt:     cloneCompactionAttemptState(t.CompactionAttempt),
		CompactionFailure:     cloneCompactionFailureState(t.CompactionFailure),
		HistoryCompactionUI:   cloneHistoryCompactionUIState(t.HistoryCompactionUI),
		Continuation:          cloneHistoryContinuationState(t.Continuation),
		ContextUsage:          cloneTurnContextUsageState(t.ContextUsage),
		Handoffs:              make(map[string]*AgentHandoffState, len(t.Handoffs)),
		HandoffOrder:          append([]string(nil), t.HandoffOrder...),
		AssistantText:         t.AssistantText,
		StreamingText:         t.StreamingText,
		ReasoningText:         t.ReasoningText,
		Retry:                 cloneTurnRetryState(t.Retry),
		ProviderUsage:         cloneTurnProviderUsageState(t.ProviderUsage),
		ProviderReportedUsage: cloneTurnProviderReportedUsageState(t.ProviderReportedUsage),
		ProviderAttempts:      cloneTurnProviderAttemptStates(t.ProviderAttempts),
		Review:                cloneReviewState(t.Review),
		WorkState:             cloneTurnWorkState(t.WorkState),
		Error:                 t.Error,
		ErrorCode:             t.ErrorCode,
		ErrorRetryable:        t.ErrorRetryable,
		Transcript:            append([]TranscriptEntryState(nil), t.Transcript...),
		ToolCallBatches:       cloneToolCallBatchStates(t.ToolCallBatches),
		ToolCallOrder:         append([]string(nil), t.ToolCallOrder...),
		ToolCalls:             make(map[string]*ToolCallState, len(t.ToolCalls)),
		CompletedAtSeq:        t.CompletedAtSeq,
		LastUpdatedAtSeq:      t.LastUpdatedAtSeq,
	}
	for id, call := range t.ToolCalls {
		copyCall := *call
		copyCall.ReusedFromSessionID = call.ReusedFromSessionID
		copyCall.ReusedFromTurnID = call.ReusedFromTurnID
		copyCall.Execution = cloneExecutionState(call.Execution)
		copyCall.Runtime = cloneToolExecRuntimeState(call.Runtime)
		copyCall.ErrorDetail = cloneToolErrorDetail(call.ErrorDetail)
		copyCall.StructuredResult = cloneStructuredResult(call.StructuredResult)
		copyCall.MutationRanges = cloneMutationRanges(call.MutationRanges)
		copyCall.WriteMutation = cloneWriteMutation(call.WriteMutation)
		copyCall.WriteMutations = cloneWriteMutations(call.WriteMutations)
		copyCall.ObservedResources = cloneObservedResources(call.ObservedResources)
		copyCall.OutputBlob = cloneToolResultBlobRef(call.OutputBlob)
		copyCall.ErrorBlob = cloneToolResultBlobRef(call.ErrorBlob)
		out.ToolCalls[id] = &copyCall
	}
	for id, handoff := range t.Handoffs {
		out.Handoffs[id] = cloneAgentHandoffState(handoff)
	}
	return out
}

func cloneToolCallBatchStates(states []ToolCallBatchState) []ToolCallBatchState {
	if len(states) == 0 {
		return nil
	}
	cloned := make([]ToolCallBatchState, 0, len(states))
	for _, state := range states {
		cloned = append(cloned, ToolCallBatchState{
			CallIDs:  append([]string(nil), state.CallIDs...),
			Sequence: state.Sequence,
		})
	}
	return cloned
}

func toolExecRuntimeStateFromPayload(payload ToolExecEndPayload) *ToolExecRuntimeState {
	if payload.Backend == "" {
		return nil
	}
	return &ToolExecRuntimeState{
		Backend: payload.Backend,
	}
}

func cloneTurnWorkState(state *TurnWorkState) *TurnWorkState {
	if state == nil {
		return nil
	}
	cloned := &TurnWorkState{
		Summary: TurnWorkStateSummaryState{
			Objective:     state.Summary.Objective,
			Decisions:     append([]string(nil), state.Summary.Decisions...),
			TouchedPaths:  append([]string(nil), state.Summary.TouchedPaths...),
			CompletedWork: append([]string(nil), state.Summary.CompletedWork...),
			Verification:  append([]string(nil), state.Summary.Verification...),
			Failures:      append([]string(nil), state.Summary.Failures...),
			OpenItems:     append([]string(nil), state.Summary.OpenItems...),
		},
	}
	if state.NativeContinuation != nil {
		cloned.NativeContinuation = &TurnNativeContinuationState{
			Contract: state.NativeContinuation.Contract,
			Slice:    cloneTurnWorkContinuationSlice(state.NativeContinuation.Slice),
		}
	}
	return cloned
}

func cloneTurnWorkContinuationSlice(slice SessionHistoryTurnPayload) SessionHistoryTurnPayload {
	return SessionHistoryTurnPayload{
		TurnID:              slice.TurnID,
		UserText:            slice.UserText,
		UserAttachments:     append([]SessionHistoryAttachmentPayload(nil), slice.UserAttachments...),
		AssistantText:       slice.AssistantText,
		ReasoningText:       slice.ReasoningText,
		WorkspacePaths:      append([]string(nil), slice.WorkspacePaths...),
		RuntimeNotes:        append([]SessionHistoryRuntimeNotePayload(nil), slice.RuntimeNotes...),
		AssistantEntries:    append([]SessionHistoryAssistantEntryPayload(nil), slice.AssistantEntries...),
		AnthropicThinking:   append([]SessionHistoryAnthropicThinkingPayload(nil), slice.AnthropicThinking...),
		ToolCalls:           cloneTurnWorkToolCalls(slice.ToolCalls),
		ToolResults:         cloneTurnWorkToolResults(slice.ToolResults),
		Executions:          append([]SessionHistoryExecutionPayload(nil), slice.Executions...),
		EntryOrder:          append([]SessionHistoryEntryPayload(nil), slice.EntryOrder...),
		ToolCallCount:       slice.ToolCallCount,
		TerminalStatus:      slice.TerminalStatus,
		TerminalSequence:    slice.TerminalSequence,
		TerminalError:       slice.TerminalError,
		TerminalRetryable:   slice.TerminalRetryable,
		SuccessfulToolCalls: slice.SuccessfulToolCalls,
		FailedToolCalls:     slice.FailedToolCalls,
		ToolNames:           append([]string(nil), slice.ToolNames...),
		FailedToolNames:     append([]string(nil), slice.FailedToolNames...),
	}
}

func cloneTurnWorkToolCalls(calls []SessionHistoryToolCallPayload) []SessionHistoryToolCallPayload {
	if len(calls) == 0 {
		return nil
	}
	cloned := make([]SessionHistoryToolCallPayload, len(calls))
	for index, call := range calls {
		cloned[index] = call
		cloned[index].GoogleThoughtSignature = append([]byte(nil), call.GoogleThoughtSignature...)
	}
	return cloned
}

func cloneTurnWorkToolResults(results []SessionHistoryToolResultPayload) []SessionHistoryToolResultPayload {
	if len(results) == 0 {
		return nil
	}
	cloned := make([]SessionHistoryToolResultPayload, len(results))
	for index, result := range results {
		cloned[index] = result
		cloned[index].StructuredResult = cloneStructuredResult(result.StructuredResult)
		cloned[index].OutputBlob = cloneToolResultBlobRef(result.OutputBlob)
		cloned[index].ErrorBlob = cloneToolResultBlobRef(result.ErrorBlob)
	}
	return cloned
}

func filterPendingRequestOrder(order []string, requestID string) []string {
	if len(order) == 0 {
		return nil
	}
	out := order[:0]
	for _, id := range order {
		if id != requestID {
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func snapshotOmitsToolCall(call *ToolCallState) bool {
	if call == nil {
		return true
	}
	return !call.Declared && call.Execution == nil && !call.Executing && !call.Completed
}

func indexToolCalls(state SessionState) map[string]*ToolCallState {
	toolCalls := make(map[string]*ToolCallState)
	for _, turn := range state.Turns {
		if turn == nil {
			continue
		}
		for callID, call := range turn.ToolCalls {
			if call == nil || callID == "" {
				continue
			}
			toolCalls[callID] = call
		}
	}
	return toolCalls
}

func SnapshotRetainsToolBody(call *ToolCallState) bool {
	if call == nil {
		return false
	}
	switch call.ToolName {
	case "write", "mkdir":
		return true
	default:
		return false
	}
}

func cloneWriteMutation(mutation *WriteMutation) *WriteMutation {
	if mutation == nil {
		return nil
	}
	copyMutation := *mutation
	copyMutation.BeforeBlob = cloneToolResultBlobRef(mutation.BeforeBlob)
	copyMutation.DiffPreview = cloneTextDiffPreview(mutation.DiffPreview)
	return &copyMutation
}

func cloneWriteMutations(mutations []WriteMutation) []WriteMutation {
	if len(mutations) == 0 {
		return nil
	}
	out := make([]WriteMutation, len(mutations))
	for idx := range mutations {
		mutation := mutations[idx]
		mutation.BeforeBlob = cloneToolResultBlobRef(mutation.BeforeBlob)
		mutation.DiffPreview = cloneTextDiffPreview(mutation.DiffPreview)
		out[idx] = mutation
	}
	return out
}

func cloneTextDiffPreview(preview *textdiff.Preview) *textdiff.Preview {
	if preview == nil {
		return nil
	}
	copyPreview := *preview
	if len(preview.Ops) > 0 {
		copyPreview.Ops = append([]textdiff.PreviewOp(nil), preview.Ops...)
	}
	return &copyPreview
}

func cloneTurnRetryState(retry *TurnRetryState) *TurnRetryState {
	if retry == nil {
		return nil
	}
	copyRetry := *retry
	return &copyRetry
}
