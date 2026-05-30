package tui

import (
	"context"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type Backend interface {
	EvaluateStartupTrust(ctx context.Context, workspaceRoot string) (app.StartupTrustState, error)
	ResolveStartupTrust(ctx context.Context, input app.ResolveStartupTrustInput) error
	WorkspaceTrustState(ctx context.Context, workspaceRoot string) (app.WorkspaceTrustState, error)
	RevokeTrust(ctx context.Context, input app.RevokeTrustInput) (app.WorkspaceTrustState, error)
	OpenWorkspaceSession(ctx context.Context, workspaceRoot string, additionalRoots []string, resume bool) (app.OpenWorkspaceSessionResult, error)
	Snapshot(ctx context.Context, sessionID string) (events.SessionState, error)
	BudgetStatus(ctx context.Context, sessionID string) (app.BudgetStatus, error)
	SessionUsageSummary(ctx context.Context, sessionID string) (app.SessionUsageSummary, error)
	Watch(ctx context.Context, sessionID string, afterSequence int64) (<-chan events.Event, error)
	LoadToolResult(ctx context.Context, sessionID, turnID, callID string) (app.ToolResultDetail, error)
	LoadToolMutationDetail(ctx context.Context, sessionID, turnID, callID string) (app.ToolMutationDetail, error)
	WorkspaceStatus(ctx context.Context, sessionID string) (app.WorkspaceStatus, error)
	RestoreTurnWrites(ctx context.Context, sessionID, sourceTurnID string) (app.RestoreSessionTurnWritesResult, error)
	BranchSessionFromTurn(ctx context.Context, input app.BranchSessionFromTurnInput) (app.BranchSessionFromTurnResult, error)
	CompactSessionHistory(ctx context.Context, sessionID, turnID string) (app.CompactSessionResult, error)
	InitializeWorkspaceInstructions(ctx context.Context, input app.InitializeWorkspaceInstructionsInput) (app.InitializeWorkspaceInstructionsResult, error)
	CompressWorkspacePromptSources(ctx context.Context, input app.CompressWorkspacePromptSourcesInput) (app.CompressWorkspacePromptSourcesResult, error)
	ListAgents(ctx context.Context, workspaceRoot string) ([]app.AvailableAgent, error)
	ListSkills(ctx context.Context, workspaceRoot string) ([]app.AvailableSkill, error)
	ListWorkspacePaths(ctx context.Context, workspaceRoot string) ([]app.WorkspacePath, error)
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
	DialogState(ctx context.Context) (app.DialogState, error)
	ListSessions(ctx context.Context) ([]app.SessionSummary, error)
	GenerateBranchSummary(ctx context.Context, sessionID string) (app.GenerateBranchSummaryResult, error)
	ListPromptHistory(ctx context.Context, limit int) ([]app.PromptHistoryEntry, error)
	DeleteSession(ctx context.Context, sessionID string) error
	SetSessionTitle(ctx context.Context, sessionID, title string) error
	SetThemeName(ctx context.Context, themeName string) error
	SetTUILayout(ctx context.Context, layout string) error
	SetPrimaryModel(ctx context.Context, sessionID string, model provider.ModelRef) error
	SetUtilityModel(ctx context.Context, model provider.ModelRef) error
	SetReviewerModel(ctx context.Context, model provider.ModelRef) error
	RefreshModelCatalog(ctx context.Context) (app.DialogState, error)
	BeginOpenAIAuth(ctx context.Context) (app.OpenAIAuthChallenge, error)
	CompleteOpenAIAuth(ctx context.Context, challenge app.OpenAIAuthChallenge) (app.DialogState, error)
	BeginGitHubCopilotAuth(ctx context.Context, baseURL string) (app.GitHubCopilotAuthChallenge, error)
	CompleteGitHubCopilotAuth(ctx context.Context, challenge app.GitHubCopilotAuthChallenge) (app.DialogState, error)
	SaveProvider(ctx context.Context, input app.ProviderConnectionInput) error
	RemoveProvider(ctx context.Context, providerID string) error
	Close() error
}
