package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/textdiff"
	"github.com/sageil/kodacode/internal/tool"
	"github.com/sageil/kodacode/internal/workspace"
)

type toolExecutionEventAppender interface {
	AppendToolExecStart(ctx context.Context, input ExecuteToolInput) error
	AppendToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error
}

type durableToolExecutionEventAppender struct {
	executor *ToolExecutor
}

func (a durableToolExecutionEventAppender) AppendToolExecStart(ctx context.Context, input ExecuteToolInput) error {
	return a.executor.appendToolExecStart(ctx, input)
}

func (a durableToolExecutionEventAppender) AppendToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error {
	return a.executor.appendToolExecEnd(ctx, input, payload, execErr, output, errorText, runtime)
}

type bufferedToolExecutionEventKind string

const (
	bufferedToolExecutionEventStart bufferedToolExecutionEventKind = "start"
	bufferedToolExecutionEventEnd   bufferedToolExecutionEventKind = "end"
)

type bufferedToolExecutionEvent struct {
	kind      bufferedToolExecutionEventKind
	input     ExecuteToolInput
	payload   events.ToolExecEndPayload
	execErr   error
	output    string
	errorText string
	runtime   *tool.ExecutionRuntime
}

type bufferedToolExecutionEventAppender struct {
	events []bufferedToolExecutionEvent
}

func (a *bufferedToolExecutionEventAppender) AppendToolExecStart(_ context.Context, input ExecuteToolInput) error {
	a.events = append(a.events, bufferedToolExecutionEvent{
		kind:  bufferedToolExecutionEventStart,
		input: cloneExecuteToolInput(input),
	})
	return nil
}

func (a *bufferedToolExecutionEventAppender) AppendToolExecEnd(_ context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error {
	a.events = append(a.events, bufferedToolExecutionEvent{
		kind:      bufferedToolExecutionEventEnd,
		input:     cloneExecuteToolInput(input),
		payload:   cloneToolExecEndPayload(payload),
		execErr:   execErr,
		output:    output,
		errorText: errorText,
		runtime:   cloneToolExecutionRuntime(runtime),
	})
	return nil
}

func (a *bufferedToolExecutionEventAppender) Commit(ctx context.Context, executor *ToolExecutor) error {
	if a == nil {
		return nil
	}
	for _, event := range a.events {
		switch event.kind {
		case bufferedToolExecutionEventStart:
			if err := executor.appendToolExecStart(ctx, event.input); err != nil {
				return err
			}
		case bufferedToolExecutionEventEnd:
			if err := executor.appendToolExecEnd(ctx, event.input, event.payload, event.execErr, event.output, event.errorText, event.runtime); err != nil {
				return err
			}
		}
	}
	return nil
}

func cloneExecuteToolInput(input ExecuteToolInput) ExecuteToolInput {
	input.Arguments = append([]byte(nil), input.Arguments...)
	input.AllowedTools = append([]string(nil), input.AllowedTools...)
	input.TemporaryGrants = append([]workspace.Grant(nil), input.TemporaryGrants...)
	input.TemporaryNetworkTargets = append([]string(nil), input.TemporaryNetworkTargets...)
	return input
}

func cloneToolExecEndPayload(payload events.ToolExecEndPayload) events.ToolExecEndPayload {
	payload.ErrorDetail = cloneToolErrorDetail(payload.ErrorDetail)
	payload.StructuredResult = cloneStructuredResult(payload.StructuredResult)
	payload.MutationRanges = append([]events.MutationRange(nil), payload.MutationRanges...)
	if payload.WriteMutation != nil {
		mutation := *payload.WriteMutation
		if mutation.BeforeBlob != nil {
			blob := *mutation.BeforeBlob
			mutation.BeforeBlob = &blob
		}
		if mutation.DiffPreview != nil {
			preview := *mutation.DiffPreview
			preview.Ops = append([]textdiff.PreviewOp(nil), preview.Ops...)
			mutation.DiffPreview = &preview
		}
		payload.WriteMutation = &mutation
	}
	payload.WriteMutations = cloneWriteMutationPayloads(payload.WriteMutations)
	payload.ObservedResources = cloneObservedToolResources(payload.ObservedResources)
	if payload.OutputBlob != nil {
		blob := *payload.OutputBlob
		payload.OutputBlob = &blob
	}
	if payload.ErrorBlob != nil {
		blob := *payload.ErrorBlob
		payload.ErrorBlob = &blob
	}
	return payload
}

func cloneWriteMutationPayloads(mutations []events.WriteMutation) []events.WriteMutation {
	if len(mutations) == 0 {
		return nil
	}
	out := make([]events.WriteMutation, len(mutations))
	for idx, mutation := range mutations {
		if mutation.BeforeBlob != nil {
			blob := *mutation.BeforeBlob
			mutation.BeforeBlob = &blob
		}
		if mutation.DiffPreview != nil {
			preview := *mutation.DiffPreview
			preview.Ops = append([]textdiff.PreviewOp(nil), preview.Ops...)
			mutation.DiffPreview = &preview
		}
		out[idx] = mutation
	}
	return out
}

func cloneToolExecutionRuntime(runtime *tool.ExecutionRuntime) *tool.ExecutionRuntime {
	if runtime == nil {
		return nil
	}
	clone := *runtime
	return &clone
}

func (e *ToolExecutor) appendToolExecStart(ctx context.Context, input ExecuteToolInput) error {
	if _, err := e.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeToolExecStart,
		Payload: events.ToolExecStartPayload{
			CallID:   input.ToolCallID,
			ToolName: input.ToolName,
			ToolKind: string(inputToolKindOrDefault(input.ToolKind)),
			Input:    string(input.Arguments),
		},
	}); err != nil {
		return err
	}
	e.logger.Op("tool execution started",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
	)
	e.logger.Debug("tool execution input",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
		"arguments", string(input.Arguments),
	)
	return nil
}

func (e *ToolExecutor) appendToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload, execErr error, output, errorText string, runtime *tool.ExecutionRuntime) error {
	if _, err := e.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
		Type:      events.TypeToolExecEnd,
		Payload:   payload,
	}); err != nil {
		return err
	}
	if execErr != nil || strings.TrimSpace(errorText) != "" {
		e.logger.Error("tool execution completed with error", execErr,
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
			"tool_error_bytes", len(errorText),
		)
	} else {
		e.logger.Op("tool execution completed",
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
		)
	}
	e.logger.Debug("tool execution output",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"tool_call_id", input.ToolCallID,
		"tool_name", input.ToolName,
		"output", output,
		"error", errorText,
	)
	if runtime != nil {
		e.logger.Debug("tool execution runtime",
			"session_id", input.SessionID,
			"turn_id", input.TurnID,
			"tool_call_id", input.ToolCallID,
			"tool_name", input.ToolName,
			"backend", runtime.Backend,
		)
	}
	if err := e.appendWorkflowVerificationEvidenceFromToolExecEnd(ctx, input, payload); err != nil {
		return err
	}
	if err := e.appendWorkflowFileMutationEvidenceFromToolExecEnd(ctx, input, payload); err != nil {
		return err
	}
	if err := e.appendWorkflowGitDiffEvidenceFromToolExecEnd(ctx, input, payload); err != nil {
		return err
	}
	return nil
}

func (e *ToolExecutor) appendWorkflowVerificationEvidenceFromToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload) error {
	if e == nil || e.sessions == nil {
		return nil
	}
	switch strings.TrimSpace(payload.ToolName) {
	case tool.TestToolName, tool.BashToolName:
	default:
		return nil
	}
	state, err := e.sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return err
	}
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return nil
	}
	command := workflowVerificationCommand(state, payload.CallID)
	commands, err := e.workflowPhaseCommands(ctx, state, workflow.WorkflowID, workflow.CurrentPhaseID)
	if err != nil {
		return err
	}
	if len(commands) == 0 || !workflowVerificationCommandMatches(commands, payload.ToolName, command) {
		return nil
	}
	phase := workflow.Phases[workflow.CurrentPhaseID]
	if phase != nil {
		for _, evidenceID := range phase.EvidenceIDs {
			evidence := workflow.Evidence[evidenceID]
			if evidence != nil && strings.TrimSpace(evidence.ToolCallID) == strings.TrimSpace(payload.CallID) {
				return nil
			}
		}
	}
	successful := payload.Successful()
	summary := workflowVerificationSummary(payload)
	if _, err := e.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload: events.WorkflowEvidenceRecordedPayload{
			EvidenceID:  newRuntimeID("workflow-evidence"),
			WorkflowID:  workflow.WorkflowID,
			PhaseID:     workflow.CurrentPhaseID,
			Type:        events.WorkflowEvidenceTypeVerificationResult,
			ToolCallID:  strings.TrimSpace(payload.CallID),
			ExecutionID: strings.TrimSpace(payload.ExecutionID),
			Command:     command,
			ExitCode:    cloneInt(payload.ExitCode),
			Successful:  &successful,
			Summary:     summary,
			Fields: map[string]string{
				"verification_tool": strings.TrimSpace(payload.ToolName),
			},
		},
	}); err != nil {
		return err
	}
	if !successful {
		_, err := e.sessions.append(ctx, events.Draft{
			SessionID: input.SessionID,
			TurnID:    workflowEventTurnID(input.TurnID),
			Type:      events.TypeWorkflowPhaseBlocked,
			Payload: events.WorkflowPhaseBlockedPayload{
				WorkflowID: workflow.WorkflowID,
				PhaseID:    workflow.CurrentPhaseID,
				StopReason: summary,
			},
		})
		return err
	}
	return nil
}

func (e *ToolExecutor) appendWorkflowGitDiffEvidenceFromToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload) error {
	if e == nil || e.sessions == nil || strings.TrimSpace(payload.ToolName) != "git_diff" || !payload.Successful() {
		return nil
	}
	state, err := e.sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return err
	}
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return nil
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	phase := workflow.Phases[phaseID]
	if phase != nil {
		for _, evidenceID := range phase.EvidenceIDs {
			evidence := workflow.Evidence[evidenceID]
			if evidence != nil && strings.TrimSpace(evidence.ToolCallID) == strings.TrimSpace(payload.CallID) {
				return nil
			}
		}
	}
	summary := strings.TrimSpace(payload.Output)
	if summary == "" {
		summary = "git diff captured"
	}
	_, err = e.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload: events.WorkflowEvidenceRecordedPayload{
			EvidenceID:  newRuntimeID("workflow-evidence"),
			WorkflowID:  workflow.WorkflowID,
			PhaseID:     phaseID,
			Type:        events.WorkflowEvidenceTypeGitDiff,
			ToolCallID:  strings.TrimSpace(payload.CallID),
			ExecutionID: strings.TrimSpace(payload.ExecutionID),
			Summary:     truncateWorkflowEvidenceSummary(summary),
		},
	})
	return err
}

func (e *ToolExecutor) appendWorkflowFileMutationEvidenceFromToolExecEnd(ctx context.Context, input ExecuteToolInput, payload events.ToolExecEndPayload) error {
	if e == nil || e.sessions == nil || !payload.Successful() {
		return nil
	}
	paths := workflowFileMutationPaths(payload)
	if len(paths) == 0 {
		return nil
	}
	state, err := e.sessions.Snapshot(ctx, input.SessionID)
	if err != nil {
		return err
	}
	workflow := state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return nil
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	phase := workflow.Phases[phaseID]
	if phase != nil {
		for _, evidenceID := range phase.EvidenceIDs {
			evidence := workflow.Evidence[evidenceID]
			if evidence != nil && strings.TrimSpace(evidence.ToolCallID) == strings.TrimSpace(payload.CallID) && evidence.Type == events.WorkflowEvidenceTypeFileMutation {
				return nil
			}
		}
	}
	summary := "modified " + strings.Join(paths, ", ")
	_, err = e.sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload: events.WorkflowEvidenceRecordedPayload{
			EvidenceID:  newRuntimeID("workflow-evidence"),
			WorkflowID:  workflow.WorkflowID,
			PhaseID:     phaseID,
			Type:        events.WorkflowEvidenceTypeFileMutation,
			ToolCallID:  strings.TrimSpace(payload.CallID),
			ExecutionID: strings.TrimSpace(payload.ExecutionID),
			Summary:     truncateWorkflowEvidenceSummary(summary),
			Fields: map[string]string{
				"paths": strings.Join(paths, ","),
				"tool":  strings.TrimSpace(payload.ToolName),
			},
		},
	})
	return err
}

func workflowFileMutationPaths(payload events.ToolExecEndPayload) []string {
	var paths []string
	if payload.WriteMutation != nil && workflowWriteMutationHasChanges(*payload.WriteMutation) {
		paths = appendUniqueValues(paths, []string{strings.TrimSpace(payload.WriteMutation.Path)})
	}
	for _, mutation := range payload.WriteMutations {
		if workflowWriteMutationHasChanges(mutation) {
			paths = appendUniqueValues(paths, []string{strings.TrimSpace(mutation.Path)})
		}
	}
	return paths
}

func workflowWriteMutationHasChanges(mutation events.WriteMutation) bool {
	if strings.TrimSpace(mutation.Path) == "" || mutation.DiffPreview == nil {
		return false
	}
	return textdiff.HasChanges(*mutation.DiffPreview)
}

func (e *ToolExecutor) workflowPhaseCommands(ctx context.Context, state events.SessionState, workflowID, phaseID string) ([]workflowVerificationCommandSpec, error) {
	if e == nil || e.workflowPhaseCommandResolver == nil {
		return nil, nil
	}
	return e.workflowPhaseCommandResolver(ctx, state.WorkspaceRoot, workflowID, phaseID)
}

func workflowVerificationCommand(state events.SessionState, callID string) string {
	for _, turn := range state.Turns {
		if turn == nil {
			continue
		}
		call := turn.ToolCalls[strings.TrimSpace(callID)]
		if call == nil || call.Execution == nil {
			continue
		}
		if command := strings.TrimSpace(call.Execution.CommandPreview); command != "" {
			return command
		}
	}
	return ""
}

func workflowVerificationCommandMatches(commands []workflowVerificationCommandSpec, toolName, command string) bool {
	toolName = strings.TrimSpace(toolName)
	command = strings.TrimSpace(command)
	if toolName == "" || command == "" {
		return false
	}
	for _, declared := range commands {
		if strings.TrimSpace(declared.ToolName) == toolName && strings.TrimSpace(declared.Command) == command {
			return true
		}
	}
	return false
}

func containsTrimmed(values []string, needle string) bool {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, value := range values {
		if strings.TrimSpace(value) == needle {
			return true
		}
	}
	return false
}

func workflowVerificationSummary(payload events.ToolExecEndPayload) string {
	if payload.Successful() {
		if output := strings.TrimSpace(payload.Output); output != "" {
			return truncateWorkflowEvidenceSummary(output)
		}
		return "verification succeeded"
	}
	if err := strings.TrimSpace(payload.Error); err != "" {
		return truncateWorkflowEvidenceSummary(err)
	}
	if output := strings.TrimSpace(payload.Output); output != "" {
		return truncateWorkflowEvidenceSummary(output)
	}
	return "verification failed"
}

func truncateWorkflowEvidenceSummary(text string) string {
	const limit = 512
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if len(text) <= limit {
		return text
	}
	return text[:limit]
}
