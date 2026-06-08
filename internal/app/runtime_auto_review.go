package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/provider"
)

const autoReviewUserText = "[auto review] Review completed tasks without saved review outcomes and record task review results."
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
	if state.Workflow != nil && state.Workflow.Status != events.WorkflowStatusCompleted {
		return RunSessionResult{}, false, nil
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

	reviewResult, err := r.runDerivedSessionTurn(ctx, runExistingTurnInput{
		SessionID:            result.SessionID,
		TurnID:               newRuntimeID("turn"),
		UserText:             autoReviewUserText,
		AgentID:              reviewer.ID,
		AdditionalFragments:  autoReviewContextPromptFragments(state, result),
		ModelRouteOverride:   modelRoute,
		PreserveSessionModel: true,
	})
	if err != nil {
		return RunSessionResult{}, false, err
	}
	reviewResult.AssistantText = combineAutoReviewAssistantText(result.AssistantText, reviewResult.AssistantText)
	return reviewResult, true, nil
}

const autoReviewContextMaxChars = 4000

func autoReviewContextPromptFragments(state events.SessionState, result RunSessionResult) []prompt.Fragment {
	lines := []string{
		"Auto-review compact context.",
		"Session history is intentionally omitted for this derived review turn.",
		"Review the completed tasks below that do not already have a saved review outcome.",
		"Use `task_review` with action `review` to record an outcome for each listed task.",
		"Completed engineer turn user request: " + strings.TrimSpace(result.UserText),
	}
	if assistant := boundedDerivedTurnText(result.AssistantText, autoReviewContextMaxChars); assistant != "" {
		lines = append(lines,
			"Completed engineer turn assistant output:",
			assistant,
		)
	}
	taskLines := autoReviewUnreviewedCompletedTaskLines(state)
	if len(taskLines) == 0 {
		taskLines = []string{"- none"}
	}
	lines = append(lines, "Completed unreviewed tasks:")
	lines = append(lines, taskLines...)
	return []prompt.Fragment{{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "auto-review-compact-context",
		Label:     "auto review context",
		Content:   strings.Join(lines, "\n"),
	}}
}

func autoReviewUnreviewedCompletedTaskLines(state events.SessionState) []string {
	lines := make([]string, 0, len(state.TaskOrder))
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[taskID]
		if task == nil || strings.TrimSpace(task.Status) != events.TaskStatusCompleted || strings.TrimSpace(task.ReviewStatus) != "" {
			continue
		}
		lines = append(lines, "- "+autoReviewTaskContextLine(*task))
	}
	return lines
}

func autoReviewTaskContextLine(task events.TaskState) string {
	parts := []string{
		"id=" + strings.TrimSpace(task.TaskID),
		"title=" + strings.TrimSpace(task.Title),
	}
	if summary := boundedDerivedTurnText(task.Progress, 800); summary != "" {
		parts = append(parts, "summary="+summary)
	}
	if notes := boundedDerivedTurnText(task.Notes, 800); notes != "" {
		parts = append(parts, "notes="+notes)
	}
	return strings.Join(parts, "; ")
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
