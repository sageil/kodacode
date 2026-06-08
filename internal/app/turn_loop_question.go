package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

const (
	loopResolutionAnswerContinue               = "Continue"
	loopResolutionAnswerStop                   = "Stop turn"
	loopResolutionAnswerBlock                  = "Block current task and stop"
	providerRequestLimitAnswerAllowOnce        = "Allow once"
	providerRequestLimitAnswerAllowSessionYOLO = "Allow per session YOLO"
)

func isLoopResolutionQuestion(request *events.QuestionRequestState) bool {
	return request != nil && strings.TrimSpace(request.Purpose) == events.QuestionPurposeTurnLoopResolution
}

func loopResolutionOptions(hasActiveTask bool) []string {
	options := []string{
		loopResolutionAnswerContinue,
		loopResolutionAnswerStop,
	}
	if hasActiveTask {
		options = append(options, loopResolutionAnswerBlock)
	}
	return options
}

func loopResolutionQuestionText(hasActiveTask bool) string {
	if hasActiveTask {
		return "The model appears to be looping on this task. Continue, stop this turn, or block the current task."
	}
	return "The model appears to be looping on this task. Continue or stop this turn."
}

func providerRequestLimitQuestionText(hasActiveTask bool) string {
	if hasActiveTask {
		return "The turn reached its assistant roundtrip limit. Continue, stop this turn, or block the current task."
	}
	return "The turn reached its assistant roundtrip limit. Continue or stop this turn."
}

func providerRequestLimitReviewPlanQuestionText() string {
	return "The turn reached its assistant roundtrip limit while running the engineer review/plan/execute workflow. Allow one more pass, stop this turn, or disable the limit for this session."
}

func providerRequestLimitAllowsSessionDisable(state events.SessionState, turnID string) bool {
	return activeReviewPlanWorkflowTask(state) != nil ||
		activeReviewPlanExecuteWorkflowTurn(state, turnID) ||
		approvedPlannerPlanExecutionTurn(state, turnID)
}

func activeReviewPlanExecuteWorkflowTurn(state events.SessionState, turnID string) bool {
	turn := state.Turns[strings.TrimSpace(turnID)]
	if turn == nil || turn.Config == nil {
		return false
	}
	return isReviewPlanHarnessParentTurn(turn.Config.AgentID, turn.UserText)
}

func approvedPlannerPlanExecutionTurn(state events.SessionState, turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return false
	}
	visited := map[string]struct{}{}
	for turnID != "" {
		if _, ok := visited[turnID]; ok {
			return false
		}
		visited[turnID] = struct{}{}
		turn := state.Turns[turnID]
		if turn == nil || turn.ContinuationStart == nil {
			return false
		}
		previousTurnID := strings.TrimSpace(turn.ContinuationStart.PreviousTurnID)
		if previousTurnID == "" {
			return false
		}
		if plannerPlanApplyAnsweredOnTurn(state, previousTurnID) {
			return true
		}
		turnID = previousTurnID
	}
	return false
}

func plannerPlanApplyAnsweredOnTurn(state events.SessionState, turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return false
	}
	for _, answer := range state.QuestionAnswers {
		if answer == nil {
			continue
		}
		if strings.TrimSpace(answer.TurnID) != turnID {
			continue
		}
		if strings.TrimSpace(answer.Purpose) == events.QuestionPurposePlannerPlanDecision &&
			strings.TrimSpace(answer.Answer) == plannerPlanApprovalApply {
			return true
		}
	}
	return false
}

func providerRequestLimitOptions(state events.SessionState, turnID string) []string {
	if providerRequestLimitAllowsSessionDisable(state, turnID) {
		return []string{
			providerRequestLimitAnswerAllowOnce,
			loopResolutionAnswerStop,
			providerRequestLimitAnswerAllowSessionYOLO,
		}
	}
	return loopResolutionOptions(activeWorkflowTask(state) != nil)
}

func providerRequestLimitQuestionTextForState(state events.SessionState, turnID string) string {
	if providerRequestLimitAllowsSessionDisable(state, turnID) {
		return providerRequestLimitReviewPlanQuestionText()
	}
	return providerRequestLimitQuestionText(activeWorkflowTask(state) != nil)
}

func (r *TurnRunner) requestLoopResolutionQuestion(ctx context.Context, sessionID, turnID string) (string, error) {
	state, err := r.sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}
	hasActiveTask := activeWorkflowTask(state) != nil
	return r.sessions.RequestQuestion(ctx, QuestionRequestInput{
		SessionID: sessionID,
		TurnID:    turnID,
		Question:  loopResolutionQuestionText(hasActiveTask),
		Options:   loopResolutionOptions(hasActiveTask),
		Purpose:   events.QuestionPurposeTurnLoopResolution,
	})
}

func (r *TurnRunner) requestProviderRequestLimitQuestion(ctx context.Context, sessionID, turnID string) (string, error) {
	state, err := r.sessions.Snapshot(ctx, sessionID)
	if err != nil {
		return "", err
	}
	return r.sessions.RequestQuestion(ctx, QuestionRequestInput{
		SessionID: sessionID,
		TurnID:    turnID,
		Question:  providerRequestLimitQuestionTextForState(state, turnID),
		Options:   providerRequestLimitOptions(state, turnID),
		Purpose:   events.QuestionPurposeTurnLoopResolution,
	})
}
