package events

import (
	"bytes"
	"encoding/json"
)

func cloneExecutionState(state *ExecutionState) *ExecutionState {
	if state == nil {
		return nil
	}
	return &ExecutionState{
		ExecutionID:      state.ExecutionID,
		ToolCallID:       state.ToolCallID,
		ToolName:         state.ToolName,
		Kind:             state.Kind,
		Intent:           state.Intent,
		Effect:           state.Effect,
		Command:          append([]string(nil), state.Command...),
		CommandPreview:   state.CommandPreview,
		WorkingDirectory: state.WorkingDirectory,
		TimeoutMS:        state.TimeoutMS,
		OutputLimit:      state.OutputLimit,
		Status:           state.Status,
		Input:            state.Input,
		Output:           state.Output,
		Error:            state.Error,
		OutputBlob:       cloneToolResultBlobRef(state.OutputBlob),
		ErrorBlob:        cloneToolResultBlobRef(state.ErrorBlob),
		OutputTruncated:  state.OutputTruncated,
		ErrorTruncated:   state.ErrorTruncated,
		ExitCode:         cloneExecutionExitCode(state.ExitCode),
		DurationMS:       state.DurationMS,
		CommandActions:   append([]string(nil), state.CommandActions...),
		Executing:        state.Executing,
		Completed:        state.Completed,
		Runtime:          cloneToolExecRuntimeState(state.Runtime),
		Background:       cloneExecutionBackgroundState(state.Background),
	}
}

func cloneExecutionBackgroundState(state *ExecutionBackgroundState) *ExecutionBackgroundState {
	if state == nil {
		return nil
	}
	return &ExecutionBackgroundState{
		PID:             state.PID,
		ProcessIdentity: state.ProcessIdentity,
		SupervisorID:    state.SupervisorID,
		Status:          state.Status,
		LogRef:          state.LogRef,
		ReadyPatterns:   append([]string(nil), state.ReadyPatterns...),
		Started:         state.Started,
		StartedAtSeq:    state.StartedAtSeq,
		Ready:           state.Ready,
		ReadyAtSeq:      state.ReadyAtSeq,
		ReadyMessage:    state.ReadyMessage,
		Port:            state.Port,
		OutputTail:      state.OutputTail,
		OutputBytes:     state.OutputBytes,
		Exited:          state.Exited,
		ExitedAtSeq:     state.ExitedAtSeq,
		ExitCode:        cloneExecutionExitCode(state.ExitCode),
		Error:           state.Error,
	}
}

func cloneExecutionApprovalState(state *ExecutionApprovalState) *ExecutionApprovalState {
	if state == nil {
		return nil
	}
	return &ExecutionApprovalState{
		RequestID:             state.RequestID,
		ExecutionID:           state.ExecutionID,
		TurnID:                state.TurnID,
		ToolCallID:            state.ToolCallID,
		ToolName:              state.ToolName,
		Command:               state.Command,
		WorkingDirectory:      state.WorkingDirectory,
		Reason:                state.Reason,
		PrefixRule:            append([]string(nil), state.PrefixRule...),
		SessionGrantPaths:     append([]string(nil), state.SessionGrantPaths...),
		NetworkTargets:        append([]string(nil), state.NetworkTargets...),
		AvailableDecisions:    append([]ExecutionApprovalDecision(nil), state.AvailableDecisions...),
		ProposedExecPolicy:    cloneExecutionPolicyAmendment(state.ProposedExecPolicy),
		ProposedNetworkPolicy: cloneExecutionNetworkPolicyAmendment(state.ProposedNetworkPolicy),
		RequestedAtSeq:        state.RequestedAtSeq,
	}
}

func cloneApprovedExecutionState(state *ApprovedExecutionState) *ApprovedExecutionState {
	if state == nil {
		return nil
	}
	return &ApprovedExecutionState{
		RequestID:            state.RequestID,
		ExecutionID:          state.ExecutionID,
		TurnID:               state.TurnID,
		ToolCallID:           state.ToolCallID,
		ToolName:             state.ToolName,
		Command:              state.Command,
		WorkingDirectory:     state.WorkingDirectory,
		Decision:             state.Decision,
		AppliedExecPolicy:    cloneExecutionPolicyAmendment(state.AppliedExecPolicy),
		AppliedNetworkPolicy: cloneExecutionNetworkPolicyAmendment(state.AppliedNetworkPolicy),
		ApprovedAtSeq:        state.ApprovedAtSeq,
	}
}

func cloneExecutionPolicyAmendment(state *ExecutionPolicyAmendment) *ExecutionPolicyAmendment {
	if state == nil {
		return nil
	}
	copyState := *state
	if state.AllowLoginShell != nil {
		value := *state.AllowLoginShell
		copyState.AllowLoginShell = &value
	}
	return &copyState
}

func cloneExecutionNetworkPolicyAmendment(state *ExecutionNetworkPolicyAmendment) *ExecutionNetworkPolicyAmendment {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func cloneExecutionExitCode(code *int) *int {
	if code == nil {
		return nil
	}
	copyCode := *code
	return &copyCode
}

func cloneToolResultBlobRef(ref *ToolResultBlobRef) *ToolResultBlobRef {
	if ref == nil {
		return nil
	}
	copyRef := *ref
	return &copyRef
}

func cloneMutationRanges(ranges []MutationRange) []MutationRange {
	if len(ranges) == 0 {
		return nil
	}
	out := make([]MutationRange, len(ranges))
	copy(out, ranges)
	return out
}

func cloneObservedResources(resources []ObservedResource) []ObservedResource {
	if len(resources) == 0 {
		return nil
	}
	out := make([]ObservedResource, len(resources))
	copy(out, resources)
	return out
}

func cloneToolErrorDetail(detail *ToolErrorDetail) *ToolErrorDetail {
	if detail == nil {
		return nil
	}
	out := *detail
	if len(detail.Fields) > 0 {
		out.Fields = make(map[string]string, len(detail.Fields))
		for key, value := range detail.Fields {
			out.Fields[key] = value
		}
	}
	return &out
}

func cloneStructuredResult(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	out := make(json.RawMessage, len(trimmed))
	copy(out, trimmed)
	return out
}
