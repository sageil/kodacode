package app

import (
	"context"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

var (
	ErrWorkflowReviewResultTargetMissing = errors.New("workflow review result target is missing")
	ErrWorkflowReviewResultDuplicate     = errors.New("workflow review result already recorded for pass")
)

func (e *ToolExecutor) toolWorkflowReviewResultManager(ctx context.Context, state events.SessionState, input ExecuteToolInput) tool.WorkflowReviewResultManager {
	return sessionWorkflowReviewResultManager{
		ctx:      ctx,
		sessions: e.sessions,
		input:    input,
		state:    state,
	}
}

type sessionWorkflowReviewResultManager struct {
	ctx      context.Context
	sessions *SessionService
	input    ExecuteToolInput
	state    events.SessionState
}

func (m sessionWorkflowReviewResultManager) RecordWorkflowReviewResult(request tool.WorkflowReviewResultRequest) (tool.WorkflowReviewResultRecord, error) {
	target, err := m.target()
	if err != nil {
		return tool.WorkflowReviewResultRecord{}, err
	}
	workflow := target.parentState.Workflow
	if workflow == nil || workflow.Status != events.WorkflowStatusActive {
		return tool.WorkflowReviewResultRecord{}, ErrWorkflowStateMissing
	}
	phaseID := strings.TrimSpace(workflow.CurrentPhaseID)
	passID := strings.TrimSpace(request.ReviewPass)
	if workflowHasReviewPassEvidenceInState(target.parentState, phaseID, passID) {
		return tool.WorkflowReviewResultRecord{}, ErrWorkflowReviewResultDuplicate
	}

	successful := strings.TrimSpace(request.OverallCorrectness) == tool.WorkflowReviewOverallCorrectnessCorrect
	status := events.TaskReviewStatusPass
	if !successful {
		status = events.TaskReviewStatusFail
	}
	reviewID := strings.TrimSpace(target.reviewID)
	if reviewID == "" {
		reviewID = newRuntimeID("review")
	}
	reviewPayload := events.ReviewRecordedPayload{
		ReviewID:           reviewID,
		SourceHandoffID:    strings.TrimSpace(target.sourceHandoffID),
		Title:              "Workflow review: " + passID,
		Findings:           workflowReviewFindingsToEvents(request.Findings),
		OverallCorrectness: strings.TrimSpace(request.OverallCorrectness),
		OverallSummary:     strings.TrimSpace(request.OverallSummary),
	}
	if err := events.ValidateReviewRecordedPayload(reviewPayload); err != nil {
		return tool.WorkflowReviewResultRecord{}, err
	}

	drafts := []events.Draft{
		{
			SessionID: target.parentSessionID,
			TurnID:    target.parentTurnID,
			Type:      events.TypeReviewRecorded,
			Payload:   reviewPayload,
		},
	}
	drafts = append(drafts, events.Draft{
		SessionID: target.parentSessionID,
		TurnID:    workflowEventTurnID(target.parentTurnID),
		Type:      events.TypeWorkflowEvidenceRecorded,
		Payload: events.WorkflowEvidenceRecordedPayload{
			EvidenceID: newRuntimeID("workflow-evidence"),
			WorkflowID: strings.TrimSpace(workflow.WorkflowID),
			PhaseID:    phaseID,
			Type:       events.WorkflowEvidenceTypeReviewOutcome,
			ToolCallID: strings.TrimSpace(m.input.ToolCallID),
			ReviewID:   reviewID,
			Successful: &successful,
			Summary:    strings.TrimSpace(request.OverallSummary),
			Fields:     workflowReviewResultEvidenceFields(passID, status, target.source),
		},
	})
	for _, draft := range drafts {
		if _, err := m.sessions.append(m.ctx, draft); err != nil {
			return tool.WorkflowReviewResultRecord{}, err
		}
	}
	return tool.WorkflowReviewResultRecord{
		ReviewID:   reviewID,
		ReviewPass: passID,
		Status:     status,
		Message:    "workflow review result recorded for " + passID,
	}, nil
}

type workflowReviewResultTarget struct {
	parentSessionID string
	parentTurnID    string
	reviewID        string
	sourceHandoffID string
	source          string
	parentState     events.SessionState
}

func (m sessionWorkflowReviewResultManager) target() (workflowReviewResultTarget, error) {
	if m.state.Workflow == nil {
		return workflowReviewResultTarget{}, ErrWorkflowReviewResultTargetMissing
	}
	return workflowReviewResultTarget{
		parentSessionID: strings.TrimSpace(m.input.SessionID),
		parentTurnID:    strings.TrimSpace(m.input.TurnID),
		source:          "workflow_review",
		parentState:     m.state,
	}, nil
}

func workflowReviewFindingsToEvents(findings []tool.WorkflowReviewFinding) []events.ReviewFindingPayload {
	if len(findings) == 0 {
		return nil
	}
	out := make([]events.ReviewFindingPayload, 0, len(findings))
	for _, finding := range findings {
		out = append(out, events.ReviewFindingPayload{
			Severity:    strings.TrimSpace(finding.Severity),
			Path:        strings.TrimSpace(finding.Path),
			Line:        finding.Line,
			Title:       strings.TrimSpace(finding.Title),
			Explanation: strings.TrimSpace(finding.Explanation),
		})
	}
	return out
}

func workflowReviewResultEvidenceFields(passID, status, source string) map[string]string {
	return map[string]string{
		"review_pass":   strings.TrimSpace(passID),
		"review_status": strings.TrimSpace(status),
		"source":        strings.TrimSpace(source),
	}
}

func workflowHasReviewPassEvidenceInState(state events.SessionState, phaseID, passID string) bool {
	return workflowHasEvidence(state.Workflow, phaseID, func(evidence *events.WorkflowEvidenceState) bool {
		if evidence.Type != events.WorkflowEvidenceTypeReviewOutcome {
			return false
		}
		return strings.TrimSpace(evidence.Fields["review_pass"]) == strings.TrimSpace(passID)
	})
}

func workflowHasReviewPassEvidence(ctx context.Context, r *Runtime, sessionID, phaseID, passID string) bool {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil || state.Workflow == nil {
		return false
	}
	return workflowHasReviewPassEvidenceInState(state, phaseID, passID)
}

func workflowReviewPassIDFromTask(task string) string {
	task = strings.TrimSpace(task)
	const prefix = "Workflow review pass `"
	if !strings.HasPrefix(task, prefix) {
		return ""
	}
	rest := strings.TrimPrefix(task, prefix)
	idx := strings.Index(rest, "`")
	if idx < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:idx])
}
