package tui

import (
	"context"

	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
)

type sessionOpenedMsg struct {
	view                  sessionView
	state                 events.SessionState
	stateOwned            bool
	stream                <-chan events.Event
	cancel                context.CancelFunc
	watchID               int
	startTurn             bool
	startTurnAgentID      string
	startTurnWorkflowID   string
	startReview           bool
	reviewInstructions    string
	reviewThinkingEnabled bool
	reviewThinkingMode    string
	reviewSkillIDs        []string
	attachments           []app.AttachmentInput
	localShellCommand     string
	err                   error
}

type sessionWatchOpenedMsg struct {
	stream    <-chan events.Event
	cancel    context.CancelFunc
	watchID   int
	startTurn bool
	err       error
}

type watchEventMsg struct {
	event events.Event
	open  bool
	id    int
}

type watchEventsMsg struct {
	events []events.Event
	closed bool
	id     int
}

type toolResultLoadedMsg struct {
	sessionID string
	ref       sessionToolCallRef
	result    app.ToolResultDetail
	err       error
}

type toolMutationDetailLoadedMsg struct {
	sessionID string
	ref       sessionToolCallRef
	detail    loadedToolMutationDetail
	err       error
}

type workspaceStatusLoadedMsg struct {
	sessionID string
	status    app.WorkspaceStatus
	err       error
}

type budgetStatusLoadedMsg struct {
	sessionID string
	status    app.BudgetStatus
	err       error
}

type sessionUsageSummaryLoadedMsg struct {
	sessionID string
	summary   app.SessionUsageSummary
	err       error
}

type promptHistoryLoadedMsg struct {
	entries []app.PromptHistoryEntry
	err     error
}

type composerSkillsLoadedMsg struct {
	skills []app.AvailableSkill
	err    error
}

type composerWorkspacePathsLoadedMsg struct {
	paths []app.WorkspacePath
	err   error
}

type operationDoneMsg struct {
	err                     error
	sessionResult           *app.RunSessionResult
	delegatedQuestionResult *app.AnswerDelegatedSessionQuestionResult
}

type turnWritesRestoredMsg struct {
	result app.RestoreSessionTurnWritesResult
	err    error
}

type sessionCompactedMsg struct {
	result app.CompactSessionResult
	err    error
}

type workspaceInstructionsInitializedMsg struct {
	result app.InitializeWorkspaceInstructionsResult
	err    error
}

type workspacePromptSourcesCompressedMsg struct {
	result app.CompressWorkspacePromptSourcesResult
	err    error
}

type turnCancelRequestedMsg struct {
	err error
}

type sessionSnapshotRefreshedMsg struct {
	sessionID string
	state     events.SessionState
	err       error
}

type transcriptCopiedMsg struct {
	label string
	err   error
}

type composerExternalEditorDoneMsg struct {
	text string
	err  error
}
