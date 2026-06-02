package tui

import (
	"context"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

func (b *LocalBackend) EvaluateStartupTrust(ctx context.Context, workspaceRoot string) (app.StartupTrustState, error) {
	return b.runtime.EvaluateStartupTrust(ctx, workspaceRoot)
}

func (b *LocalBackend) ResolveStartupTrust(ctx context.Context, input app.ResolveStartupTrustInput) error {
	return b.runtime.ResolveStartupTrust(ctx, input)
}

func (b *LocalBackend) WorkspaceTrustState(ctx context.Context, workspaceRoot string) (app.WorkspaceTrustState, error) {
	return b.runtime.WorkspaceTrustState(ctx, workspaceRoot)
}

func (b *LocalBackend) RevokeTrust(ctx context.Context, input app.RevokeTrustInput) (app.WorkspaceTrustState, error) {
	return b.runtime.RevokeTrust(ctx, input)
}

func (b *LocalBackend) OpenWorkspaceSession(ctx context.Context, workspaceRoot string, additionalRoots []string, resume bool) (app.OpenWorkspaceSessionResult, error) {
	return b.runtime.OpenWorkspaceSession(ctx, workspaceRoot, additionalRoots, resume)
}

func (b *LocalBackend) Snapshot(ctx context.Context, sessionID string) (events.SessionState, error) {
	return b.runtime.SnapshotSession(ctx, sessionID)
}

func (b *LocalBackend) BudgetStatus(ctx context.Context, sessionID string) (app.BudgetStatus, error) {
	return b.runtime.BudgetStatus(ctx, sessionID)
}

func (b *LocalBackend) SessionUsageSummary(ctx context.Context, sessionID string) (app.SessionUsageSummary, error) {
	return b.runtime.SessionUsageSummary(ctx, sessionID)
}

func (b *LocalBackend) Watch(ctx context.Context, sessionID string, afterSequence int64) (<-chan events.Event, error) {
	return b.runtime.WatchSession(ctx, sessionID, afterSequence)
}

func (b *LocalBackend) LoadToolResult(ctx context.Context, sessionID, turnID, callID string) (app.ToolResultDetail, error) {
	return b.runtime.LoadSessionToolResult(ctx, sessionID, turnID, callID)
}

func (b *LocalBackend) LoadToolMutationDetail(ctx context.Context, sessionID, turnID, callID string) (app.ToolMutationDetail, error) {
	return b.runtime.LoadSessionToolMutationDetail(ctx, sessionID, turnID, callID)
}

func (b *LocalBackend) WorkspaceStatus(ctx context.Context, sessionID string) (app.WorkspaceStatus, error) {
	return b.runtime.WorkspaceStatus(ctx, sessionID)
}

func (b *LocalBackend) RestoreTurnWrites(ctx context.Context, sessionID, sourceTurnID string) (app.RestoreSessionTurnWritesResult, error) {
	return b.runtime.RestoreSessionTurnWrites(ctx, app.RestoreSessionTurnWritesInput{
		SessionID:    sessionID,
		SourceTurnID: sourceTurnID,
	})
}

func (b *LocalBackend) BranchSessionFromTurn(ctx context.Context, input app.BranchSessionFromTurnInput) (app.BranchSessionFromTurnResult, error) {
	return b.runtime.BranchSessionFromTurn(ctx, input)
}

func (b *LocalBackend) SetSessionTitle(ctx context.Context, sessionID, title string) error {
	_, err := b.runtime.Sessions.SetTitle(ctx, sessionID, title)
	return err
}

func (b *LocalBackend) CompactSessionHistory(ctx context.Context, sessionID, turnID string) (app.CompactSessionResult, error) {
	return b.runtime.CompactSessionHistory(ctx, app.CompactSessionInput{
		SessionID: sessionID,
		TurnID:    turnID,
	})
}

func (b *LocalBackend) InitializeWorkspaceInstructions(ctx context.Context, input app.InitializeWorkspaceInstructionsInput) (app.InitializeWorkspaceInstructionsResult, error) {
	return b.runtime.InitializeWorkspaceInstructions(ctx, input)
}

func (b *LocalBackend) CompressWorkspacePromptSources(ctx context.Context, input app.CompressWorkspacePromptSourcesInput) (app.CompressWorkspacePromptSourcesResult, error) {
	return b.runtime.CompressWorkspacePromptSources(ctx, input)
}

func (b *LocalBackend) ListAgents(ctx context.Context, workspaceRoot string) ([]app.AvailableAgent, error) {
	return b.runtime.ListAgents(ctx, workspaceRoot)
}

func (b *LocalBackend) ListWorkflows(ctx context.Context, workspaceRoot string) ([]app.AvailableWorkflow, error) {
	return b.runtime.ListWorkflows(ctx, workspaceRoot)
}

func (b *LocalBackend) ListSkills(ctx context.Context, workspaceRoot string) ([]app.AvailableSkill, error) {
	return b.runtime.ListSkills(ctx, workspaceRoot)
}

func (b *LocalBackend) SetPermissionMode(ctx context.Context, sessionID string, mode app.PermissionMode) error {
	return b.runtime.SetSessionPermissionMode(ctx, sessionID, mode)
}

func (b *LocalBackend) ResumeWorkflow(ctx context.Context, sessionID, turnID string) error {
	return b.runtime.ResumeWorkflow(ctx, app.ResumeWorkflowInput{
		SessionID: sessionID,
		TurnID:    turnID,
	})
}

func (b *LocalBackend) StartTurn(ctx context.Context, sessionID, turnID, userText string, attachments []app.AttachmentInput, agentID, workflowID string, thinkingEnabled bool, thinkingMode string, skillIDs []string) error {
	_, err := b.runtime.StartSessionTurn(ctx, app.StartSessionTurnInput{
		SessionID:       sessionID,
		TurnID:          turnID,
		UserText:        userText,
		Attachments:     append([]app.AttachmentInput(nil), attachments...),
		AgentID:         agentID,
		WorkflowID:      workflowID,
		SkillIDs:        append([]string(nil), skillIDs...),
		ThinkingEnabled: thinkingEnabled,
		ThinkingMode:    thinkingMode,
	})
	return err
}

func (b *LocalBackend) StartReview(ctx context.Context, sessionID, turnID, instructions string, thinkingEnabled bool, thinkingMode string, skillIDs []string) error {
	_, err := b.runtime.StartSessionReview(ctx, app.StartSessionReviewInput{
		SessionID:       sessionID,
		TurnID:          turnID,
		Instructions:    instructions,
		SkillIDs:        append([]string(nil), skillIDs...),
		ThinkingEnabled: thinkingEnabled,
		ThinkingMode:    thinkingMode,
	})
	return err
}

func (b *LocalBackend) CancelTurn(ctx context.Context, sessionID, turnID string) error {
	return b.runtime.CancelSessionTurn(ctx, app.CancelSessionTurnInput{
		SessionID: sessionID,
		TurnID:    turnID,
	})
}

func (b *LocalBackend) RunLocalShellCommand(ctx context.Context, sessionID, turnID, command string) error {
	_, err := b.runtime.RunSessionLocalShellCommand(ctx, app.LocalShellCommandInput{
		SessionID: sessionID,
		TurnID:    turnID,
		Command:   command,
	})
	return err
}

func (b *LocalBackend) AnswerQuestion(
	ctx context.Context,
	sessionID, turnID, requestID, userText, answer string,
	skillIDs []string,
) (app.RunSessionResult, error) {
	return b.runtime.AnswerSessionQuestion(ctx, app.AnswerSessionQuestionInput{
		SessionID: sessionID,
		TurnID:    turnID,
		RequestID: requestID,
		Answer:    answer,
		UserText:  userText,
		SkillIDs:  append([]string(nil), skillIDs...),
	})
}

func (b *LocalBackend) AnswerDelegatedQuestion(
	ctx context.Context,
	sessionID, handoffID, answer string,
) (app.AnswerDelegatedSessionQuestionResult, error) {
	return b.runtime.AnswerDelegatedSessionQuestion(ctx, app.AnswerDelegatedSessionQuestionInput{
		ParentSessionID: sessionID,
		HandoffID:       handoffID,
		Answer:          answer,
	})
}

func (b *LocalBackend) ResolvePermission(
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
) error {
	_, err := b.runtime.ResolveSessionTurn(ctx, app.ResolveSessionTurnInput{
		SessionID:              sessionID,
		TurnID:                 turnID,
		PermissionRequestID:    requestID,
		UserText:               userText,
		SkillIDs:               append([]string(nil), skillIDs...),
		Decision:               decision,
		Scope:                  scope,
		GrantPath:              grantPath,
		Recursive:              recursive,
		ExecutionDecision:      executionDecision,
		ExecutionExecPolicy:    executionExecPolicy,
		ExecutionNetworkPolicy: executionNetworkPolicy,
	})
	return err
}

func (b *LocalBackend) ResolveDelegatedPermission(
	ctx context.Context,
	sessionID, handoffID string,
	decision events.PermissionDecision,
	scope events.PermissionScope,
	grantPath string,
	recursive bool,
	executionDecision events.ExecutionApprovalDecision,
	executionExecPolicy *events.ExecutionPolicyAmendment,
	executionNetworkPolicy *events.ExecutionNetworkPolicyAmendment,
) error {
	_, err := b.runtime.ResolveDelegatedSessionTurn(ctx, app.ResolveDelegatedSessionTurnInput{
		ParentSessionID:        sessionID,
		HandoffID:              handoffID,
		Decision:               decision,
		Scope:                  scope,
		GrantPath:              grantPath,
		Recursive:              recursive,
		ExecutionDecision:      executionDecision,
		ExecutionExecPolicy:    executionExecPolicy,
		ExecutionNetworkPolicy: executionNetworkPolicy,
	})
	return err
}

func (b *LocalBackend) DialogState(_ context.Context) (app.DialogState, error) {
	return b.runtime.DialogState()
}

func (b *LocalBackend) ListSessions(ctx context.Context) ([]app.SessionSummary, error) {
	return b.runtime.ListSessions(ctx)
}

func (b *LocalBackend) GenerateBranchSummary(ctx context.Context, sessionID string) (app.GenerateBranchSummaryResult, error) {
	return b.runtime.GenerateBranchSummary(ctx, app.GenerateBranchSummaryInput{SessionID: sessionID})
}

func (b *LocalBackend) DeleteSession(ctx context.Context, sessionID string) error {
	return b.runtime.DeleteSession(ctx, sessionID)
}
