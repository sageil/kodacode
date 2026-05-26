package tui

import (
	"context"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

type controller interface {
	Snapshot(ctx context.Context, sessionID string) (events.SessionState, error)
	BudgetStatus(ctx context.Context, sessionID string) (app.BudgetStatus, error)
	SessionUsageSummary(ctx context.Context, sessionID string) (app.SessionUsageSummary, error)
	Watch(ctx context.Context, sessionID string, afterSequence int64) (<-chan events.Event, error)
	LoadToolResult(ctx context.Context, sessionID, turnID, callID string) (app.ToolResultDetail, error)
	LoadToolMutationDetail(ctx context.Context, sessionID, turnID, callID string) (app.ToolMutationDetail, error)
	WorkspaceStatus(ctx context.Context, sessionID string) (app.WorkspaceStatus, error)
	RestoreTurnWrites(ctx context.Context, sessionID, sourceTurnID string) (app.RestoreSessionTurnWritesResult, error)
	CompactSessionHistory(ctx context.Context, sessionID, turnID string) (app.CompactSessionResult, error)
	SetPermissionMode(ctx context.Context, sessionID string, mode app.PermissionMode) error
	StartTurn(ctx context.Context, sessionID, turnID, userText string, attachments []app.AttachmentInput, agentID string, thinkingEnabled bool, thinkingMode string, skillIDs []string) error
	StartReview(ctx context.Context, sessionID, turnID, instructions string, thinkingEnabled bool, thinkingMode string, skillIDs []string) error
	CancelTurn(ctx context.Context, sessionID, turnID string) error
	RunLocalShellCommand(ctx context.Context, sessionID, turnID, command string) error
	AnswerQuestion(
		ctx context.Context,
		sessionID, turnID, requestID, userText, answer string,
		skillIDs []string,
	) (app.RunSessionResult, error)
	AnswerDelegatedQuestion(
		ctx context.Context,
		sessionID, handoffID, answer string,
	) (app.AnswerDelegatedSessionQuestionResult, error)
	ResolvePermission(
		ctx context.Context,
		sessionID, turnID, requestID, userText string,
		skillIDs []string,
		decision events.PermissionDecision,
		scope events.PermissionScope,
		grantPath string,
		recursive bool,
		executionDecision events.ExecutionApprovalDecision,
		executionExecPolicy *events.ExecutionPolicyAmendment,
		executionNetworkPolicy *events.ExecutionNetworkPolicyAmendment,
	) error
	ResolveDelegatedPermission(
		ctx context.Context,
		sessionID, handoffID string,
		decision events.PermissionDecision,
		scope events.PermissionScope,
		grantPath string,
		recursive bool,
		executionDecision events.ExecutionApprovalDecision,
		executionExecPolicy *events.ExecutionPolicyAmendment,
		executionNetworkPolicy *events.ExecutionNetworkPolicyAmendment,
	) error
	ReuseDelegatedResult(ctx context.Context, sessionID, handoffID string) error
}
