package tui

import (
	"context"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func startTurnCmd(ctx context.Context, controller controller, sessionID, turnID, userText string, attachments []app.AttachmentInput, agentID, workflowID string, thinkingEnabled bool, thinkingMode string, skillIDs []string) tea.Cmd {
	return func() tea.Msg {
		return operationDoneMsg{err: controller.StartTurn(ctx, sessionID, turnID, userText, attachments, agentID, workflowID, thinkingEnabled, thinkingMode, skillIDs)}
	}
}

func startReviewCmd(ctx context.Context, controller controller, sessionID, turnID, instructions string, thinkingEnabled bool, thinkingMode string, skillIDs []string) tea.Cmd {
	return func() tea.Msg {
		return operationDoneMsg{err: controller.StartReview(ctx, sessionID, turnID, instructions, thinkingEnabled, thinkingMode, skillIDs)}
	}
}

func cancelTurnCmd(ctx context.Context, controller controller, sessionID, turnID string) tea.Cmd {
	return func() tea.Msg {
		return turnCancelRequestedMsg{err: controller.CancelTurn(ctx, sessionID, turnID)}
	}
}

func refreshSessionSnapshotCmd(ctx context.Context, controller controller, sessionID string) tea.Cmd {
	return func() tea.Msg {
		state, err := controller.Snapshot(ctx, sessionID)
		return sessionSnapshotRefreshedMsg{
			sessionID: sessionID,
			state:     state,
			err:       err,
		}
	}
}

func restoreTurnWritesCmd(ctx context.Context, controller controller, sessionID, sourceTurnID string) tea.Cmd {
	return func() tea.Msg {
		result, err := controller.RestoreTurnWrites(ctx, sessionID, sourceTurnID)
		return turnWritesRestoredMsg{
			result: result,
			err:    err,
		}
	}
}

func resumeWorkflowCmd(ctx context.Context, controller controller, sessionID, turnID string) tea.Cmd {
	return func() tea.Msg {
		return workflowResumedMsg{err: controller.ResumeWorkflow(ctx, sessionID, turnID)}
	}
}

func compactSessionHistoryCmd(ctx context.Context, controller controller, sessionID, turnID string) tea.Cmd {
	return func() tea.Msg {
		result, err := controller.CompactSessionHistory(ctx, sessionID, turnID)
		return sessionCompactedMsg{
			result: result,
			err:    err,
		}
	}
}

func resolvePermissionCmd(
	ctx context.Context,
	controller controller,
	sessionID, turnID, requestID, userText string,
	skillIDs []string,
	decision events.PermissionDecision,
	scope events.PermissionScope,
	grantPath string,
	recursive bool,
	executionDecision events.ExecutionApprovalDecision,
	executionExecPolicy *events.ExecutionPolicyAmendment,
	executionNetworkPolicy *events.ExecutionNetworkPolicyAmendment,
) tea.Cmd {
	return func() tea.Msg {
		return operationDoneMsg{
			err: controller.ResolvePermission(
				ctx,
				sessionID,
				turnID,
				requestID,
				userText,
				skillIDs,
				decision,
				scope,
				grantPath,
				recursive,
				executionDecision,
				executionExecPolicy,
				executionNetworkPolicy,
			),
		}
	}
}

func answerQuestionCmd(
	ctx context.Context,
	controller controller,
	sessionID, turnID, requestID, userText, answer string,
	skillIDs []string,
) tea.Cmd {
	return func() tea.Msg {
		result, err := controller.AnswerQuestion(
			ctx,
			sessionID,
			turnID,
			requestID,
			userText,
			answer,
			skillIDs,
		)
		return operationDoneMsg{
			err:           err,
			sessionResult: &result,
		}
	}
}

func answerDelegatedQuestionCmd(
	ctx context.Context,
	controller controller,
	sessionID, handoffID, answer string,
) tea.Cmd {
	return func() tea.Msg {
		result, err := controller.AnswerDelegatedQuestion(
			ctx,
			sessionID,
			handoffID,
			answer,
		)
		return operationDoneMsg{
			err:                     err,
			delegatedQuestionResult: &result,
		}
	}
}

func resolveDelegatedPermissionCmd(
	ctx context.Context,
	controller controller,
	sessionID, handoffID string,
	decision events.PermissionDecision,
	scope events.PermissionScope,
	grantPath string,
	recursive bool,
	executionDecision events.ExecutionApprovalDecision,
	executionExecPolicy *events.ExecutionPolicyAmendment,
	executionNetworkPolicy *events.ExecutionNetworkPolicyAmendment,
) tea.Cmd {
	return func() tea.Msg {
		return operationDoneMsg{
			err: controller.ResolveDelegatedPermission(
				ctx,
				sessionID,
				handoffID,
				decision,
				scope,
				grantPath,
				recursive,
				executionDecision,
				executionExecPolicy,
				executionNetworkPolicy,
			),
		}
	}
}
