package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

const (
	taskBlockReasonExecutionStalledNoProgress           = "execution stalled: the model repeated the same tool calls without making progress"
	taskBlockReasonExecutionStalledProviderRequestLimit = "execution stalled: the turn hit the provider request limit without completing"
	taskBlockReasonReviewStalledNoProgress              = "review stalled: the model repeated the same tool calls without making progress while addressing review findings"
	taskBlockReasonReviewStalledProviderRequestLimit    = "review stalled: the turn hit the provider request limit while addressing review findings"
)

func shouldApplyTaskWorkflowFailurePolicyCode(code events.TurnFailureCode) bool {
	switch code {
	case events.TurnFailureCodeProviderRequestLimit, events.TurnFailureCodeNoProgress:
		return true
	default:
		return false
	}
}

func (r *Runtime) applyTaskWorkflowFailurePolicyForTurn(ctx context.Context, sessionID, turnID string) error {
	if r == nil || r.Sessions == nil {
		return nil
	}
	state, err := r.Sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return err
	}
	turn := state.Turns[turnID]
	if turn == nil || turn.Status != events.TurnStatusFailed || !shouldApplyTaskWorkflowFailurePolicyCode(turn.ErrorCode) {
		return nil
	}
	task := activeWorkflowTask(state)
	if task == nil {
		return nil
	}
	_, err = r.Sessions.BlockTask(ctx, BlockTaskInput{
		SessionID:   sessionID,
		TurnID:      turnID,
		TaskID:      task.TaskID,
		BlockReason: taskWorkflowBlockReasonForCode(task, turn.ErrorCode),
	})
	return err
}

func activeWorkflowTask(state events.SessionState) *events.TaskState {
	activeTaskIDs := make([]string, 0, len(state.TaskOrder))
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[taskID]
		if task == nil || strings.TrimSpace(task.Status) != events.TaskStatusInProgress {
			continue
		}
		activeTaskIDs = append(activeTaskIDs, taskID)
	}
	if len(activeTaskIDs) == 0 {
		return nil
	}
	var selected *events.TaskState
	for _, candidateID := range activeTaskIDs {
		deepest := true
		for _, otherID := range activeTaskIDs {
			if otherID == candidateID {
				continue
			}
			if !taskIsAncestorOf(state, otherID, candidateID) {
				deepest = false
				break
			}
		}
		if deepest {
			if selected != nil {
				return nil
			}
			selected = state.Tasks[candidateID]
		}
	}
	return selected
}

func activeReviewPlanWorkflowTask(state events.SessionState) *events.TaskState {
	task := activeWorkflowTask(state)
	if !taskHasOutstandingReviewFindings(task) {
		return nil
	}
	return task
}

func taskWorkflowBlockReasonForCode(task *events.TaskState, code events.TurnFailureCode) string {
	if taskHasOutstandingReviewFindings(task) {
		if code == events.TurnFailureCodeProviderRequestLimit {
			return taskBlockReasonReviewStalledProviderRequestLimit
		}
		return taskBlockReasonReviewStalledNoProgress
	}
	if code == events.TurnFailureCodeProviderRequestLimit {
		return taskBlockReasonExecutionStalledProviderRequestLimit
	}
	return taskBlockReasonExecutionStalledNoProgress
}

func taskHasOutstandingReviewFindings(task *events.TaskState) bool {
	if task == nil {
		return false
	}
	switch strings.TrimSpace(task.ReviewStatus) {
	case events.TaskReviewStatusConcern, events.TaskReviewStatusFail:
		return true
	default:
		return false
	}
}
