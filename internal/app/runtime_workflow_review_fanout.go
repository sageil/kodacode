package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/sageil/kodacode/internal/events"
	workflowpkg "github.com/sageil/kodacode/internal/workflow"
)

const workflowReviewFanoutMaxConcurrency = 3

func (r *Runtime) startWorkflowReviewFanoutPhaseTurn(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext) (RunSessionResult, error) {
	if strings.TrimSpace(input.UserText) != "" || len(input.ResolvedAttachments) > 0 {
		if err := r.Runner.appendUserMessage(ctx, input.SessionID, input.TurnID, input.UserText, cloneProviderAttachments(input.ResolvedAttachments)); err != nil {
			return RunSessionResult{}, err
		}
	}
	phaseID := strings.TrimSpace(workflowPhase.Phase.ID)
	workflowBudget := workflowTurnBudgetFromDefinition(workflowPhase.WorkflowID, workflowPhase.Definition)
	workflowBudget.SessionID = input.SessionID
	passes := workflowReviewFanoutMissingPasses(ctx, r, input.SessionID, phaseID, workflowPhase.Phase.ReviewPasses)
	if err := r.appendWorkflowReviewFanoutStarted(ctx, input.SessionID, input.TurnID, passes); err != nil {
		return RunSessionResult{}, err
	}
	results, err := r.runWorkflowReviewFanoutPasses(ctx, input, workflowPhase, workflowBudget, passes)
	if err != nil {
		if textErr := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, input.TurnID, "Parallel workflow review failed: "+strings.TrimSpace(err.Error())); textErr != nil {
			return RunSessionResult{}, textErr
		}
		result, failErr := r.recordTurnFailure(ctx, input.SessionID, input.TurnID, input.UserText, cloneProviderAttachments(input.ResolvedAttachments), err)
		if failErr != nil {
			return RunSessionResult{}, failErr
		}
		if _, transitionErr := r.maybeApplyWorkflowTurnResultTransition(ctx, input.SessionID, input.TurnID, result); transitionErr != nil {
			return RunSessionResult{}, transitionErr
		}
		return result, nil
	}

	lines := []string{"Parallel workflow review:"}
	var pending *workflowReviewFanoutPassResult
	var missing []string
	for index := range results {
		result := results[index]
		passID := strings.TrimSpace(result.Pass.ID)
		if summary, ok := workflowReviewFanoutEvidenceSummary(ctx, r, input.SessionID, phaseID, passID); ok {
			lines = append(lines, "- "+passID+": "+summary)
			continue
		}
		if result.Child.ChildTurn.Status != TurnRunStatusCompleted {
			if result.Child.ChildTurn.Status == TurnRunStatusFailed {
				missing = append(missing, passID)
			} else if pending == nil {
				pending = &results[index]
			}
			lines = append(lines, "- "+passID+": "+workflowReviewFanoutIncompleteSummary(result))
			continue
		}
		missing = append(missing, passID)
		lines = append(lines, "- "+passID+": invalid structured result, retry needed")
	}
	if len(lines) == 1 {
		lines = append(lines, "- no new review passes needed")
	}
	if len(missing) > 0 {
		if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, input.TurnID, strings.Join(lines, "\n")); err != nil {
			return RunSessionResult{}, err
		}
		if err := r.blockWorkflowPhase(ctx, input.SessionID, input.TurnID, workflowPhase.WorkflowID, phaseID, "workflow review pass missing valid structured result: "+strings.Join(missing, ", ")); err != nil {
			return RunSessionResult{}, err
		}
		if err := r.Runner.appendTurnDone(ctx, input.SessionID, input.TurnID); err != nil {
			return RunSessionResult{}, err
		}
		return r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{Status: TurnRunStatusCompleted})
	}
	if pending != nil {
		if err := appendTextToParentTurn(ctx, r.Sessions, input.SessionID, input.TurnID, strings.Join(lines, "\n")); err != nil {
			return RunSessionResult{}, err
		}
		return r.loadSessionTurnResult(ctx, input.SessionID, input.TurnID, RunTurnResult{
			Status:           pending.Child.ChildTurn.Status,
			PendingRequestID: workflowReviewFanoutPendingRequestID(*pending),
		})
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
	if continued, ok, err := r.continueWorkflowIfRunnable(ctx, input.SessionID, input.TurnID, result); err != nil || ok {
		return continued, err
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

func (r *Runtime) appendWorkflowReviewFanoutStarted(ctx context.Context, sessionID, turnID string, passes []workflowpkg.ReviewPass) error {
	if len(passes) == 0 {
		return nil
	}
	passIDs := make([]string, 0, len(passes))
	for _, pass := range passes {
		if passID := strings.TrimSpace(pass.ID); passID != "" {
			passIDs = append(passIDs, passID)
		}
	}
	if len(passIDs) == 0 {
		return nil
	}
	return appendTextToParentTurn(ctx, r.Sessions, sessionID, turnID, "Parallel workflow review started: "+strings.Join(passIDs, ", "))
}

func workflowReviewFanoutIncompleteSummary(result workflowReviewFanoutPassResult) string {
	if result.Child.ChildTurn.Status == TurnRunStatusFailed {
		message := strings.TrimSpace(result.Child.ChildTurn.Error)
		if message == "" {
			message = "reviewer child turn failed"
		}
		if strings.Contains(message, ErrDelegatedReviewStructuredOutputInvalid.Error()) {
			return "invalid structured result, retry needed - " + message
		}
		return "failed, retry needed - " + message
	}
	if strings.TrimSpace(result.Child.ChildTurn.PendingRequestID) != "" {
		return "waiting on " + strings.TrimSpace(result.Child.ChildTurn.PendingRequestID)
	}
	if result.Err != nil {
		return "failed, retry needed - " + strings.TrimSpace(result.Err.Error())
	}
	return "waiting"
}

func workflowReviewFanoutPendingRequestID(result workflowReviewFanoutPassResult) string {
	if handoffID := strings.TrimSpace(result.Child.HandoffID); handoffID != "" {
		return handoffID
	}
	return strings.TrimSpace(result.Child.ChildTurn.PendingRequestID)
}

func workflowReviewFanoutEvidenceSummary(ctx context.Context, r *Runtime, sessionID, phaseID, passID string) (string, bool) {
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil || state.Workflow == nil {
		return "", false
	}
	for _, evidenceID := range state.Workflow.EvidenceOrder {
		evidence := state.Workflow.Evidence[evidenceID]
		if evidence == nil || strings.TrimSpace(evidence.PhaseID) != strings.TrimSpace(phaseID) {
			continue
		}
		if evidence.Type != events.WorkflowEvidenceTypeReviewOutcome {
			continue
		}
		if strings.TrimSpace(evidence.Fields["review_pass"]) != strings.TrimSpace(passID) {
			continue
		}
		status := strings.TrimSpace(evidence.Fields["review_status"])
		if status == "" {
			if evidence.Successful != nil && *evidence.Successful {
				status = events.TaskReviewStatusPass
			} else {
				status = events.TaskReviewStatusFail
			}
		}
		summary := strings.TrimSpace(evidence.Summary)
		if summary == "" {
			summary = "review recorded"
		}
		return status + " - " + summary, true
	}
	return "", false
}

type workflowReviewFanoutPassResult struct {
	Index int
	Pass  workflowpkg.ReviewPass
	Child DelegateSessionTurnResult
	Err   error
}

func workflowReviewFanoutMissingPasses(ctx context.Context, r *Runtime, sessionID, phaseID string, passes []workflowpkg.ReviewPass) []workflowpkg.ReviewPass {
	missing := make([]workflowpkg.ReviewPass, 0, len(passes))
	for _, pass := range passes {
		passID := strings.TrimSpace(pass.ID)
		if passID == "" {
			continue
		}
		if workflowHasReviewPassEvidence(ctx, r, sessionID, phaseID, passID) {
			continue
		}
		missing = append(missing, pass)
	}
	return missing
}

func (r *Runtime) runWorkflowReviewFanoutPasses(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext, workflowBudget workflowTurnBudget, passes []workflowpkg.ReviewPass) ([]workflowReviewFanoutPassResult, error) {
	results := make([]workflowReviewFanoutPassResult, len(passes))
	if len(passes) == 0 {
		return results, nil
	}
	contextSummary := workflowReviewFanoutContextSummary(ctx, r, input.SessionID, workflowPhase.WorkflowID, strings.TrimSpace(workflowPhase.Phase.ID))
	concurrency := workflowReviewFanoutConcurrency(workflowBudget, len(passes))
	if concurrency <= 1 {
		for index, pass := range passes {
			result := r.runWorkflowReviewFanoutPass(ctx, input, workflowPhase, workflowBudget, contextSummary, index, pass)
			results[index] = result
			if result.Err != nil {
				return results, result.Err
			}
		}
		return results, nil
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	queue := make(chan int)
	errs := make(chan error, 1)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range queue {
				result := r.runWorkflowReviewFanoutPass(ctx, input, workflowPhase, workflowBudget, contextSummary, index, passes[index])
				results[index] = result
				if result.Err != nil {
					select {
					case errs <- result.Err:
					default:
					}
					cancel()
					return
				}
			}
		}()
	}
stopped:
	for index := range passes {
		select {
		case <-ctx.Done():
			break stopped
		case queue <- index:
		}
	}
	close(queue)
	wg.Wait()
	select {
	case err := <-errs:
		return results, err
	default:
	}
	if err := ctx.Err(); err != nil {
		return results, err
	}
	for _, result := range results {
		if result.Err != nil {
			return results, result.Err
		}
	}
	return results, nil
}

func workflowReviewFanoutConcurrency(budget workflowTurnBudget, passCount int) int {
	if passCount <= 1 {
		return 1
	}
	if budget.enabled() {
		return 1
	}
	return min(passCount, workflowReviewFanoutMaxConcurrency)
}

func (r *Runtime) runWorkflowReviewFanoutPass(ctx context.Context, input runExistingTurnInput, workflowPhase workflowPhaseTurnContext, workflowBudget workflowTurnBudget, contextSummary string, index int, pass workflowpkg.ReviewPass) workflowReviewFanoutPassResult {
	result := workflowReviewFanoutPassResult{
		Index: index,
		Pass:  pass,
	}
	if err := r.Runner.enforceWorkflowBudgetLimit(ctx, input.SessionID, workflowBudget); err != nil {
		result.Err = err
		return result
	}
	reviewAgentID := workflowPhaseAgentID(reviewerAgentID, workflowPhase.Phase)
	child, err := r.DelegateSessionTurn(ctx, DelegateSessionTurnInput{
		ParentSessionID:    input.SessionID,
		ParentTurnID:       input.TurnID,
		ParentAgentID:      reviewAgentID,
		ChildAgentID:       reviewAgentID,
		Task:               workflowReviewFanoutTask(workflowPhase.WorkflowID, pass),
		ContextSummary:     contextSummary,
		ModelRouteOverride: input.ModelRouteOverride,
		WorkflowBudget:     workflowBudget,
	})
	result.Child = child
	result.Err = err
	if result.Err != nil {
		return result
	}
	_ = r.appendWorkflowReviewFanoutPassFinished(ctx, input.SessionID, input.TurnID, pass, result)
	result.Err = r.Runner.enforceWorkflowBudgetLimit(ctx, input.SessionID, workflowBudget)
	return result
}

func (r *Runtime) appendWorkflowReviewFanoutPassFinished(ctx context.Context, sessionID, turnID string, pass workflowpkg.ReviewPass, result workflowReviewFanoutPassResult) error {
	passID := strings.TrimSpace(pass.ID)
	if passID == "" {
		passID = fmt.Sprintf("pass %d", result.Index+1)
	}
	status := "completed"
	switch result.Child.ChildTurn.Status {
	case TurnRunStatusFailed:
		status = "failed"
	case TurnRunStatusPending:
		status = "waiting"
	}
	return appendTextToParentTurn(ctx, r.Sessions, sessionID, turnID, "Parallel workflow review pass "+passID+": "+status)
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
	parts = append(parts,
		"Read/search as needed.",
		"Then call `workflow_review_result` exactly once with this `review_pass`, `overall_correctness`, `overall_summary`, and `findings`.",
		"Do not return JSON in assistant text as the review result.",
	)
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
