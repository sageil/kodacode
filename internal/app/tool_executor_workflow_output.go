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
	return tool.WorkflowPhaseOutputRecord{
		RecordedKeys: keys,
		Message:      fmt.Sprintf("recorded workflow phase output for %s: %s", phaseID, strings.Join(keys, ", ")),
	}, nil
}

func workflowPhaseOutputToolSummary(keys []string) string {
	if len(keys) == 0 {
		return "workflow phase output recorded"
	}
	return "workflow phase output recorded: " + strings.Join(keys, ", ")
}
