package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func (e *ToolExecutor) toolWorkflowPhaseOutputManager(ctx context.Context, state events.SessionState, input ExecuteToolInput) tool.WorkflowPhaseOutputManager {
	if state.Workflow == nil || state.Workflow.Status != events.WorkflowStatusActive {
		return nil
	}
	return sessionWorkflowPhaseOutputManager{
		ctx:      ctx,
		sessions: e.sessions,
		input:    input,
		state:    state,
	}
}

type sessionWorkflowPhaseOutputManager struct {
	ctx      context.Context
	sessions *SessionService
	input    ExecuteToolInput
	state    events.SessionState
}

func (m sessionWorkflowPhaseOutputManager) RecordWorkflowPhaseOutput(request tool.WorkflowPhaseOutputRequest) (tool.WorkflowPhaseOutputRecord, error) {
	workflow := m.state.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return tool.WorkflowPhaseOutputRecord{}, ErrWorkflowStateMissing
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	fields := make(map[string]string, len(request.Fields))
	keys := make([]string, 0, len(request.Fields))
	for key, value := range request.Fields {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		fields[key] = value
		keys = append(keys, key)
	}
	if len(fields) == 0 {
		return tool.WorkflowPhaseOutputRecord{}, tool.ErrWorkflowPhaseOutputFieldsRequired
	}
	slices.Sort(keys)
	if _, err := m.sessions.append(m.ctx, events.Draft{
		SessionID: m.input.SessionID,
		TurnID:    workflowEventTurnID(m.input.TurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload: events.WorkflowEvidenceRecordedPayload{
			EvidenceID: newRuntimeID("workflow-evidence"),
			WorkflowID: strings.TrimSpace(workflow.WorkflowID),
			PhaseID:    phaseID,
			Type:       events.WorkflowEvidenceTypePhaseOutput,
			ToolCallID: strings.TrimSpace(m.input.ToolCallID),
			Summary:    workflowPhaseOutputToolSummary(keys),
			Fields:     cloneEvidenceFields(fields),
		},
	}); err != nil {
		return tool.WorkflowPhaseOutputRecord{}, err
	}
	if err := m.recordVerificationEvidenceFromPhaseOutput(workflow, phaseID, fields); err != nil {
		return tool.WorkflowPhaseOutputRecord{}, err
	}
	return tool.WorkflowPhaseOutputRecord{
		RecordedKeys: keys,
		Message:      fmt.Sprintf("recorded workflow phase output for %s: %s", phaseID, strings.Join(keys, ", ")),
	}, nil
}

func (m sessionWorkflowPhaseOutputManager) recordVerificationEvidenceFromPhaseOutput(workflow *events.WorkflowState, phaseID string, fields map[string]string) error {
	command := strings.TrimSpace(fields["commands_run"])
	result := strings.TrimSpace(fields["result"])
	if workflow == nil || command == "" || result == "" {
		return nil
	}
	successful, ok := workflowPhaseOutputVerificationSuccessful(result)
	if blockers := workflowPhaseOutputVerificationBlockers(fields); len(blockers) > 0 {
		successful = false
		ok = true
	}
	summary := workflowPhaseOutputVerificationSummary(fields)
	payload := events.WorkflowEvidenceRecordedPayload{
		EvidenceID: newRuntimeID("workflow-evidence"),
		WorkflowID: strings.TrimSpace(workflow.WorkflowID),
		PhaseID:    phaseID,
		Type:       events.WorkflowEvidenceTypeVerificationResult,
		ToolCallID: strings.TrimSpace(m.input.ToolCallID),
		Command:    command,
		Summary:    summary,
		Fields:     cloneEvidenceFields(fields),
	}
	if ok {
		payload.Successful = &successful
	}
	if _, err := m.sessions.append(m.ctx, events.Draft{
		SessionID: m.input.SessionID,
		TurnID:    workflowEventTurnID(m.input.TurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload:   payload,
	}); err != nil {
		return err
	}
	return nil
}

func workflowPhaseOutputVerificationSuccessful(result string) (bool, bool) {
	normalized := strings.ToLower(strings.TrimSpace(result))
	if normalized == "" {
		return false, false
	}
	for _, marker := range []string{"fail", "failed", "failure"} {
		if strings.Contains(normalized, marker) && !strings.Contains(normalized, "no fail") {
			return false, true
		}
	}
	for _, marker := range []string{"error", "errored", "broken"} {
		if strings.Contains(normalized, marker) && !strings.Contains(normalized, "no error") {
			return false, true
		}
	}
	for _, marker := range []string{"pass", "passed", "success", "successful", "clean", "green"} {
		if strings.Contains(normalized, marker) {
			return true, true
		}
	}
	return false, false
}

func workflowPhaseOutputVerificationSummary(fields map[string]string) string {
	command := strings.TrimSpace(fields["commands_run"])
	result := strings.TrimSpace(fields["result"])
	failures := strings.TrimSpace(fields["failures"])
	unverified := strings.TrimSpace(fields["unverified_criteria"])
	deferred := strings.TrimSpace(fields["deferred_items"])
	parts := []string{}
	if result != "" {
		parts = append(parts, result)
	}
	if command != "" {
		parts = append(parts, command)
	}
	if failures != "" && !strings.EqualFold(failures, "none") && !strings.EqualFold(failures, "none identified") {
		parts = append(parts, "failures: "+failures)
	}
	if workflowPhaseOutputFieldHasBlockingValue(unverified) {
		parts = append(parts, "unverified criteria: "+unverified)
	}
	if workflowPhaseOutputFieldHasBlockingValue(deferred) {
		parts = append(parts, "deferred items: "+deferred)
	}
	if len(parts) == 0 {
		return "verification result recorded"
	}
	return strings.Join(parts, " - ")
}

func workflowPhaseOutputVerificationBlockers(fields map[string]string) []string {
	var blockers []string
	for _, key := range []string{"failures", "unverified_criteria", "deferred_items"} {
		value := strings.TrimSpace(fields[key])
		if workflowPhaseOutputFieldHasBlockingValue(value) {
			blockers = append(blockers, key)
		}
	}
	return blockers
}

func workflowPhaseOutputFieldHasBlockingValue(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		return false
	}
	normalized = strings.Trim(normalized, " .")
	switch normalized {
	case "none", "none identified", "none found", "n/a", "na", "not applicable", "[]":
		return false
	default:
		return true
	}
}

func workflowPhaseOutputToolSummary(keys []string) string {
	if len(keys) == 0 {
		return "workflow phase output recorded"
	}
	return "workflow phase output recorded: " + strings.Join(keys, ", ")
}
