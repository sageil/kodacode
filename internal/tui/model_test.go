package tui

import (
	"context"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type fakeController struct {
	snapshotErr              error
	budgetStatusErr          error
	sessionUsageSummaryErr   error
	watchErr                 error
	startupTrustErr          error
	resolveStartupTrustErr   error
	workspaceTrustErr        error
	revokeTrustErr           error
	snapshots                map[string]events.SessionState
	watchCh                  <-chan events.Event
	watchByID                map[string]<-chan events.Event
	workspaceStatusErr       error
	restoreTurnWritesErr     error
	compactSessionErr        error
	initInstructionsErr      error
	compressPromptSourcesErr error

	startErr                      error
	startReviewErr                error
	cancelTurnErr                 error
	localShellErr                 error
	answerQuestionErr             error
	answerDelegatedQuestionErr    error
	answerQuestionResult          app.RunSessionResult
	answerDelegatedQuestionResult app.AnswerDelegatedSessionQuestionResult
	resolveErr                    error
	delegatedResolveErr           error
	reuseErr                      error
	dialogStateErr                error
	listSessionsErr               error
	deleteSessionErr              error
	refreshDialogErr              error
	beginOpenAIAuthErr            error
	completeOpenAIAuthErr         error
	beginGitHubCopilotAuthErr     error
	completeGitHubCopilotErr      error
	saveProviderErr               error
	removeProviderErr             error
	setTUILayoutErr               error
	loadToolResultErr             error
	loadToolMutationDetailErr     error
	dialogStateSet                bool
	gitHubCopilotStateSet         bool

	startCalls                   []startCall
	startReviewCalls             []startReviewCall
	cancelTurnCalls              []cancelTurnCall
	localShellCalls              []localShellCall
	answerQuestionCalls          []answerQuestionCall
	answerDelegatedQuestionCalls []answerDelegatedQuestionCall
	permissionModeCalls          []permissionModeCall
	resolveStartupTrustCalls     []app.ResolveStartupTrustInput
	revokeTrustCalls             []app.RevokeTrustInput
	snapshotCalls                []string
	budgetStatusCalls            []string
	sessionUsageSummaryCalls     []string
	watchCalls                   []watchCall
	resolveCalls                 []resolveCall
	delegatedResolveCalls        []resolveDelegatedCall
	reuseCalls                   []reuseCall
	deleteSessionCalls           []string
	setTUILayoutCalls            []string
	setPrimaryModelCalls         []setPrimaryModelCall
	setUtilityModelCalls         []setUtilityModelCall
	setReviewerModelCalls        []setReviewerModelCall
	refreshDialogCalls           int
	beginOpenAIAuthCalls         int
	completeOpenAIAuthCalls      []app.OpenAIAuthChallenge
	beginGitHubCopilotAuthCalls  []string
	completeGitHubCopilotCalls   []app.GitHubCopilotAuthChallenge
	saveProviderCalls            []app.ProviderConnectionInput
	removeProviderCalls          []string
	loadToolResultCalls          []sessionToolCallRef
	loadToolMutationDetailCalls  []sessionToolCallRef
	workspaceStatusCalls         []string
	restoreTurnWritesCalls       []string
	compactSessionCalls          []compactSessionCall
	initInstructionCalls         []app.InitializeWorkspaceInstructionsInput
	compressPromptSourceCalls    []app.CompressWorkspacePromptSourcesInput
	promptHistoryCalls           []promptHistoryCall

	dialogState                 app.DialogState
	openAIChallenge             app.OpenAIAuthChallenge
	openAIState                 app.DialogState
	gitHubCopilotChallenge      app.GitHubCopilotAuthChallenge
	gitHubCopilotState          app.DialogState
	budgetStatus                app.BudgetStatus
	sessionUsageSummary         app.SessionUsageSummary
	startupTrust                app.StartupTrustState
	workspaceTrust              app.WorkspaceTrustState
	sessions                    []app.SessionSummary
	toolResults                 map[sessionToolCallRef]app.ToolResultDetail
	toolMutationDetails         map[sessionToolCallRef]app.ToolMutationDetail
	workspaceStatus             app.WorkspaceStatus
	restoreTurnWritesResult     app.RestoreSessionTurnWritesResult
	compactSessionResult        app.CompactSessionResult
	initInstructionsResult      app.InitializeWorkspaceInstructionsResult
	compressPromptSourcesResult app.CompressWorkspacePromptSourcesResult
	promptHistory               []app.PromptHistoryEntry
	agents                      []app.AvailableAgent
	skills                      []app.AvailableSkill
	workspacePaths              []app.WorkspacePath
	openAIChallengeSet          bool
	openAIStateSet              bool
}

type watchCall struct {
	SessionID     string
	AfterSequence int64
}

type startCall struct {
	SessionID       string
	TurnID          string
	UserText        string
	Attachments     []app.AttachmentInput
	AgentID         string
	ThinkingEnabled bool
	ThinkingMode    string
	SkillIDs        []string
}

type startReviewCall struct {
	SessionID       string
	TurnID          string
	Instructions    string
	ThinkingEnabled bool
	ThinkingMode    string
	SkillIDs        []string
}

type cancelTurnCall struct {
	SessionID string
	TurnID    string
}

type permissionModeCall struct {
	SessionID string
	Mode      app.PermissionMode
}

type localShellCall struct {
	SessionID string
	TurnID    string
	Command   string
}

type compactSessionCall struct {
	SessionID string
	TurnID    string
}

type answerQuestionCall struct {
	SessionID string
	TurnID    string
	RequestID string
	UserText  string
	Answer    string
	SkillIDs  []string
}

type answerDelegatedQuestionCall struct {
	SessionID string
	HandoffID string
	Answer    string
}

type setPrimaryModelCall struct {
	SessionID string
	Model     provider.ModelRef
}

type setUtilityModelCall struct {
	Model provider.ModelRef
}

type setReviewerModelCall struct {
	Model provider.ModelRef
}

type resolveCall struct {
	SessionID              string
	TurnID                 string
	RequestID              string
	UserText               string
	SkillIDs               []string
	Decision               events.PermissionDecision
	Scope                  events.PermissionScope
	GrantPath              string
	Recursive              bool
	ExecutionDecision      events.ExecutionApprovalDecision
	ExecutionExecPolicy    *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy *events.ExecutionNetworkPolicyAmendment
}

type resolveDelegatedCall struct {
	SessionID              string
	HandoffID              string
	Decision               events.PermissionDecision
	Scope                  events.PermissionScope
	GrantPath              string
	Recursive              bool
	ExecutionDecision      events.ExecutionApprovalDecision
	ExecutionExecPolicy    *events.ExecutionPolicyAmendment
	ExecutionNetworkPolicy *events.ExecutionNetworkPolicyAmendment
}

type reuseCall struct {
	SessionID string
	HandoffID string
}

type promptHistoryCall struct {
	Limit int
}

func (f *fakeController) Snapshot(_ context.Context, sessionID string) (events.SessionState, error) {
	f.snapshotCalls = append(f.snapshotCalls, sessionID)
	if f.snapshotErr != nil {
		return events.SessionState{}, f.snapshotErr
	}
	if snapshot, ok := f.snapshots[sessionID]; ok {
		return snapshot, nil
	}
	return events.SessionState{SessionID: sessionID, LastSequence: -1}, nil
}

func (f *fakeController) EvaluateStartupTrust(_ context.Context, _ string) (app.StartupTrustState, error) {
	if f.startupTrustErr != nil {
		return app.StartupTrustState{}, f.startupTrustErr
	}
	return f.startupTrust, nil
}

func (f *fakeController) ResolveStartupTrust(_ context.Context, input app.ResolveStartupTrustInput) error {
	f.resolveStartupTrustCalls = append(f.resolveStartupTrustCalls, app.ResolveStartupTrustInput{
		WorkspaceRoot:   input.WorkspaceRoot,
		TrustWorkspace:  input.TrustWorkspace,
		ServerDecisions: cloneBoolMap(input.ServerDecisions),
	})
	return f.resolveStartupTrustErr
}

func (f *fakeController) WorkspaceTrustState(_ context.Context, _ string) (app.WorkspaceTrustState, error) {
	if f.workspaceTrustErr != nil {
		return app.WorkspaceTrustState{}, f.workspaceTrustErr
	}
	return f.workspaceTrust, nil
}

func (f *fakeController) RevokeTrust(_ context.Context, input app.RevokeTrustInput) (app.WorkspaceTrustState, error) {
	f.revokeTrustCalls = append(f.revokeTrustCalls, input)
	if f.revokeTrustErr != nil {
		return app.WorkspaceTrustState{}, f.revokeTrustErr
	}
	return f.workspaceTrust, nil
}

func (f *fakeController) BudgetStatus(_ context.Context, sessionID string) (app.BudgetStatus, error) {
	f.budgetStatusCalls = append(f.budgetStatusCalls, sessionID)
	if f.budgetStatusErr != nil {
		return app.BudgetStatus{}, f.budgetStatusErr
	}
	return f.budgetStatus, nil
}

func (f *fakeController) SessionUsageSummary(_ context.Context, sessionID string) (app.SessionUsageSummary, error) {
	f.sessionUsageSummaryCalls = append(f.sessionUsageSummaryCalls, sessionID)
	if f.sessionUsageSummaryErr != nil {
		return app.SessionUsageSummary{}, f.sessionUsageSummaryErr
	}
	return f.sessionUsageSummary, nil
}

func (f *fakeController) Watch(_ context.Context, sessionID string, afterSequence int64) (<-chan events.Event, error) {
	f.watchCalls = append(f.watchCalls, watchCall{
		SessionID:     sessionID,
		AfterSequence: afterSequence,
	})
	if f.watchErr != nil {
		return nil, f.watchErr
	}
	if watchCh, ok := f.watchByID[sessionID]; ok {
		return watchCh, nil
	}
	return f.watchCh, nil
}

func (f *fakeController) LoadToolResult(_ context.Context, sessionID, turnID, callID string) (app.ToolResultDetail, error) {
	ref := sessionToolCallRef{TurnID: turnID, CallID: callID}
	f.loadToolResultCalls = append(f.loadToolResultCalls, ref)
	if f.loadToolResultErr != nil {
		return app.ToolResultDetail{}, f.loadToolResultErr
	}
	if result, ok := f.toolResults[ref]; ok {
		return result, nil
	}
	return app.ToolResultDetail{}, nil
}

func (f *fakeController) LoadToolMutationDetail(_ context.Context, sessionID, turnID, callID string) (app.ToolMutationDetail, error) {
	ref := sessionToolCallRef{TurnID: turnID, CallID: callID}
	f.loadToolMutationDetailCalls = append(f.loadToolMutationDetailCalls, ref)
	if f.loadToolMutationDetailErr != nil {
		return app.ToolMutationDetail{}, f.loadToolMutationDetailErr
	}
	if detail, ok := f.toolMutationDetails[ref]; ok {
		return detail, nil
	}
	return app.ToolMutationDetail{}, nil
}

func TestHandleDialogClosedClearsToolMutationDetailCache(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.toolHydration.loadedMutations[scopedToolKey(model.sessionID, ref)] = loadedToolMutationDetail{
		detail: app.ToolMutationDetail{Path: "/repo/file.txt", Before: "before\n", After: "after\n", Existed: true},
	}
	model.toolHydration.loadingMutations[scopedToolKey(model.sessionID, ref)] = true
	model.dialog = &toolDetailDialog{id: dialogIDToolDetail}

	updated, _ := model.handleDialogClosed(dialogClosedMsg{id: dialogIDToolDetail})
	next := updated.(Model)

	if len(next.toolHydration.loadedMutations) != 0 {
		t.Fatalf("loadedToolMutations = %#v, want empty", next.toolHydration.loadedMutations)
	}
	if len(next.toolHydration.loadingMutations) != 0 {
		t.Fatalf("loadingToolMutations = %#v, want empty", next.toolHydration.loadingMutations)
	}
}

func (f *fakeController) WorkspaceStatus(_ context.Context, sessionID string) (app.WorkspaceStatus, error) {
	f.workspaceStatusCalls = append(f.workspaceStatusCalls, sessionID)
	if f.workspaceStatusErr != nil {
		return app.WorkspaceStatus{}, f.workspaceStatusErr
	}
	return f.workspaceStatus, nil
}

func (f *fakeController) RestoreTurnWrites(_ context.Context, sessionID, sourceTurnID string) (app.RestoreSessionTurnWritesResult, error) {
	f.restoreTurnWritesCalls = append(f.restoreTurnWritesCalls, sessionID+":"+sourceTurnID)
	if f.restoreTurnWritesErr != nil {
		return app.RestoreSessionTurnWritesResult{}, f.restoreTurnWritesErr
	}
	if strings.TrimSpace(f.restoreTurnWritesResult.SourceTurnID) != "" || len(f.restoreTurnWritesResult.Paths) > 0 {
		return f.restoreTurnWritesResult, nil
	}
	return app.RestoreSessionTurnWritesResult{SourceTurnID: sourceTurnID}, nil
}

func (f *fakeController) CompactSessionHistory(_ context.Context, sessionID, turnID string) (app.CompactSessionResult, error) {
	f.compactSessionCalls = append(f.compactSessionCalls, compactSessionCall{
		SessionID: sessionID,
		TurnID:    turnID,
	})
	if f.compactSessionErr != nil {
		return app.CompactSessionResult{}, f.compactSessionErr
	}
	if f.compactSessionResult.TurnID != "" || f.compactSessionResult.Continuation != nil {
		return f.compactSessionResult, nil
	}
	return app.CompactSessionResult{TurnID: turnID}, nil
}

func (f *fakeController) InitializeWorkspaceInstructions(_ context.Context, input app.InitializeWorkspaceInstructionsInput) (app.InitializeWorkspaceInstructionsResult, error) {
	f.initInstructionCalls = append(f.initInstructionCalls, input)
	if f.initInstructionsErr != nil {
		return app.InitializeWorkspaceInstructionsResult{}, f.initInstructionsErr
	}
	if strings.TrimSpace(f.initInstructionsResult.WorkspaceRoot) != "" ||
		strings.TrimSpace(f.initInstructionsResult.AgentsPath) != "" ||
		strings.TrimSpace(f.initInstructionsResult.ClaudePath) != "" ||
		f.initInstructionsResult.AgentsCreated ||
		f.initInstructionsResult.ClaudeCreated {
		return f.initInstructionsResult, nil
	}
	result := app.InitializeWorkspaceInstructionsResult{
		WorkspaceRoot: strings.TrimSpace(input.WorkspaceRoot),
		AgentsPath:    filepath.Join(strings.TrimSpace(input.WorkspaceRoot), "AGENTS.md"),
	}
	if input.IncludeClaude {
		result.ClaudePath = filepath.Join(strings.TrimSpace(input.WorkspaceRoot), "CLAUDE.md")
	}
	return result, nil
}

func (f *fakeController) CompressWorkspacePromptSources(_ context.Context, input app.CompressWorkspacePromptSourcesInput) (app.CompressWorkspacePromptSourcesResult, error) {
	f.compressPromptSourceCalls = append(f.compressPromptSourceCalls, input)
	if f.compressPromptSourcesErr != nil {
		return app.CompressWorkspacePromptSourcesResult{}, f.compressPromptSourcesErr
	}
	if strings.TrimSpace(f.compressPromptSourcesResult.WorkspaceRoot) != "" ||
		strings.TrimSpace(f.compressPromptSourcesResult.AgentsPath) != "" ||
		f.compressPromptSourcesResult.AgentsPresent ||
		f.compressPromptSourcesResult.AgentsUpdated ||
		f.compressPromptSourcesResult.MemoryCount > 0 ||
		f.compressPromptSourcesResult.MemoryUpdatedCount > 0 ||
		f.compressPromptSourcesResult.AgentsBytesBefore > 0 ||
		f.compressPromptSourcesResult.AgentsBytesAfter > 0 ||
		f.compressPromptSourcesResult.MemoryBytesBefore > 0 ||
		f.compressPromptSourcesResult.MemoryBytesAfter > 0 {
		return f.compressPromptSourcesResult, nil
	}
	return app.CompressWorkspacePromptSourcesResult{
		WorkspaceRoot: strings.TrimSpace(input.WorkspaceRoot),
	}, nil
}

func (f *fakeController) ListAgents(_ context.Context, _ string) ([]app.AvailableAgent, error) {
	return append([]app.AvailableAgent(nil), f.agents...), nil
}

func (f *fakeController) ListSkills(_ context.Context, _ string) ([]app.AvailableSkill, error) {
	return append([]app.AvailableSkill(nil), f.skills...), nil
}

func (f *fakeController) ListWorkspacePaths(_ context.Context, _ string) ([]app.WorkspacePath, error) {
	return append([]app.WorkspacePath(nil), f.workspacePaths...), nil
}

func (f *fakeController) SetPermissionMode(_ context.Context, sessionID string, mode app.PermissionMode) error {
	f.permissionModeCalls = append(f.permissionModeCalls, permissionModeCall{
		SessionID: sessionID,
		Mode:      mode,
	})
	return nil
}

func (f *fakeController) StartTurn(_ context.Context, sessionID, turnID, userText string, attachments []app.AttachmentInput, agentID string, thinkingEnabled bool, thinkingMode string, skillIDs []string) error {
	f.startCalls = append(f.startCalls, startCall{
		SessionID:       sessionID,
		TurnID:          turnID,
		UserText:        userText,
		Attachments:     append([]app.AttachmentInput(nil), attachments...),
		AgentID:         agentID,
		ThinkingEnabled: thinkingEnabled,
		ThinkingMode:    thinkingMode,
		SkillIDs:        append([]string(nil), skillIDs...),
	})
	return f.startErr
}

func (f *fakeController) StartReview(_ context.Context, sessionID, turnID, instructions string, thinkingEnabled bool, thinkingMode string, skillIDs []string) error {
	f.startReviewCalls = append(f.startReviewCalls, startReviewCall{
		SessionID:       sessionID,
		TurnID:          turnID,
		Instructions:    instructions,
		ThinkingEnabled: thinkingEnabled,
		ThinkingMode:    thinkingMode,
		SkillIDs:        append([]string(nil), skillIDs...),
	})
	return f.startReviewErr
}

func (f *fakeController) CancelTurn(_ context.Context, sessionID, turnID string) error {
	f.cancelTurnCalls = append(f.cancelTurnCalls, cancelTurnCall{
		SessionID: sessionID,
		TurnID:    turnID,
	})
	return f.cancelTurnErr
}

func (f *fakeController) RunLocalShellCommand(_ context.Context, sessionID, turnID, command string) error {
	f.localShellCalls = append(f.localShellCalls, localShellCall{
		SessionID: sessionID,
		TurnID:    turnID,
		Command:   command,
	})
	return f.localShellErr
}

func (f *fakeController) AnswerQuestion(_ context.Context, sessionID, turnID, requestID, userText, answer string, skillIDs []string) (app.RunSessionResult, error) {
	f.answerQuestionCalls = append(f.answerQuestionCalls, answerQuestionCall{
		SessionID: sessionID,
		TurnID:    turnID,
		RequestID: requestID,
		UserText:  userText,
		Answer:    answer,
		SkillIDs:  append([]string(nil), skillIDs...),
	})
	return f.answerQuestionResult, f.answerQuestionErr
}

func (f *fakeController) AnswerDelegatedQuestion(_ context.Context, sessionID, handoffID, answer string) (app.AnswerDelegatedSessionQuestionResult, error) {
	f.answerDelegatedQuestionCalls = append(f.answerDelegatedQuestionCalls, answerDelegatedQuestionCall{
		SessionID: sessionID,
		HandoffID: handoffID,
		Answer:    answer,
	})
	return f.answerDelegatedQuestionResult, f.answerDelegatedQuestionErr
}

func (f *fakeController) ResolvePermission(
	_ context.Context,
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
	f.resolveCalls = append(f.resolveCalls, resolveCall{
		SessionID:              sessionID,
		TurnID:                 turnID,
		RequestID:              requestID,
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
	return f.resolveErr
}

func (f *fakeController) ResolveDelegatedPermission(
	_ context.Context,
	sessionID, handoffID string,
	decision events.PermissionDecision,
	scope events.PermissionScope,
	grantPath string,
	recursive bool,
	executionDecision events.ExecutionApprovalDecision,
	executionExecPolicy *events.ExecutionPolicyAmendment,
	executionNetworkPolicy *events.ExecutionNetworkPolicyAmendment,
) error {
	f.delegatedResolveCalls = append(f.delegatedResolveCalls, resolveDelegatedCall{
		SessionID:              sessionID,
		HandoffID:              handoffID,
		Decision:               decision,
		Scope:                  scope,
		GrantPath:              grantPath,
		Recursive:              recursive,
		ExecutionDecision:      executionDecision,
		ExecutionExecPolicy:    executionExecPolicy,
		ExecutionNetworkPolicy: executionNetworkPolicy,
	})
	return f.delegatedResolveErr
}

func (f *fakeController) ReuseDelegatedResult(_ context.Context, sessionID, handoffID string) error {
	f.reuseCalls = append(f.reuseCalls, reuseCall{
		SessionID: sessionID,
		HandoffID: handoffID,
	})
	return f.reuseErr
}

func (f *fakeController) OpenWorkspaceSession(_ context.Context, workspaceRoot string, _ []string, resume bool) (app.OpenWorkspaceSessionResult, error) {
	if strings.TrimSpace(workspaceRoot) == "" {
		return app.OpenWorkspaceSessionResult{}, app.ErrWorkspaceRootRequired
	}
	sessionID := "session-created"
	if len(f.snapshots) > 0 {
		for id := range f.snapshots {
			sessionID = id
			break
		}
	}
	return app.OpenWorkspaceSessionResult{SessionID: sessionID, Resumed: resume}, nil
}

func (f *fakeController) DialogState(_ context.Context) (app.DialogState, error) {
	if f.dialogStateErr != nil {
		return app.DialogState{}, f.dialogStateErr
	}
	return f.effectiveDialogState(), nil
}

func (f *fakeController) ListSessions(_ context.Context) ([]app.SessionSummary, error) {
	if f.listSessionsErr != nil {
		return nil, f.listSessionsErr
	}
	return append([]app.SessionSummary(nil), f.sessions...), nil
}

func (f *fakeController) DeleteSession(_ context.Context, sessionID string) error {
	f.deleteSessionCalls = append(f.deleteSessionCalls, sessionID)
	return f.deleteSessionErr
}

func (f *fakeController) SetThemeName(_ context.Context, _ string) error { return nil }

func (f *fakeController) SetTUILayout(_ context.Context, layout string) error {
	f.setTUILayoutCalls = append(f.setTUILayoutCalls, layout)
	return f.setTUILayoutErr
}

func (f *fakeController) SetPrimaryModel(_ context.Context, sessionID string, model provider.ModelRef) error {
	f.setPrimaryModelCalls = append(f.setPrimaryModelCalls, setPrimaryModelCall{
		SessionID: sessionID,
		Model:     model,
	})
	return nil
}

func (f *fakeController) SetUtilityModel(_ context.Context, model provider.ModelRef) error {
	f.setUtilityModelCalls = append(f.setUtilityModelCalls, setUtilityModelCall{Model: model})
	f.dialogState.UtilityModel = model
	return nil
}

func (f *fakeController) SetReviewerModel(_ context.Context, model provider.ModelRef) error {
	f.setReviewerModelCalls = append(f.setReviewerModelCalls, setReviewerModelCall{Model: model})
	f.dialogState.ReviewModelRoute = provider.ModelRoute{Primary: model}
	return nil
}

func (f *fakeController) RefreshModelCatalog(_ context.Context) (app.DialogState, error) {
	f.refreshDialogCalls++
	if f.refreshDialogErr != nil {
		return app.DialogState{}, f.refreshDialogErr
	}
	return f.effectiveDialogState(), nil
}

func (f *fakeController) BeginOpenAIAuth(_ context.Context) (app.OpenAIAuthChallenge, error) {
	f.beginOpenAIAuthCalls++
	if f.beginOpenAIAuthErr != nil {
		return app.OpenAIAuthChallenge{}, f.beginOpenAIAuthErr
	}
	if f.openAIChallengeSet {
		return f.openAIChallenge, nil
	}
	return app.OpenAIAuthChallenge{
		FlowID:           "flow-1",
		AuthorizationURL: "https://auth.openai.com/oauth/authorize?state=test",
		RedirectURI:      "http://localhost:1455/auth/callback",
	}, nil
}

func (f *fakeController) CompleteOpenAIAuth(_ context.Context, challenge app.OpenAIAuthChallenge) (app.DialogState, error) {
	f.completeOpenAIAuthCalls = append(f.completeOpenAIAuthCalls, challenge)
	if f.completeOpenAIAuthErr != nil {
		return app.DialogState{}, f.completeOpenAIAuthErr
	}
	if f.openAIStateSet {
		return f.openAIState, nil
	}
	return f.effectiveDialogState(), nil
}

func (f *fakeController) BeginGitHubCopilotAuth(_ context.Context, baseURL string) (app.GitHubCopilotAuthChallenge, error) {
	f.beginGitHubCopilotAuthCalls = append(f.beginGitHubCopilotAuthCalls, baseURL)
	if f.beginGitHubCopilotAuthErr != nil {
		return app.GitHubCopilotAuthChallenge{}, f.beginGitHubCopilotAuthErr
	}
	if strings.TrimSpace(f.gitHubCopilotChallenge.VerificationURL) == "" {
		return app.GitHubCopilotAuthChallenge{
			BaseURL:         baseURL,
			DeviceCode:      "device-code",
			UserCode:        "ABCD-EFGH",
			VerificationURL: "https://github.com/login/device",
			ExpiresIn:       900,
			Interval:        5,
		}, nil
	}
	return f.gitHubCopilotChallenge, nil
}

func (f *fakeController) CompleteGitHubCopilotAuth(_ context.Context, challenge app.GitHubCopilotAuthChallenge) (app.DialogState, error) {
	f.completeGitHubCopilotCalls = append(f.completeGitHubCopilotCalls, challenge)
	if f.completeGitHubCopilotErr != nil {
		return app.DialogState{}, f.completeGitHubCopilotErr
	}
	if f.gitHubCopilotStateSet {
		return f.gitHubCopilotState, nil
	}
	return f.effectiveDialogState(), nil
}

func (f *fakeController) SaveProvider(_ context.Context, input app.ProviderConnectionInput) error {
	if f.saveProviderErr != nil {
		return f.saveProviderErr
	}
	f.saveProviderCalls = append(f.saveProviderCalls, input)
	return nil
}

func (f *fakeController) RemoveProvider(_ context.Context, providerID string) error {
	if f.removeProviderErr != nil {
		return f.removeProviderErr
	}
	f.removeProviderCalls = append(f.removeProviderCalls, providerID)
	return nil
}

func (f *fakeController) Close() error { return nil }

func (f *fakeController) effectiveDialogState() app.DialogState {
	if f.dialogStateSet {
		return f.dialogState
	}
	state := f.dialogState
	if modelRouteConfigured(state.ModelRoute) || len(state.ConnectedProviders) > 0 || len(state.AvailableModels) > 0 {
		return state
	}
	state.ModelRoute = provider.ModelRoute{
		Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	}
	state.ConnectedProviders = []app.ConnectedProvider{{
		ProviderID: "openai",
	}}
	state.AvailableModels = []app.AvailableModel{{
		Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
		ProviderName: "OpenAI",
		ModelName:    "GPT-5",
		Capacity:     provider.NormalizeModelCapacity(128000, 128000, 0),
	}}
	return state
}

func TestModelDigitChoiceResolvesSessionPermission(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "list pictures",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "list",
		Path:       externalDir,
		ToolName:   "list",
		Command:    `list {"path":"` + externalDir + `","include_hidden":false}`,
		Reason:     "list directory contents",
	}))

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if next.interaction.resolveReq != "perm-1" {
		t.Fatalf("resolveReq = %q", next.interaction.resolveReq)
	}

	msg := cmd()
	done, ok := msg.(operationDoneMsg)
	if !ok {
		t.Fatalf("cmd() msg = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.resolveCalls) != 1 {
		t.Fatalf("resolve calls = %#v", controller.resolveCalls)
	}
	got := controller.resolveCalls[0]
	if got.Decision != events.PermissionDecisionApproved || got.Scope != events.PermissionScopeSession || got.GrantPath != externalDir {
		t.Fatalf("resolve call = %#v", got)
	}
}

func TestModelDigitChoiceAnswersQuestionFromTranscript(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "investigate failing task routes",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		Question:   "How should I investigate the failing task routes?",
		Options:    []string{"Read tests", "Inspect middleware"},
	}))
	model.busy = true

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	next := updated.(Model)
	if !next.busy {
		t.Fatalf("busy = false, want true")
	}
	if !next.animation.ticking {
		t.Fatal("animTicking = false, want true after answering a question")
	}
	if next.interaction.resolveReq != "q-1" {
		t.Fatalf("resolveReq = %q, want q-1", next.interaction.resolveReq)
	}

	done := operationDoneFromCmd(t, cmd)
	if done.err != nil {
		t.Fatalf("operation err = %v", done.err)
	}
	if len(controller.answerQuestionCalls) != 1 {
		t.Fatalf("answerQuestionCalls = %#v", controller.answerQuestionCalls)
	}
	if got := controller.answerQuestionCalls[0]; got.RequestID != "q-1" || got.Answer != "Inspect middleware" {
		t.Fatalf("answerQuestion call = %#v", got)
	}
}

func TestModelQuestionPromptIgnoresRepeatedInputWhileResolutionInFlight(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "investigate failing task routes",
	})
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeQuestionRequested, "session-1", "turn-1", events.QuestionRequestedPayload{
		QuestionID: "q-1",
		ToolCallID: "call-question-1",
		ToolName:   "question",
		Question:   "How should I investigate the failing task routes?",
		Options:    []string{"Read tests", "Inspect middleware"},
	}))

	model.busy = true
	model.interaction.resolveReq = "q-1"

	updated, cmd := model.Update(tea.KeyPressMsg{Text: "2", Code: '2'})
	next := updated.(Model)
	if !next.busy {
		t.Fatal("busy = false, want true while question resolution is in flight")
	}
	if cmd != nil {
		t.Fatalf("cmd = %#v, want nil while question resolution is in flight", cmd)
	}
	if len(controller.answerQuestionCalls) != 0 {
		t.Fatalf("answerQuestionCalls = %#v, want none", controller.answerQuestionCalls)
	}
}

func TestModelThemeAppliedClearsFooterError(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})
	model.footerNotice.err = "theme updated"

	applied, err := theme.Load("rose-pine-moon")
	if err != nil {
		t.Fatalf("theme.Load() error = %v", err)
	}

	updated, _ := model.Update(themeAppliedMsg{name: "rose-pine-moon", theme: applied})
	next := updated.(Model)
	if next.footerNotice.err != "" {
		t.Fatalf("footerError = %q, want empty", next.footerNotice.err)
	}
	if next.themeName != "rose-pine-moon" {
		t.Fatalf("themeName = %q, want rose-pine-moon", next.themeName)
	}
	if next.theme == nil || next.theme.Name != "rose-pine-moon" {
		t.Fatalf("theme = %#v", next.theme)
	}
}

func TestSetPrimaryModelCmdUsesActiveSessionID(t *testing.T) {
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := setPrimaryModelCmd(ctx, controller, sessionView{
		SessionID:     "session-42",
		WorkspaceRoot: "/repo",
	}, provider.ModelRef{
		ProviderID: "deepseek",
		ModelID:    "deepseek-chat",
	}, 1)
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	if setMsg, ok := msg.(primaryModelSetMsg); !ok || setMsg.err != nil {
		t.Fatalf("cmd() = %#v", msg)
	}
	if len(controller.setPrimaryModelCalls) != 1 {
		t.Fatalf("setPrimaryModelCalls = %#v", controller.setPrimaryModelCalls)
	}
	if controller.setPrimaryModelCalls[0].SessionID != "session-42" {
		t.Fatalf("sessionID = %q, want session-42", controller.setPrimaryModelCalls[0].SessionID)
	}
	if got := controller.setPrimaryModelCalls[0].Model.String(); got != "deepseek/deepseek-chat" {
		t.Fatalf("model = %q, want deepseek/deepseek-chat", got)
	}
}

func TestSetPrimaryModelCmdCreatesSessionWhenMissing(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{
		snapshots: map[string]events.SessionState{
			"session-created": {
				SessionID:     "session-created",
				WorkspaceRoot: "/repo",
				LastSequence:  3,
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := setPrimaryModelCmd(ctx, controller, sessionView{
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	}, provider.ModelRef{
		ProviderID: "deepseek",
		ModelID:    "deepseek-chat",
	}, 1)
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	opened, ok := msg.(sessionOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want sessionOpenedMsg", msg)
	}
	if opened.err != nil {
		t.Fatalf("sessionOpenedMsg.err = %v", opened.err)
	}
	if len(controller.setPrimaryModelCalls) != 1 {
		t.Fatalf("setPrimaryModelCalls = %#v", controller.setPrimaryModelCalls)
	}
	if controller.setPrimaryModelCalls[0].SessionID != "session-created" {
		t.Fatalf("sessionID = %q, want session-created", controller.setPrimaryModelCalls[0].SessionID)
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		WorkspaceRoot: "/repo",
	})
	updated, _ := model.Update(opened)
	final := updated.(Model)
	if final.sessionID != "session-created" {
		t.Fatalf("sessionID = %q, want session-created", final.sessionID)
	}
}

func TestSetUtilityModelCmdPersistsConfiguredUtilityModel(t *testing.T) {
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	cmd := setUtilityModelCmd(ctx, controller, ref)
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	setMsg, ok := msg.(utilityModelSetMsg)
	if !ok || setMsg.err != nil {
		t.Fatalf("cmd() = %#v", msg)
	}
	if len(controller.setUtilityModelCalls) != 1 {
		t.Fatalf("setUtilityModelCalls = %#v", controller.setUtilityModelCalls)
	}
	if got := controller.setUtilityModelCalls[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("model = %q, want openai/gpt-5-mini", got)
	}
	if got := setMsg.state.UtilityModel.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("utility model = %q, want openai/gpt-5-mini", got)
	}
}

func TestSetReviewerModelCmdPersistsConfiguredReviewerModel(t *testing.T) {
	controller := &fakeController{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ref := provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}
	cmd := setReviewerModelCmd(ctx, controller, ref)
	if cmd == nil {
		t.Fatal("cmd = nil")
	}
	msg := cmd()
	setMsg, ok := msg.(reviewerModelSetMsg)
	if !ok || setMsg.err != nil {
		t.Fatalf("cmd() = %#v", msg)
	}
	if len(controller.setReviewerModelCalls) != 1 {
		t.Fatalf("setReviewerModelCalls = %#v", controller.setReviewerModelCalls)
	}
	if got := controller.setReviewerModelCalls[0].Model.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("model = %q, want openai/gpt-5-mini", got)
	}
	if got := setMsg.state.ReviewModelRoute.Primary.String(); got != "openai/gpt-5-mini" {
		t.Fatalf("reviewer model = %q, want openai/gpt-5-mini", got)
	}
}

func TestRunComposerUtilityModelCommandOpensUtilityModelDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5 Mini",
			}},
			UtilityModel: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	invocation := composerCommandInvocation{Command: composerCommand{ID: "utility-model", Name: "/utility-model"}}
	updated, cmd := model.runComposerCommand(invocation)
	if cmd == nil {
		t.Fatal("open utility model cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.kind != commandPaletteUtilityModel {
		t.Fatalf("dialog kind = %v, want utility model", dialog.kind)
	}
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "GPT-5 Mini") {
		t.Fatalf("utility model dialog missing model row\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "Manage Sessions") {
		t.Fatalf("utility model dialog should not render action rows\nrendered:\n%s", rendered)
	}
	if updated.(Model).dialog != nil {
		t.Fatal("dialog should not be pre-opened before dialogOpenedMsg is applied")
	}
}

func TestRunComposerReviewerModelCommandOpensReviewerModelDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5 Mini",
			}},
			ReviewModelRoute: provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	invocation := composerCommandInvocation{Command: composerCommand{ID: "reviewer-model", Name: "/reviewer-model"}}
	updated, cmd := model.runComposerCommand(invocation)
	if cmd == nil {
		t.Fatal("open reviewer model cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.kind != commandPaletteReviewerModel {
		t.Fatalf("dialog kind = %v, want reviewer model", dialog.kind)
	}
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "GPT-5 Mini") {
		t.Fatalf("reviewer model dialog missing model row\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "Manage Sessions") {
		t.Fatalf("reviewer model dialog should not render action rows\nrendered:\n%s", rendered)
	}
	if updated.(Model).dialog != nil {
		t.Fatal("dialog should not be pre-opened before dialogOpenedMsg is applied")
	}
}

func TestPrimaryModelSetMsgRefreshesAvailableModels(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "togetherai", ModelID: "llama-3.3-70b-instruct"},
			},
		},
	}
	initial := events.SessionState{
		SessionID: "session-1",
		Model:     "openai/gpt-5",
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
		InitialState:  &initial,
	})

	updated, _ := model.Update(primaryModelSetMsg{})
	next := updated.(Model)
	if got := next.providerCatalog.defaultModelRoute.Primary.String(); got != "togetherai/llama-3.3-70b-instruct" {
		t.Fatalf("default primary = %q, want togetherai/llama-3.3-70b-instruct", got)
	}
	if got, ok := effectiveSelectedAgentModelRef(next, next.projector.CurrentState()); !ok || got.String() != "openai/gpt-5" {
		t.Fatalf("effective selected model = %q, %t; want openai/gpt-5, true", got.String(), ok)
	}
}

func TestUtilityModelSetMsgShowsFooterActivity(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	updated, _ := model.Update(utilityModelSetMsg{
		state: app.DialogState{
			UtilityModel: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"},
		},
	})
	next := updated.(Model)
	if next.footerNotice.activity == nil || next.footerNotice.activity.text != "Utility model: openai/gpt-5-mini" {
		t.Fatalf("footerActivity = %#v", next.footerNotice.activity)
	}
}

func TestReviewerModelSetMsgShowsFooterActivity(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	updated, _ := model.Update(reviewerModelSetMsg{
		state: app.DialogState{
			ReviewModelRoute: provider.ModelRoute{Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5-mini"}},
		},
	})
	next := updated.(Model)
	if next.footerNotice.activity == nil || next.footerNotice.activity.text != "Reviewer model: openai/gpt-5-mini" {
		t.Fatalf("footerActivity = %#v", next.footerNotice.activity)
	}
}

func TestRefreshAvailableModelsCachesDialogStateModels(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "google", ModelID: "gemini-2.5-pro"},
			},
			AvailableModels: []app.AvailableModel{{
				Ref:       provider.ModelRef{ProviderID: "google", ModelID: "gemini-2.5-pro"},
				ModelName: "Gemini 2.5 Pro",
				Capacity:  provider.NormalizeModelCapacity(1000000, 1000000, 0),
			}},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})
	model.providerCatalog.models = map[string]app.AvailableModel{
		"openai/gpt-5": {
			Ref:      provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			Capacity: provider.NormalizeModelCapacity(128000, 128000, 0),
		},
	}

	if err := model.refreshDialogState(); err != nil {
		t.Fatalf("refreshDialogState() error = %v", err)
	}

	entry, ok := model.providerCatalog.models["google/gemini-2.5-pro"]
	if !ok {
		t.Fatalf("availableModels = %#v, want refreshed selected model entry", model.providerCatalog.models)
	}
	if entry.Capacity.WindowTokens != 1000000 || entry.Capacity.InputTokens != 1000000 {
		t.Fatalf("capacity = %#v, want 1000000 token input/window", entry.Capacity)
	}
}

func TestRefreshAvailableModelsMaterializesExactCurrentModelWhenCatalogMisses(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "github-copilot", ModelID: "gpt-4.1"},
			},
			ConnectedProviders: []app.ConnectedProvider{{
				ProviderID: "github-copilot",
			}},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
	})

	if err := model.refreshDialogState(); err != nil {
		t.Fatalf("refreshDialogState() error = %v", err)
	}

	entry, ok := model.providerCatalog.models["github-copilot/gpt-4.1"]
	if !ok {
		t.Fatalf("availableModels = %#v, want exact current model entry", model.providerCatalog.models)
	}
	if entry.Ref.String() != "github-copilot/gpt-4.1" || entry.ModelName != "gpt-4.1" {
		t.Fatalf("entry = %#v", entry)
	}
	if entry.Capacity.InputTokens != 0 || entry.Capacity.WindowTokens != 0 {
		t.Fatalf("entry capacity = %#v, want zero-capacity exact placeholder", entry.Capacity)
	}
}

func TestCommandPaletteCtrlRRefreshesModelCatalogInPlace(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5",
			}},
		},
	}
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	opened, ok := model.openModelDialog()().(dialogOpenedMsg)
	if !ok {
		t.Fatal("openModelDialog() did not return dialogOpenedMsg")
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	dialog.filter.SetValue("gpt-5")
	dialog.refilter()
	model.dialog = dialog
	model.dialog.SetFrame(120, 40)

	updated := any(model)

	refreshKey := tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}
	updated, cmd := updated.(Model).Update(refreshKey)
	if cmd == nil {
		t.Fatal("refresh request cmd = nil")
	}
	refreshRequested, ok := cmd().(modelCatalogRefreshRequestedMsg)
	if !ok {
		t.Fatalf("refresh request msg = %#v", cmd())
	}
	if refreshRequested.selected.String() != "openai/gpt-5" {
		t.Fatalf("selected = %#v", refreshRequested.selected)
	}

	controller.dialogState = app.DialogState{
		ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
		AvailableModels: []app.AvailableModel{{
			Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
			ProviderName: "OpenAI",
			ModelName:    "GPT-5.4",
		}},
	}

	updated, cmd = updated.(Model).Update(refreshRequested)
	if cmd == nil {
		t.Fatal("refresh dialog cmd = nil")
	}
	refreshed, ok := cmd().(modelCatalogRefreshedMsg)
	if !ok {
		t.Fatalf("refresh cmd() = %#v", cmd())
	}
	updated, _ = updated.(Model).Update(refreshed)

	final := updated.(Model)
	if controller.refreshDialogCalls != 1 {
		t.Fatalf("refreshDialogCalls = %d, want 1", controller.refreshDialogCalls)
	}
	if final.footerNotice.err != "" {
		t.Fatalf("footerError = %q, want empty", final.footerNotice.err)
	}
	dialog, ok = final.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", final.dialog)
	}
	if dialog.kind != commandPaletteModel {
		t.Fatalf("dialog kind = %v, want model", dialog.kind)
	}
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[ model ]", "GPT-5.4"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("refreshed palette missing %q\nrendered:\n%s", needle, rendered)
		}
	}
}

func TestRenderModelViewShowsInlinePermissionPrompt(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	externalDir := filepath.Join(t.TempDir(), "Pictures")
	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "list pictures",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 80})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "listing pictures",
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeToolCallDeclared, "session-1", "turn-1", events.ToolCallDeclaredPayload{
		CallID:   "call-1",
		ToolName: "list",
		Input:    `{"path":"Pictures","include_hidden":false}`,
	}))
	applyModelEvent(t, &model, draftEvent(4, events.TypePermissionRequested, "session-1", "turn-1", events.PermissionRequestedPayload{
		Kind:       events.PermissionRequestKindPath,
		RequestID:  "perm-1",
		ToolCallID: "call-1",
		Access:     "list",
		Path:       externalDir,
		ToolName:   "list",
		Command:    `list {"path":"` + externalDir + `","include_hidden":false}`,
		Reason:     "list directory contents",
	}))

	rendered := renderModelView(model)
	for _, needle := range []string{
		"Permission required",
		"allow once",
		"Pictures",
	} {
		if !containsLine(rendered, needle) {
			t.Fatalf("rendered view missing %q\n%s", needle, rendered)
		}
	}
	for _, forbidden := range []string{
		"Waiting on approval in inspector",
		"Waiting on execution approval in inspector",
	} {
		if containsLine(rendered, forbidden) {
			t.Fatalf("rendered view still contains stale inspector copy %q\n%s", forbidden, rendered)
		}
	}
}

func TestCtrlPOpensUnifiedCommandPalette(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default execution agent"},
			{ID: "engineer", Description: "workflow execution agent"},
		},
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	updated, cmd := model.Update(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl})
	if cmd == nil {
		t.Fatal("open command palette cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("open cmd() = %#v", cmd())
	}
	updated, _ = updated.(Model).Update(opened)

	final := updated.(Model)
	dialog, ok := final.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", final.dialog)
	}
	if dialog.ID() != dialogIDCommandPalette {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDCommandPalette)
	}
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[action ]", "Switch model", "Switch agent", "Switch theme", "Manage Sessions", "Connect provider"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("command palette missing %q\nrendered:\n%s", needle, rendered)
		}
	}
}

func TestSaveProviderCmdRefreshesDialogStateAndCachesModels(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "nvidia"}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "nvidia", ModelID: "llama-3.1-nemotron-ultra-253b-v1"},
				ProviderName: "NVIDIA",
				ModelName:    "Llama 3.1 Nemotron Ultra 253B",
				Capacity:     provider.NormalizeModelCapacity(128000, 128000, 0),
				ToolCalls:    true,
			}},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	cmd := saveProviderCmd(ctx, controller, app.ProviderConnectionInput{
		ProviderID: "nvidia",
		APIKey:     "token",
		BaseURL:    "https://integrate.api.nvidia.com/v1",
	})
	if cmd == nil {
		t.Fatal("save provider cmd = nil")
	}
	msg := cmd()
	refreshed, ok := msg.(modelCatalogRefreshedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want modelCatalogRefreshedMsg", msg)
	}
	updated, _ := model.Update(refreshed)
	final := updated.(Model)

	if len(controller.saveProviderCalls) != 1 {
		t.Fatalf("saveProviderCalls = %d, want 1", len(controller.saveProviderCalls))
	}
	if controller.refreshDialogCalls != 1 {
		t.Fatalf("refreshDialogCalls = %d, want 1", controller.refreshDialogCalls)
	}
	if final.footerNotice.err != "" {
		t.Fatalf("footerError = %q, want empty", final.footerNotice.err)
	}
	if _, ok := final.providerCatalog.models["nvidia/llama-3.1-nemotron-ultra-253b-v1"]; !ok {
		t.Fatalf("availableModels = %#v, want refreshed nvidia model", final.providerCatalog.models)
	}
}

func TestModelCatalogRefreshUpdatesOpenConnectPaletteEntries(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	dialog := newConnectDialog(buildConnectEntries(app.DialogState{
		ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
	}), &defaultTheme)
	dialog.refilter()
	model.dialog = dialog

	updated, _ := model.Update(modelCatalogRefreshedMsg{
		state: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{
				{ProviderID: "openai"},
				{ProviderID: "nvidia", BaseURL: "https://integrate.api.nvidia.com/v1"},
			},
		},
	})
	final := updated.(Model)
	refreshedDialog, ok := final.dialog.(*connectDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *connectDialog", final.dialog)
	}
	rendered := renderTestDialogContentPlain(refreshedDialog)
	if !strings.Contains(rendered, "NVIDIA") {
		t.Fatalf("connect palette missing refreshed provider entry\nrendered:\n%s", rendered)
	}
}

func TestHandleDialogClosedGitHubCopilotAuthRequestOpensAuthDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	_, cmd := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDConnect,
		result: gitHubCopilotAuthDialogRequest{
			BaseURL: "https://api.githubcopilot.com",
		},
	})
	if cmd == nil {
		t.Fatal("cmd = nil, want dialog opener")
	}
	msg := cmd()
	opened, ok := msg.(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogOpenedMsg", msg)
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	if opened.dialog == nil || opened.dialog.ID() != dialogIDGitHubCopilotAuth {
		t.Fatalf("dialog = %#v, want github copilot auth dialog", opened.dialog)
	}
}

func TestHandleDialogClosedOpenAIAuthRequestOpensAuthDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	_, cmd := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDConnect,
		result: openAIAuthDialogRequest{},
	})
	if cmd == nil {
		t.Fatal("cmd = nil, want dialog opener")
	}
	msg := cmd()
	opened, ok := msg.(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogOpenedMsg", msg)
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	if opened.dialog == nil || opened.dialog.ID() != dialogIDOpenAIAuth {
		t.Fatalf("dialog = %#v, want openai auth dialog", opened.dialog)
	}
}

func TestHandleDialogClosedOpenAIAuthResultCachesDialogState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	next, cmd := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDOpenAIAuth,
		result: openAIAuthDialogResult{
			State: app.DialogState{
				ConnectedProviders: []app.ConnectedProvider{{
					ProviderID: "openai",
				}},
			},
		},
	})
	if cmd == nil {
		t.Fatal("cmd = nil, want footer update command")
	}
	final := next.(Model)
	if _, ok := final.providerCatalog.connectedProviders["openai"]; !ok {
		t.Fatalf("connectedProviders = %#v, want openai cached", final.providerCatalog.connectedProviders)
	}
}

func TestHandleDialogClosedGitHubCopilotAuthResultCachesDialogState(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	next, cmd := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDGitHubCopilotAuth,
		result: gitHubCopilotAuthDialogResult{
			State: app.DialogState{
				ConnectedProviders: []app.ConnectedProvider{{
					ProviderID: "github-copilot",
					BaseURL:    "https://api.githubcopilot.com",
				}},
			},
		},
	})
	if cmd == nil {
		t.Fatal("cmd = nil, want footer update command")
	}
	final := next.(Model)
	if _, ok := final.providerCatalog.connectedProviders["github-copilot"]; !ok {
		t.Fatalf("connectedProviders = %#v, want github-copilot cached", final.providerCatalog.connectedProviders)
	}
}

func TestCommandPaletteQueryShowsMatchingAgent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default execution agent"},
			{ID: "engineer", Description: "workflow execution agent"},
		},
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	opened, ok := model.openAgentDialog()().(dialogOpenedMsg)
	if !ok {
		t.Fatal("openAgentDialog() did not return dialogOpenedMsg")
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	dialog.filter.SetValue("engineer")
	dialog.refilter()
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[ agent ]", "engineer", "workflow execution agent"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("agent query render missing %q\nrendered:\n%s", needle, rendered)
		}
	}
}

func TestCommandPaletteQueryShowsDisabledAgentsWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default execution agent"},
			{ID: "engineer", Description: "workflow execution agent"},
		},
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
		},
	}
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{AgentID: "builder"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})

	opened, ok := model.openAgentDialog()().(dialogOpenedMsg)
	if !ok {
		t.Fatal("openAgentDialog() did not return dialogOpenedMsg")
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	dialog.filter.SetValue("engineer")
	dialog.refilter()
	dialog.SetFrame(140, 40)
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[ agent ]", "engineer", "workflow execution agent", "locked while turn ru"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("running turn palette missing disabled agent result %q\nrendered:\n%s", needle, rendered)
		}
	}
	if _, cmd := dialog.activateListSelection(); cmd != nil {
		t.Fatalf("disabled agent selection should not close the dialog")
	}
}

func TestCommandPaletteQueryShowsDisabledModelsWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5.4",
				Capacity:     provider.NormalizeModelCapacity(128000, 64000, 0),
				CostInput:    1.25,
				CostOutput:   10,
				Reasoning:    true,
				ToolCalls:    true,
				Vision:       true,
			}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
			},
		},
	}
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{AgentID: "builder"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})

	opened, ok := model.openModelDialog()().(dialogOpenedMsg)
	if !ok {
		t.Fatal("openModelDialog() did not return dialogOpenedMsg")
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	dialog.filter.SetValue("gpt-5.4")
	dialog.refilter()
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[ model ]", "OpenAI", "GPT-5.4", "locked while turn runs"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("running turn palette missing disabled model result %q\nrendered:\n%s", needle, rendered)
		}
	}
}

func TestCommandPaletteQueryShowsDisabledSessionsWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
		},
		sessions: []app.SessionSummary{{
			ID:    "session-2",
			Title: "Previous session",
		}},
	}
	state := events.SessionState{
		SessionID:     "session-1",
		WorkspaceRoot: "/repo",
		TurnOrder:     []string{"turn-1"},
		Turns: map[string]*events.TurnState{
			"turn-1": {
				TurnID: "turn-1",
				Status: events.TurnStatusRunning,
				Config: &events.TurnConfigState{AgentID: "builder"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState:  &state,
	})

	opened, ok := model.openCommandPaletteWithQuery("manage sessions")().(dialogOpenedMsg)
	if !ok {
		t.Fatal("openCommandPaletteWithQuery() did not return dialogOpenedMsg")
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[action ]", "Manage Sessions", "locked while turn runs"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("running turn palette missing disabled session result %q\nrendered:\n%s", needle, rendered)
		}
	}
}

func TestCommandPaletteQueryShowsMatchingModelWithoutDuplicateRef(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5.4",
				Capacity:     provider.NormalizeModelCapacity(128000, 64000, 0),
				CostInput:    1.25,
				CostOutput:   10,
				Reasoning:    true,
				ToolCalls:    true,
				Vision:       true,
			}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
			},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	opened, ok := model.openModelDialog()().(dialogOpenedMsg)
	if !ok {
		t.Fatal("openModelDialog() did not return dialogOpenedMsg")
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	dialog.filter.SetValue("gpt-5.4")
	dialog.refilter()
	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"[ model ]", "Provider", "Model", "Input", "Window", "$/M", "Caps", "OpenAI", "GPT-5.4", "64k", "128k", "$1.2/$10", "R T V"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("model query render missing %q\nrendered:\n%s", needle, rendered)
		}
	}
	if strings.Contains(rendered, "openai/gpt-5.4") {
		t.Fatalf("model query render still shows duplicate raw ref\nrendered:\n%s", rendered)
	}
}

func TestCommandPaletteModelQueryShowsCapabilitiesLikeKoda(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5.4"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5.4",
				Capacity:     provider.NormalizeModelCapacity(128000, 128000, 0),
				CostInput:    1.25,
				CostOutput:   10,
				Reasoning:    true,
				ToolCalls:    true,
				Vision:       true,
			}},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	opened, ok := model.openModelDialog()().(dialogOpenedMsg)
	if !ok {
		t.Fatal("openModelDialog() did not return dialogOpenedMsg")
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	dialog.filter.SetValue("gpt-5.4")
	dialog.refilter()
	dialog.SetFrame(120, 40)

	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{"Provider", "Model", "Input", "Window", "$/M", "Caps", "OpenAI", "GPT-5.4", "128k", "$1.2/$10", "R T V"} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("model dialog missing %q\nrendered:\n%s", needle, rendered)
		}
	}
}

func TestSessionsPurgeDialogArrowsNavigateOptionsNotButtons(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	dialog := newSessionsDialog([]sessionItem{
		{ID: "old-1", UpdatedAt: time.Now().Add(-10 * 24 * time.Hour).Unix()},
		{ID: "old-2", UpdatedAt: time.Now().Add(-40 * 24 * time.Hour).Unix()},
		{ID: "old-3", UpdatedAt: time.Now().Add(-200 * 24 * time.Hour).Unix()},
	}, &defaultTheme)

	if _, cmd := dialog.handleShortcutPurge(); cmd != nil {
		cmd()
	}
	if dialog.mode != sessionsDialogPurge {
		t.Fatalf("dialog mode = %v, want purge", dialog.mode)
	}
	if dialog.focusedButtonIndex() != -1 {
		t.Fatalf("purge dialog should open with option focus, got button index %d", dialog.focusedButtonIndex())
	}

	dialog.handleDown()
	if dialog.purgeCursor != 1 {
		t.Fatalf("purge cursor = %d, want 1", dialog.purgeCursor)
	}
	if dialog.focusedButtonIndex() != -1 {
		t.Fatalf("down should keep focus on purge options, got button index %d", dialog.focusedButtonIndex())
	}

	if cmd := dialog.moveFocus(1); cmd != nil {
		cmd()
	}
	if dialog.focusedButtonIndex() != 0 {
		t.Fatalf("tab should move focus to first button, got %d", dialog.focusedButtonIndex())
	}

	dialog.handleDown()
	if dialog.purgeCursor != 2 {
		t.Fatalf("purge cursor after button-focused down = %d, want 2", dialog.purgeCursor)
	}
	if dialog.focusedButtonIndex() != -1 {
		t.Fatalf("down should leave button row and return to purge options, got button index %d", dialog.focusedButtonIndex())
	}
}

func TestSessionsDialogViewOmitsDeleteAndPurgeShortcutHints(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	dialog := newSessionsDialog([]sessionItem{
		{ID: "one", Title: "Intro to Coding: Getting Started", UpdatedAt: time.Now().Add(-26 * time.Minute).Unix()},
	}, &defaultTheme)
	dialog.SetFrame(160, 40)

	rendered := renderTestDialogContentPlain(dialog)
	if strings.Contains(rendered, "ctrl+d delete") || strings.Contains(rendered, "ctrl+p purge") {
		t.Fatalf("sessions dialog still shows delete/purge shortcuts\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, "↑/↓ select • tab buttons • enter confirm • esc back") {
		t.Fatalf("sessions dialog missing updated footer hint\nrendered:\n%s", rendered)
	}
}

func TestSessionsDialogWidthFitsButtonsAndHint(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	dialog := newSessionsDialog([]sessionItem{
		{ID: "one", Title: "Intro to Coding: Getting Started", UpdatedAt: time.Now().Add(-26 * time.Minute).Unix()},
	}, &defaultTheme)
	dialog.SetFrame(200, 40)

	rendered := renderTestDialogContentPlain(dialog)
	for _, needle := range []string{
		"[ Open ]  [ New ]  [ Delete ]  [ Purge ]  [ Cancel ]",
		"↑/↓ select • tab buttons • enter confirm • esc back",
	} {
		if !strings.Contains(rendered, needle) {
			t.Fatalf("sessions dialog clipped %q\nrendered:\n%s", needle, rendered)
		}
	}
}

func TestPurgeSessionsAndReopenDialogCmdSkipsCurrentSession(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	controller := &fakeController{
		sessions: []app.SessionSummary{
			{ID: "session-current", Title: "Current", UpdatedAt: time.Now()},
			{ID: "session-a", Title: "A", UpdatedAt: time.Now().Add(-time.Hour)},
			{ID: "session-b", Title: "B", UpdatedAt: time.Now().Add(-2 * time.Hour)},
		},
	}

	cmd := purgeSessionsAndReopenDialogCmd(
		context.Background(),
		controller,
		"session-current",
		[]string{"session-current", "session-a", "session-b"},
		&defaultTheme,
		120,
		40,
	)
	if cmd == nil {
		t.Fatal("purgeSessionsAndReopenDialogCmd() = nil")
	}
	msg := cmd()
	opened, ok := msg.(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogOpenedMsg", msg)
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	if got, want := controller.deleteSessionCalls, []string{"session-a", "session-b"}; !slices.Equal(got, want) {
		t.Fatalf("deleteSessionCalls = %#v, want %#v", got, want)
	}
	dialog, ok := opened.dialog.(*sessionsDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *sessionsDialog", opened.dialog)
	}
	if len(dialog.sessionItems) != 2 {
		t.Fatalf("session item count = %d, want 2", len(dialog.sessionItems))
	}
	for _, item := range dialog.sessionItems {
		if item.ID == "session-current" {
			t.Fatalf("dialog included current session after purge: %#v", dialog.sessionItems)
		}
	}
}

func TestModelSubmitComposerStartsTurnWithSelectedAgent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.agentID = "engineer"
	model.composer.SetValue("Improve middleware layer")

	updated, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", msg)
	}
	for _, subcmd := range batch {
		if subcmd == nil {
			continue
		}
		if done, ok := subcmd().(operationDoneMsg); ok && done.err != nil {
			t.Fatalf("operation err = %v", done.err)
		}
	}
	if len(controller.startCalls) != 1 {
		t.Fatalf("start calls = %#v", controller.startCalls)
	}
	if got := controller.startCalls[0].AgentID; got != "engineer" {
		t.Fatalf("start agent = %q, want engineer", got)
	}
	if next := updated.(Model); next.agentID != "engineer" {
		t.Fatalf("model agent = %q, want engineer", next.agentID)
	}
}

func TestModelInitOpensConnectDialogWhenNoProvidersConfigured(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogStateSet: true,
		dialogState:    app.DialogState{},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		WorkspaceRoot: "/repo",
	})

	cmd := model.Init()
	if cmd == nil {
		t.Fatal("Init() cmd = nil")
	}
	msg := cmd()
	var opened dialogOpenedMsg
	switch typed := msg.(type) {
	case dialogOpenedMsg:
		opened = typed
	case tea.BatchMsg:
		found := false
		for _, subcmd := range typed {
			if subcmd == nil {
				continue
			}
			if candidate, ok := subcmd().(dialogOpenedMsg); ok {
				opened = candidate
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("Init() batch missing dialogOpenedMsg: %#v", typed)
		}
	default:
		t.Fatalf("Init() msg = %#v", msg)
	}
	dialog, ok := opened.dialog.(*connectDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.ID() != dialogIDConnect {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDConnect)
	}
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "Anthropic") {
		t.Fatalf("startup dialog missing provider entry:\n%s", rendered)
	}
}

func TestSubmitComposerOpensConnectDialogWhenNoProvidersConfigured(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogStateSet: true,
		dialogState:    app.DialogState{},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.composer.SetValue("hello")

	updated, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want none", controller.startCalls)
	}
	if updated.(Model).busy {
		t.Fatal("busy = true, want false")
	}
	msg := cmd()
	opened, ok := msg.(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogOpenedMsg", msg)
	}
	dialog, ok := opened.dialog.(*connectDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.ID() != dialogIDConnect {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDConnect)
	}
}

func TestSubmitComposerOpensModelDialogWhenProviderConfiguredButNoModelSelected(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogStateSet: true,
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			AvailableModels: []app.AvailableModel{{
				Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
				ProviderName: "OpenAI",
				ModelName:    "GPT-5",
			}},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	model.composer.SetValue("hello")

	updated, cmd := model.submitComposer()
	if cmd == nil {
		t.Fatal("submitComposer cmd = nil")
	}
	if len(controller.startCalls) != 0 {
		t.Fatalf("startCalls = %#v, want none", controller.startCalls)
	}
	if updated.(Model).busy {
		t.Fatal("busy = true, want false")
	}
	msg := cmd()
	opened, ok := msg.(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v, want dialogOpenedMsg", msg)
	}
	dialog, ok := opened.dialog.(*commandPaletteDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	rendered := renderTestDialogContentPlain(dialog)
	if !strings.Contains(rendered, "[ model ]") || !strings.Contains(rendered, "GPT-5") {
		t.Fatalf("model dialog missing model section:\n%s", rendered)
	}
}

func TestCommandPaletteAgentSelectionRefreshesInspectorDetailsAgent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default agent"},
			{ID: "engineer", Description: "workflow execution agent"},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
		InitialState: &events.SessionState{
			SessionID:     "session-1",
			WorkspaceRoot: "/repo",
			TurnOrder:     []string{"turn-0", "turn-1"},
			Turns: map[string]*events.TurnState{
				"turn-0": {
					TurnID: "turn-0",
					Status: events.TurnStatusCompleted,
					Config: &events.TurnConfigState{
						AgentID: "builder",
					},
				},
				"turn-1": {
					TurnID: "turn-1",
					Status: events.TurnStatusCompleted,
					Config: &events.TurnConfigState{
						AgentID: "builder",
					},
				},
			},
		},
	})
	model.selection.detailTurnID = "turn-0"
	model.selection.handoffID = "handoff-1"
	model.selection.callTurnID = "turn-0"
	model.selection.callID = "call-1"

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	updated, _ := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDCommandPalette,
		result: agentItem{ID: "engineer"},
	})
	next := updated.(Model)

	if next.agentID != "engineer" {
		t.Fatalf("agentID = %q, want engineer", next.agentID)
	}
	if next.selection.detailTurnID != next.turnID {
		t.Fatalf("detailTurnID = %q, want %q", next.selection.detailTurnID, next.turnID)
	}
	if next.selection.handoffID != "" {
		t.Fatalf("selectedHandoffID = %q, want cleared", next.selection.handoffID)
	}
	if next.selection.callTurnID != "" || next.selection.callID != "" {
		t.Fatalf("selected call = %q/%q, want cleared", next.selection.callTurnID, next.selection.callID)
	}
	rendered := ansi.Strip(next.inspector.body.View())
	if !strings.Contains(rendered, "engineer") {
		t.Fatalf("inspector details missing updated agent\nrendered:\n%s", rendered)
	}
	if strings.Contains(rendered, "builder") {
		t.Fatalf("inspector details should not show stale agent\nrendered:\n%s", rendered)
	}
}

func TestCommandPaletteAgentSelectionIgnoredWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		agents: []app.AvailableAgent{
			{ID: "builder", Description: "default agent"},
			{ID: "engineer", Description: "workflow execution agent"},
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
		InitialState: &events.SessionState{
			SessionID:     "session-1",
			WorkspaceRoot: "/repo",
			TurnOrder:     []string{"turn-0", "turn-1"},
			Turns: map[string]*events.TurnState{
				"turn-0": {
					TurnID: "turn-0",
					Status: events.TurnStatusCompleted,
					Config: &events.TurnConfigState{
						AgentID: "builder",
					},
				},
				"turn-1": {
					TurnID: "turn-1",
					Status: events.TurnStatusRunning,
					Config: &events.TurnConfigState{
						AgentID: "builder",
					},
				},
			},
		},
	})
	model.selection.detailTurnID = "turn-0"
	model.selection.handoffID = "handoff-1"
	model.selection.callTurnID = "turn-0"
	model.selection.callID = "call-1"

	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	model = modelIface.(Model)

	updated, _ := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDCommandPalette,
		result: agentItem{ID: "engineer"},
	})
	next := updated.(Model)

	if next.agentID != "builder" {
		t.Fatalf("agentID = %q, want builder while turn runs", next.agentID)
	}
	if next.selection.detailTurnID != "turn-0" {
		t.Fatalf("detailTurnID = %q, want preserved while turn runs", next.selection.detailTurnID)
	}
	if next.selection.handoffID != "handoff-1" {
		t.Fatalf("selectedHandoffID = %q, want preserved while turn runs", next.selection.handoffID)
	}
	if next.selection.callTurnID != "turn-0" || next.selection.callID != "call-1" {
		t.Fatalf("selected call = %q/%q, want preserved while turn runs", next.selection.callTurnID, next.selection.callID)
	}
}

func TestCommandPaletteModelSelectionIgnoredWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState: &events.SessionState{
			SessionID:     "session-1",
			WorkspaceRoot: "/repo",
			TurnOrder:     []string{"turn-1"},
			Turns: map[string]*events.TurnState{
				"turn-1": {
					TurnID: "turn-1",
					Status: events.TurnStatusRunning,
				},
			},
		},
	})

	updated, cmd := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDCommandPalette,
		result: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
	})
	if cmd != nil {
		t.Fatal("disabled model selection should not produce a command")
	}
	if updated.(Model).busy {
		t.Fatal("busy = true, want false")
	}
}

func TestCommandPaletteSessionSelectionIgnoredWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState: &events.SessionState{
			SessionID:     "session-1",
			WorkspaceRoot: "/repo",
			TurnOrder:     []string{"turn-1"},
			Turns: map[string]*events.TurnState{
				"turn-1": {
					TurnID: "turn-1",
					Status: events.TurnStatusRunning,
				},
			},
		},
	})

	updated, cmd := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDCommandPalette,
		result: sessionsDialogResult{
			OpenSessionID: "session-2",
		},
	})
	if cmd != nil {
		t.Fatal("disabled session selection should not produce a command")
	}
	if updated.(Model).busy {
		t.Fatal("busy = true, want false")
	}
}

func TestCommandPaletteManageSessionsIgnoredWhileCurrentTurnRunning(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		InitialState: &events.SessionState{
			SessionID:     "session-1",
			WorkspaceRoot: "/repo",
			TurnOrder:     []string{"turn-1"},
			Turns: map[string]*events.TurnState{
				"turn-1": {
					TurnID: "turn-1",
					Status: events.TurnStatusRunning,
				},
			},
		},
	})

	updated, cmd := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDCommandPalette,
		result: commandPaletteActionResult{ActionID: "manage-sessions"},
	})
	if cmd != nil {
		t.Fatal("disabled manage sessions action should not produce a command")
	}
	if updated.(Model).dialog != nil {
		t.Fatal("dialog should remain closed")
	}
}

func TestCommandPaletteManageSessionsOpensSessionsDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		dialogState: app.DialogState{
			ConnectedProviders: []app.ConnectedProvider{{ProviderID: "openai"}},
			ModelRoute: provider.ModelRoute{
				Primary: provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			},
		},
		sessions: []app.SessionSummary{{
			ID:    "session-2",
			Title: "Previous session",
		}},
	}

	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      "hello",
	})

	next, cmd := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDCommandPalette,
		result: commandPaletteActionResult{ActionID: "manage-sessions"},
	})
	if cmd == nil {
		t.Fatal("open sessions cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	dialog, ok := opened.dialog.(*sessionsDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.ID() != dialogIDSessions {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDSessions)
	}
	if next.(Model).dialog != nil {
		t.Fatal("palette dialog should be cleared before opening sessions dialog")
	}
}

func TestSkillsDialogResultUpdatesSelectedSkills(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		AgentID:       "builder",
		SkillIDs:      []string{"review"},
	})

	updated, _ := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDSkills,
		result: skillsDialogResult{
			SkillIDs: []string{"review", "search"},
		},
	})
	next := updated.(Model)

	if got := next.skillIDs; len(got) != 2 || got[0] != "review" || got[1] != "search" {
		t.Fatalf("skillIDs = %#v", got)
	}
	if next.footerNotice.activity == nil || next.footerNotice.activity.text != "Skills: review, search" {
		t.Fatalf("footerActivity = %#v", next.footerNotice.activity)
	}
}

func TestCommandPaletteActionOpensTrustDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		workspaceTrust: app.WorkspaceTrustState{
			WorkspaceRoot: "/repo",
			Trusted:       true,
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	next, cmd := model.handleDialogClosed(dialogClosedMsg{
		id:     dialogIDCommandPalette,
		result: commandPaletteActionResult{ActionID: "manage-trust"},
	})
	if cmd == nil {
		t.Fatal("open trust cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	dialog, ok := opened.dialog.(*trustDialog)
	if !ok {
		t.Fatalf("dialog = %#v", opened.dialog)
	}
	if dialog.ID() != dialogIDTrust {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDTrust)
	}
	if next.(Model).dialog != nil {
		t.Fatal("palette dialog should be cleared before opening trust dialog")
	}
}

func TestTrustDialogResultRevokesTrustAndReopensDialog(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	controller := &fakeController{
		workspaceTrust: app.WorkspaceTrustState{
			WorkspaceRoot: "/repo",
			Trusted:       false,
		},
	}
	model := NewModel(controller, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})

	next, cmd := model.handleDialogClosed(dialogClosedMsg{
		id: dialogIDTrust,
		result: trustDialogResult{
			Scope:       app.RevokeTrustScopeServer,
			Fingerprint: "server-a",
		},
	})
	if cmd == nil {
		t.Fatal("revoke trust cmd = nil")
	}
	opened, ok := cmd().(dialogOpenedMsg)
	if !ok {
		t.Fatalf("cmd() = %#v", cmd())
	}
	if opened.err != nil {
		t.Fatalf("dialogOpenedMsg.err = %v", opened.err)
	}
	dialog, ok := opened.dialog.(*trustDialog)
	if !ok {
		t.Fatalf("dialog = %#v, want *trustDialog", opened.dialog)
	}
	if dialog.ID() != dialogIDTrust {
		t.Fatalf("dialog id = %q, want %q", dialog.ID(), dialogIDTrust)
	}
	if len(controller.revokeTrustCalls) != 1 {
		t.Fatalf("revokeTrustCalls = %#v, want one call", controller.revokeTrustCalls)
	}
	call := controller.revokeTrustCalls[0]
	if call.SessionID != "session-1" || call.WorkspaceRoot != "/repo" || call.Scope != app.RevokeTrustScopeServer || call.Fingerprint != "server-a" {
		t.Fatalf("revokeTrustCalls[0] = %#v", call)
	}
	if next.(Model).dialog != nil {
		t.Fatal("trust dialog should be cleared before reopening")
	}
}

func TestRenderModelViewFillsWindowSize(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	rendered := renderModelView(model)
	if got := lipgloss.Width(rendered); got != 120 {
		t.Fatalf("rendered width = %d, want 120", got)
	}
	if got := lipgloss.Height(rendered); got != 40 {
		t.Fatalf("rendered height = %d, want 40", got)
	}

	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if got := ansi.StringWidth(ansi.Strip(line)); got != 120 {
			t.Fatalf("line %d width = %d, want 120\n%q", i, got, ansi.Strip(line))
		}
	}
}

func TestRenderModelViewFillsWindowSizeWithPaneContent(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
		AgentID:      "builder",
		Model:        "openai/gpt-5.4",
		AllowedTools: []string{"read"},
	}))
	applyModelEvent(t, &model, draftEvent(2, events.TypeAssistantPreviewDelta, "session-1", "turn-1", events.AssistantPreviewDeltaPayload{
		Content: "Hi.",
	}))
	applyModelEvent(t, &model, draftEvent(3, events.TypeTurnDone, "session-1", "turn-1", events.TurnDonePayload{}))

	rendered := renderModelView(model)
	lines := strings.Split(rendered, "\n")
	for i, line := range lines {
		if got := ansi.StringWidth(ansi.Strip(line)); got != 120 {
			t.Fatalf("line %d width = %d, want 120\n%q", i, got, ansi.Strip(line))
		}
	}
}

func TestPlaceBlockStylesWhitespaceWithBackground(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	bg := toneValue(model.theme, toneBGAlt)
	rendered := placeBlock(20, 3, bg, "hello")
	if got := lipgloss.Width(rendered); got != 20 {
		t.Fatalf("placeBlock width = %d, want 20", got)
	}
	if got := lipgloss.Height(rendered); got != 3 {
		t.Fatalf("placeBlock height = %d, want 3", got)
	}

	bgANSI := backgroundANSI(bg)
	lines := strings.Split(rendered, "\n")
	for _, idx := range []int{0, 2} {
		if !strings.Contains(lines[idx], bgANSI) {
			t.Fatalf("line %d missing whitespace background", idx)
		}
	}
}

func TestPlaceBlockAllowsTransparentWhitespace(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	rendered := placeBlock(20, 3, "", "hello")
	if got := lipgloss.Width(rendered); got != 20 {
		t.Fatalf("placeBlock width = %d, want 20", got)
	}
	if got := lipgloss.Height(rendered); got != 3 {
		t.Fatalf("placeBlock height = %d, want 3", got)
	}

	bgANSI := backgroundANSI(toneValue(model.theme, toneBG))
	lines := strings.Split(rendered, "\n")
	for _, idx := range []int{0, 2} {
		if strings.Contains(lines[idx], bgANSI) {
			t.Fatalf("line %d unexpectedly contains shell background", idx)
		}
	}
}

func TestShellColumnPersistsBackgroundForTextLines(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	bg := toneValue(model.theme, tonePanel)
	rendered := renderToneBlock(model.theme, tonePanel, 20, 3, "hello\nworld")
	lines := strings.Split(rendered, "\n")
	bgANSI := backgroundANSI(bg)

	for _, idx := range []int{0, 1, 2} {
		if !strings.Contains(lines[idx], bgANSI) {
			t.Fatalf("line %d missing column background", idx)
		}
	}
}

func TestRenderDialogBoxDoesNotPaintFullSurfaceBackground(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Tones.BGAlt = "#112233"

	dialog := &framedStaticDialog{
		id:    dialogIDCommandPalette,
		theme: &customTheme,
		width: 32,
		content: renderStandaloneDialogContent(&customTheme, max(32-dialogFrameInset*2, 1), dialogStandaloneFrame{
			Body: strings.Join([]string{
				dialogTitleStyle(&customTheme).Render("Select Theme"),
				"",
				dialogHintStyle(&customTheme).Render("type to filter"),
			}, "\n"),
		}),
	}
	rendered := renderTestDialogContent(dialog)

	bgANSI := backgroundANSI(dialogSurfaceTone(&customTheme))
	if strings.Contains(rendered, bgANSI) {
		t.Fatalf("dialog unexpectedly painted full dialog background\nrendered:\n%s", rendered)
	}
	stripped := ansi.Strip(rendered)
	if !strings.Contains(stripped, "Select Theme") || !strings.Contains(stripped, "type to filter") {
		t.Fatalf("dialog missing content\nrendered:\n%s", stripped)
	}
}

func TestRenderPaletteDialogBoxShowsPromptHeaderAndDivider(t *testing.T) {
	customTheme := theme.StaticDefault()
	rendered := ansi.Strip(renderPaletteDialogContentSized(&customTheme, max(32-dialogFrameInset*2, 1), dialogPaletteFrame{
		Prompt: "switch model",
		Body:   "  one\n  two",
		Hint:   "enter select",
	}, 0))

	if !strings.Contains(rendered, "❯ switch model") {
		t.Fatalf("palette dialog missing prompt header\nrendered:\n%s", rendered)
	}
	if !strings.Contains(rendered, strings.Repeat("─", 8)) {
		t.Fatalf("palette dialog missing divider\nrendered:\n%s", rendered)
	}
}

func TestCommandPaletteViewDoesNotPaintFullDialogBackground(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Tones.BGAlt = "#112233"

	dialog := newCommandPaletteActions(commandPaletteActionsItems{
		ModelItems: []modelItem{{
			Ref:          provider.ModelRef{ProviderID: "openai", ModelID: "gpt-5"},
			ProviderName: "OpenAI",
			ModelName:    "GPT-5",
		}},
	}, &customTheme)
	dialog.SetFrame(120, 40)

	rendered := renderTestDialogContent(dialog)
	if strings.Contains(rendered, backgroundANSI(dialogSurfaceTone(&customTheme))) {
		t.Fatalf("palette unexpectedly painted full dialog background\nrendered:\n%s", rendered)
	}

	stripped := ansi.Strip(rendered)
	for _, want := range []string{
		"Manage Sessions",
		"Connect provider",
		"↑/↓ navigate",
	} {
		if !strings.Contains(stripped, want) {
			t.Fatalf("palette missing %q\nrendered:\n%s", want, stripped)
		}
	}
}

func TestThemeDialogViewPaintsFieldBackground(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Tones.BGAlt = "#112233"
	customTheme.Tones.PanelAlt = "#223344"

	dialog := newThemeDialog([]themeItem{{Name: "ayu-dark", DisplayName: "ayu-dark"}}, "ayu-dark", &customTheme)
	styles := dialog.filter.Styles()

	if got := styles.Focused.Text.GetBackground(); got == nil {
		t.Fatalf("focused text style missing field background")
	}
	if got := styles.Focused.Placeholder.GetBackground(); got == nil {
		t.Fatalf("focused placeholder style missing field background")
	}
}

func TestRenderMainShellUsesPanelToneForSideColumns(t *testing.T) {
	customTheme := theme.StaticDefault()
	customTheme.Name = "panel-proof"
	customTheme.Tones.BGAlt = "#010203"
	customTheme.Tones.Panel = "#123456"
	customTheme.Tones.PanelAlt = "#234567"
	customTheme.Tones.Line = "#345678"
	customTheme.Tones.LineStrong = "#456789"

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &customTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)
	applyModelEvent(t, &model, draftEvent(0, events.TypeSessionConfigured, "session-1", "turn-1", events.SessionConfiguredPayload{
		WorkspaceRoot: "/repo",
	}))
	applyModelEvent(t, &model, draftEvent(1, events.TypeTurnConfigured, "session-1", "turn-1", events.TurnConfiguredPayload{
		AgentID: "builder",
		Model:   "openai/gpt-5.2",
	}))

	state := model.projector.Snapshot()
	layout := resolveShellLayout(model, state)
	rendered := renderMainShell(model, state, layout)

	if !strings.Contains(rendered, backgroundANSI(customTheme.Tones.PanelAlt)) {
		t.Fatalf("renderMainShell missing side panel background %q", customTheme.Tones.PanelAlt)
	}
}

func TestModelViewDisablesMouseCaptureForNativeSelection(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	view := model.View()
	if !view.AltScreen {
		t.Fatal("view.AltScreen = false, want true")
	}
}

func TestModelViewSetsBackgroundColorFromShellTone(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	view := model.View()
	want := lipgloss.Color(toneValue(model.theme, toneBG))
	if !reflect.DeepEqual(view.BackgroundColor, want) {
		t.Fatalf("view.BackgroundColor = %#v, want %#v", view.BackgroundColor, want)
	}
}

func TestModelViewSetsComposerCursorWhenFocused(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	model.chrome.focus = focusComposer
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	view := model.View()
	if view.Cursor == nil {
		t.Fatal("view.Cursor = nil, want composer cursor")
	}
}

func TestModelViewSetsWindowTitleFromSessionTitle(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Title:     `**"Optimizing Your Project’s Performance"**`,
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	view := model.View()
	if got := view.WindowTitle; got != "KC | Optimizing Your Projects Performance" {
		t.Fatalf("view.WindowTitle = %q, want %q", got, "KC | Optimizing Your Projects Performance")
	}
}

func TestModelViewUsesDefaultWindowTitleWhenSessionTitleMissing(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	view := model.View()
	if got := view.WindowTitle; got != "KC | Workspace session" {
		t.Fatalf("view.WindowTitle = %q, want %q", got, "KC | Workspace session")
	}
}

func TestModelViewClearsWindowTitleDuringShutdown(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	model.projector = events.NewProjectorFromSnapshot(events.SessionState{
		SessionID: "session-1",
		Title:     "Cache review",
	})
	model.shuttingDown = true
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	view := model.View()
	if got := view.WindowTitle; got != "" {
		t.Fatalf("view.WindowTitle = %q, want empty during shutdown", got)
	}
}

func TestRenderTranscriptBlockIsTextOnly(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	bg := toneValue(model.theme, tonePanelAlt)
	rendered := renderTranscriptBlock(model, "You • turn request", ".", 80, transcriptBlockStyle{
		alignRight: true,
		accent:     colorFor(model.theme, "primary", "#7cc7ff"),
	})

	if strings.Contains(rendered, backgroundANSI(bg)) {
		t.Fatalf("renderTranscriptBlock unexpectedly contains panel background")
	}
	stripped := ansi.Strip(rendered)
	for _, border := range []string{"┌", "┐", "└", "┘", "│", "─"} {
		if strings.Contains(stripped, border) {
			t.Fatalf("renderTranscriptBlock unexpectedly contains border %q in %q", border, stripped)
		}
	}
}

func TestShellColumnDividerDoesNotPaintShellBackground(t *testing.T) {
	defaultTheme := theme.StaticDefault()
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	model := NewModel(&fakeController{}, ModelConfig{
		Context:       ctx,
		Theme:         &defaultTheme,
		SessionID:     "session-1",
		TurnID:        "turn-1",
		WorkspaceRoot: "/repo",
		UserText:      ".",
	})
	modelIface, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = modelIface.(Model)

	rendered := shellColumnDivider(model, 3)
	bgANSI := backgroundANSI(toneValue(model.theme, toneBG))
	if strings.Contains(rendered, bgANSI) {
		t.Fatalf("divider unexpectedly contains shell background")
	}
}

func draftEvent(sequence int64, eventType events.Type, sessionID, turnID string, payload events.Payload) events.Event {
	return events.Event{
		ID:        "event-id",
		SessionID: sessionID,
		TurnID:    turnID,
		Sequence:  sequence,
		Time:      time.Unix(sequence+1, 0).UTC(),
		Type:      eventType,
		Payload:   payload,
	}
}

func applyModelEvent(t *testing.T, model *Model, event events.Event) {
	t.Helper()
	if model.width == 0 || model.height == 0 {
		model.width = 140
		model.height = 80
	}
	if err := model.projector.Apply(event); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	model.refreshTranscriptTurnSourceKeysForBatch(model.projector.Snapshot(), []events.Event{event})
	model.syncFocusState()
	model.syncViewportLayout()
}

func containsLine(text, needle string) bool {
	normalizedText := strings.Join(strings.Fields(ansi.Strip(text)), " ")
	normalizedNeedle := strings.Join(strings.Fields(needle), " ")
	return strings.Contains(normalizedText, normalizedNeedle)
}

func cloneBoolMap(input map[string]bool) map[string]bool {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]bool, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}
