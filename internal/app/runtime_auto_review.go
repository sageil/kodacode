package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

const autoReviewUserText = "[auto review] Review completed tasks without durable review outcomes and record task review results."
const autoReviewAssistantPrefix = "Review:\n"

func (r *Runtime) maybeRunAutoReview(
	ctx context.Context,
	workspaceRoot, completedAgentID string,
	result RunSessionResult,
	reviewMode WorkflowReviewMode,
) (RunSessionResult, bool, error) {
	if r == nil || strings.TrimSpace(completedAgentID) != "engineer" {
		return RunSessionResult{}, false, nil
	}
	if result.Status != TurnRunStatusCompleted || strings.TrimSpace(result.SessionID) == "" {
		return RunSessionResult{}, false, nil
	}
	if strings.TrimSpace(string(reviewMode)) == "" {
		reviewMode = r.Config.Workflow.ReviewMode
	}
	if strings.TrimSpace(string(reviewMode)) != string(WorkflowReviewAuto) {
		return RunSessionResult{}, false, nil
	}

	state, err := r.Sessions.Snapshot(ctx, result.SessionID)
	if err != nil {
		return RunSessionResult{}, false, err
	}
	if !shouldAutoReviewTasks(state) {
		return RunSessionResult{}, false, nil
	}

	reviewer, err := r.resolveTurnAgent(workspaceRoot, "reviewer")
	if err != nil {
		return RunSessionResult{}, false, err
	}
	modelRoute, err := r.resolveAutoReviewModelRoute(reviewer, state)
	if err != nil {
		return RunSessionResult{}, false, err
	}

	reviewResult, err := r.runExistingSessionTurn(ctx, runExistingTurnInput{
		SessionID:            result.SessionID,
		TurnID:               newRuntimeID("turn"),
		UserText:             autoReviewUserText,
		AgentID:              reviewer.ID,
		ModelRouteOverride:   modelRoute,
		PreserveSessionModel: true,
	})
	if err != nil {
		return RunSessionResult{}, false, err
	}
	reviewResult.AssistantText = combineAutoReviewAssistantText(result.AssistantText, reviewResult.AssistantText)
	return reviewResult, true, nil
}

func (r *Runtime) resolveAutoReviewModelRoute(reviewer agent.Definition, state events.SessionState) (provider.ModelRoute, error) {
	current := r.Config.ModelRoute
	if sessionRoute, ok := configuredSessionModelRoute(state); ok {
		current = sessionRoute
	}
	return r.resolveReviewerModelRoute(reviewer, current)
}

func shouldAutoReviewTasks(state events.SessionState) bool {
	if len(state.TaskOrder) == 0 {
		return false
	}
	latestCompletedUnreviewed := int64(0)
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[taskID]
		if task == nil {
			continue
		}
		if strings.TrimSpace(task.Status) == events.TaskStatusCompleted &&
			strings.TrimSpace(task.ReviewStatus) == "" &&
			task.CompletedAtSeq > latestCompletedUnreviewed {
			latestCompletedUnreviewed = task.CompletedAtSeq
		}
	}
	if latestCompletedUnreviewed == 0 {
		return false
	}
	return latestCompletedUnreviewed > latestAutoReviewAttemptSequence(state)
}

func latestAutoReviewAttemptSequence(state events.SessionState) int64 {
	latest := int64(0)
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil || strings.TrimSpace(turn.UserText) != autoReviewUserText {
			continue
		}
		sequence := max(turn.LastUpdatedAtSeq, turn.CompletedAtSeq)
		if sequence > latest {
			latest = sequence
		}
	}
	return latest
}

func combineAutoReviewAssistantText(primary, review string) string {
	primary = strings.TrimSpace(primary)
	review = formatAutoReviewAssistantText(review)
	switch {
	case primary == "":
		return review
	case review == "":
		return primary
	default:
		return primary + "\n\n" + review
	}
}

func formatAutoReviewAssistantText(review string) string {
	review = strings.TrimSpace(review)
	switch {
	case review == "":
		return ""
	case strings.HasPrefix(review, autoReviewAssistantPrefix):
		return review
	default:
		return autoReviewAssistantPrefix + review
	}
}
