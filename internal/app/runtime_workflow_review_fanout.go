package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

func (r *Runtime) startWorkflowReviewFanoutPhaseTurn(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext) (RunSessionResult, error) {
	if strings.TrimSpace(input.UserText) != "" || len(input.ResolvedAttachments) > 0 {
		if err := r.Runner.appendUserMessage(ctx, input.SessionID, input.TurnID, input.UserText, cloneProviderAttachments(input.ResolvedAttachments)); err != nil {
			return RunSessionResult{}, err
		}
	}
	phaseID := strings.TrimSpace(workflowPhase.Phase.ID)
	workflowBudget := workflowTurnBudgetFromDefinition(workflowPhase.WorkflowID, workflowPhase.Definition)
	workflowBudget.SessionID = input.SessionID
	lines := []string{"Workflow review fan-out:"}
	for _, pass := range workflowPhase.Phase.ReviewPasses {
		passID := strings.TrimSpace(pass.ID)
		if passID == "" {
			continue
		}
		if workflowHasReviewPassEvidence(ctx, r, input.SessionID, phaseID, passID) {
			continue
		}
		result, err := r.DelegateSessionTurn(ctx, DelegateSessionTurnInput{
			ParentSessionID:    input.SessionID,
			ParentTurnID:       input.TurnID,
			ParentAgentID:      reviewerAgentID,
			ChildAgentID:       reviewerAgentID,
			Task:               workflowReviewFanoutTask(workflowPhase.WorkflowID, pass),
			ContextSummary:     workflowReviewFanoutContextSummary(ctx, r, input.SessionID, workflowPhase.WorkflowID, phaseID),
			ModelRouteOverride: input.ModelRouteOverride,
			WorkflowBudget:     workflowBudget,
		})
		if err != nil {
			return RunSessionResult{}, err
		}
		if result.ChildTurn.Status != TurnRunStatusCompleted {
			if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, input.TurnID, "Workflow review fan-out is waiting on delegated reviewer pass `"+passID+"`."); err != nil {
				return RunSessionResult{}, err
			}
			return r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{Status: result.ChildTurn.Status, PendingRequestID: result.ChildTurn.PendingRequestID})
		}
		summary, err := r.recordWorkflowReviewFanoutEvidence(ctx, input.SessionID, input.TurnID, workflowPhase.WorkflowID, phaseID, passID, result.HandoffID)
		if err != nil {
			return RunSessionResult{}, err
		}
		lines = append(lines, "- "+passID+": "+summary)
	}
	if len(lines) == 1 {
		lines = append(lines, "- no new review passes needed")
	}
	if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, input.TurnID, strings.Join(lines, "\n")); err != nil {
		return RunSessionResult{}, err
	}
	if err := r.Runner.appendTurnDone(ctx, input.SessionID, input.TurnID); err != nil {
		return RunSessionResult{}, err
	}
	result, err := r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{Status: TurnRunStatusCompleted})
	if err != nil {
		return RunSessionResult{}, err
	}
	revised, err := r.maybeReviseWorkflowAfterReviewFailure(ctx, input.SessionID, input.TurnID)
	if err != nil {
		return RunSessionResult{}, err
	}
	if revised {
		return result, nil
	}
	if err := r.AdvanceWorkflow(ctx, AdvanceWorkflowInput{
		SessionID: input.SessionID,
		TurnID:    input.TurnID,
	}); err != nil {
		if errors.Is(err, ErrWorkflowEvidenceMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	_, definition, _, err := r.activeWorkflowState(ctx, input.SessionID)
	if err != nil {
		if errors.Is(err, ErrWorkflowStateMissing) {
			return result, nil
		}
		return RunSessionResult{}, err
	}
	return r.completeFinalWorkflowPhaseIfReached(ctx, input.SessionID, input.TurnID, definition, result)
}

func workflowReviewFanoutTask(workflowID string, pass workflowpkg.ReviewPass) string {
	passID := strings.TrimSpace(pass.ID)
	description := strings.TrimSpace(pass.Description)
	parts := []string{"Workflow review pass `" + passID + "`."}
	if workflowID = strings.TrimSpace(workflowID); workflowID != "" {
		parts = append(parts, "Workflow: "+workflowID+".")
	}
	if description != "" {
		parts = append(parts, "Focus: "+description)
	}
	parts = append(parts, "Return exactly one structured review JSON object.")
	return strings.Join(parts, "\n")
}

func workflowReviewFanoutContextSummary(ctx context.Context, r *Runtime, sessionID, workflowID, phaseID string) string {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil || state.Workflow == nil {
		return "Review the current workspace state for workflow " + strings.TrimSpace(workflowID) + "."
	}
	parts := []string{"Review workflow `" + strings.TrimSpace(workflowID) + "` phase `" + strings.TrimSpace(phaseID) + "`."}
	for _, evidenceID := range state.Workflow.EvidenceOrder {
		evidence := state.Workflow.Evidence[evidenceID]
		if evidence == nil {
			continue
		}
		switch evidence.Type {
		case events.WorkflowEvidenceTypeGitDiff, events.WorkflowEvidenceTypeVerificationResult, events.WorkflowEvidenceTypePhaseOutput:
		default:
			continue
		}
		summary := strings.TrimSpace(evidence.Summary)
		if summary == "" {
			continue
		}
		parts = append(parts, strings.TrimSpace(evidence.PhaseID)+" "+strings.TrimSpace(evidence.Type)+": "+summary)
	}
	return strings.Join(parts, "\n")
}

func workflowHasReviewPassEvidence(ctx context.Context, r *Runtime, sessionID, phaseID, passID string) bool {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil || state.Workflow == nil {
		return false
	}
	for _, evidenceID := range state.Workflow.EvidenceOrder {
		evidence := state.Workflow.Evidence[evidenceID]
		if evidence == nil || strings.TrimSpace(evidence.PhaseID) != strings.TrimSpace(phaseID) {
			continue
		}
		if evidence.Type != events.WorkflowEvidenceTypeReviewOutcome {
			continue
		}
		if strings.TrimSpace(evidence.Fields["review_pass"]) == strings.TrimSpace(passID) {
			return true
		}
	}
	return false
}

func (r *Runtime) recordWorkflowReviewFanoutEvidence(ctx context.Context, sessionID, turnID, workflowID, phaseID, passID, handoffID string) (string, error) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}
	review := workflowReviewForHandoff(state, handoffID)
	if review == nil {
		return "", fmt.Errorf("workflow review fan-out result missing review for handoff %s", strings.TrimSpace(handoffID))
	}
	successful := review.OverallCorrectness == events.ReviewOverallCorrectnessCorrect
	status := events.TaskReviewStatusPass
	if !successful {
		status = events.TaskReviewStatusFail
	}
	summary := strings.TrimSpace(review.OverallSummary)
	if summary == "" {
		summary = "review recorded"
	}
	if err := r.RecordWorkflowEvidence(ctx, RecordWorkflowEvidenceInput{
		SessionID:  sessionID,
		TurnID:     turnID,
		PhaseID:    phaseID,
		Type:       events.WorkflowEvidenceTypeReviewOutcome,
		ReviewID:   strings.TrimSpace(review.ReviewID),
		Successful: &successful,
		Summary:    summary,
		Fields: map[string]string{
			"review_pass":   strings.TrimSpace(passID),
			"review_status": status,
			"source":        "fanout",
		},
	}); err != nil {
		return "", err
	}
	return status + " - " + summary, nil
}

func workflowReviewForHandoff(state events.SessionState, handoffID string) *events.ReviewState {
	handoffID = strings.TrimSpace(handoffID)
	for _, reviewID := range state.ReviewOrder {
		review := state.Reviews[reviewID]
		if review == nil {
			continue
		}
		if strings.TrimSpace(review.SourceHandoffID) == handoffID || strings.TrimSpace(review.ReviewID) == handoffID {
			return review
		}
	}
	return nil
}
