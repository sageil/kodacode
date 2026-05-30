package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	tuitheme "github.com/sageil/kodacode/internal/tui/theme"
)

type workspaceSessionOpenRequest struct {
	WorkspaceRoot     string
	UserText          string
	Attachments       []app.AttachmentInput
	LocalShellCommand string
	TurnID            string
	AgentID           string
	StartTurnAgentID  string
	ThinkingEnabled   bool
	ReasoningVariant  string
	SkillIDs          []string
	InspectorOpen     bool
	WideSidebarOpen   bool
	WatchID           int
	AfterOpen         func(context.Context, Backend, string) error
}

type workspaceSessionReviewRequest struct {
	WorkspaceRoot    string
	TurnID           string
	Instructions     string
	AgentID          string
	ThinkingEnabled  bool
	ReasoningVariant string
	SkillIDs         []string
	InspectorOpen    bool
	WideSidebarOpen  bool
	WatchID          int
}

type sessionSwitchRequest struct {
	SessionID        string
	WorkspaceRoot    string
	AgentID          string
	ThinkingEnabled  bool
	ReasoningVariant string
	SkillIDs         []string
	InspectorOpen    bool
	WideSidebarOpen  bool
	WatchID          int
}

type timelineBranchRequest struct {
	SourceSessionID  string
	SourceTurnID     string
	WorkspaceRoot    string
	AgentID          string
	ThinkingEnabled  bool
	ReasoningVariant string
	SkillIDs         []string
	InspectorOpen    bool
	WideSidebarOpen  bool
	WatchID          int
}

func openWorkspaceSessionCmd(ctx context.Context, backend Backend, req workspaceSessionOpenRequest) tea.Cmd {
	return func() tea.Msg {
		session, err := backend.OpenWorkspaceSession(ctx, req.WorkspaceRoot, nil, false)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		if req.AfterOpen != nil {
			if err := req.AfterOpen(ctx, backend, session.SessionID); err != nil {
				return sessionOpenedMsg{err: err}
			}
		}
		state, err := backend.Snapshot(ctx, session.SessionID)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		watchCtx, cancel := context.WithCancel(ctx)
		stream, err := backend.Watch(watchCtx, session.SessionID, state.LastSequence)
		if err != nil {
			cancel()
			return sessionOpenedMsg{err: err}
		}
		resolvedTurnID := resolvedOpenedSessionTurnID(state, req.TurnID, strings.TrimSpace(req.UserText) != "" || strings.TrimSpace(req.LocalShellCommand) != "")
		return sessionOpenedMsg{
			view: sessionView{
				SessionID:        session.SessionID,
				TurnID:           resolvedTurnID,
				UserText:         req.UserText,
				AgentID:          req.AgentID,
				SkillIDs:         append([]string(nil), req.SkillIDs...),
				ThinkingEnabled:  req.ThinkingEnabled,
				ReasoningVariant: req.ReasoningVariant,
				WorkspaceRoot:    req.WorkspaceRoot,
				DetailTurnID:     resolvedTurnID,
				Focus:            focusTranscript,
				InspectorOpen:    req.InspectorOpen,
				WideSidebarOpen:  req.WideSidebarOpen,
			},
			state:             state,
			stateOwned:        true,
			stream:            stream,
			cancel:            cancel,
			watchID:           req.WatchID,
			startTurn:         strings.TrimSpace(req.UserText) != "",
			startTurnAgentID:  strings.TrimSpace(req.StartTurnAgentID),
			attachments:       append([]app.AttachmentInput(nil), req.Attachments...),
			localShellCommand: req.LocalShellCommand,
		}
	}
}

func branchSessionFromTurnCmd(ctx context.Context, backend Backend, req timelineBranchRequest) tea.Cmd {
	return func() tea.Msg {
		result, err := backend.BranchSessionFromTurn(ctx, app.BranchSessionFromTurnInput{
			SourceSessionID: req.SourceSessionID,
			SourceTurnID:    req.SourceTurnID,
		})
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		state, err := backend.Snapshot(ctx, result.SessionID)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		watchCtx, cancel := context.WithCancel(ctx)
		stream, err := backend.Watch(watchCtx, result.SessionID, state.LastSequence)
		if err != nil {
			cancel()
			return sessionOpenedMsg{err: err}
		}
		initialTurn := initialTurnID(state, false)
		return sessionOpenedMsg{
			view: sessionView{
				SessionID:        result.SessionID,
				TurnID:           initialTurn,
				UserText:         "",
				AgentID:          req.AgentID,
				SkillIDs:         append([]string(nil), req.SkillIDs...),
				ThinkingEnabled:  req.ThinkingEnabled,
				ReasoningVariant: req.ReasoningVariant,
				WorkspaceRoot:    req.WorkspaceRoot,
				DetailTurnID:     initialTurn,
				Focus:            focusComposer,
				InspectorOpen:    req.InspectorOpen,
				WideSidebarOpen:  req.WideSidebarOpen,
			},
			state:      state,
			stateOwned: true,
			stream:     stream,
			cancel:     cancel,
			watchID:    req.WatchID,
			startTurn:  false,
		}
	}
}

func openWorkspaceSessionForReviewCmd(ctx context.Context, backend Backend, req workspaceSessionReviewRequest) tea.Cmd {
	return func() tea.Msg {
		session, err := backend.OpenWorkspaceSession(ctx, req.WorkspaceRoot, nil, false)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		state, err := backend.Snapshot(ctx, session.SessionID)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		watchCtx, cancel := context.WithCancel(ctx)
		stream, err := backend.Watch(watchCtx, session.SessionID, state.LastSequence)
		if err != nil {
			cancel()
			return sessionOpenedMsg{err: err}
		}
		resolvedTurnID := resolvedOpenedSessionTurnID(state, req.TurnID, true)
		return sessionOpenedMsg{
			view: sessionView{
				SessionID:        session.SessionID,
				TurnID:           resolvedTurnID,
				UserText:         "",
				AgentID:          req.AgentID,
				SkillIDs:         append([]string(nil), req.SkillIDs...),
				ThinkingEnabled:  req.ThinkingEnabled,
				ReasoningVariant: req.ReasoningVariant,
				WorkspaceRoot:    req.WorkspaceRoot,
				DetailTurnID:     resolvedTurnID,
				Focus:            focusTranscript,
				InspectorOpen:    req.InspectorOpen,
				WideSidebarOpen:  req.WideSidebarOpen,
			},
			state:                 state,
			stateOwned:            true,
			stream:                stream,
			cancel:                cancel,
			watchID:               req.WatchID,
			startReview:           true,
			reviewInstructions:    strings.TrimSpace(req.Instructions),
			reviewThinkingEnabled: req.ThinkingEnabled,
			reviewThinkingMode:    req.ReasoningVariant,
			reviewSkillIDs:        append([]string(nil), req.SkillIDs...),
		}
	}
}

func resolvedOpenedSessionTurnID(state events.SessionState, requested string, startAction bool) string {
	if trimmed := strings.TrimSpace(requested); trimmed != "" {
		return trimmed
	}
	return initialTurnID(state, startAction)
}

func switchSessionCmd(ctx context.Context, backend Backend, req sessionSwitchRequest) tea.Cmd {
	return func() tea.Msg {
		state, err := backend.Snapshot(ctx, req.SessionID)
		if err != nil {
			return sessionOpenedMsg{err: err}
		}
		watchCtx, cancel := context.WithCancel(ctx)
		stream, err := backend.Watch(watchCtx, req.SessionID, state.LastSequence)
		if err != nil {
			cancel()
			return sessionOpenedMsg{err: err}
		}
		initialTurn := initialTurnID(state, false)
		return sessionOpenedMsg{
			view: sessionView{
				SessionID:        req.SessionID,
				TurnID:           initialTurn,
				UserText:         "",
				AgentID:          req.AgentID,
				SkillIDs:         append([]string(nil), req.SkillIDs...),
				ThinkingEnabled:  req.ThinkingEnabled,
				ReasoningVariant: req.ReasoningVariant,
				WorkspaceRoot:    req.WorkspaceRoot,
				DetailTurnID:     initialTurn,
				Focus:            focusTranscript,
				InspectorOpen:    req.InspectorOpen,
				WideSidebarOpen:  req.WideSidebarOpen,
			},
			state:      state,
			stateOwned: true,
			stream:     stream,
			cancel:     cancel,
			watchID:    req.WatchID,
			startTurn:  false,
		}
	}
}

func deleteSessionAndReopenDialogCmd(ctx context.Context, backend Backend, currentSessionID, targetSessionID string, th *tuitheme.Theme, width, height int) tea.Cmd {
	return func() tea.Msg {
		if strings.TrimSpace(targetSessionID) == "" {
			return nil
		}
		if err := backend.DeleteSession(ctx, targetSessionID); err != nil {
			return footerErrorMsg{err: err}
		}
		sessions, err := backend.ListSessions(ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newSessionsDialog(buildSessionItems(filterSessionSummaries(sessions, currentSessionID)), th)
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func purgeSessionsAndReopenDialogCmd(ctx context.Context, backend Backend, currentSessionID string, ids []string, th *tuitheme.Theme, width, height int) tea.Cmd {
	return func() tea.Msg {
		for _, id := range ids {
			if id == currentSessionID {
				continue
			}
			if err := backend.DeleteSession(ctx, id); err != nil {
				return footerErrorMsg{err: err}
			}
		}
		sessions, err := backend.ListSessions(ctx)
		if err != nil {
			return dialogOpenedMsg{err: err}
		}
		dialog := newSessionsDialog(buildSessionItems(filterSessionSummaries(sessions, currentSessionID)), th)
		dialog.SetFrame(width, height)
		return dialogOpenedMsg{dialog: dialog}
	}
}

func filterSessionSummaries(summaries []app.SessionSummary, excludedSessionID string) []app.SessionSummary {
	filtered := make([]app.SessionSummary, 0, len(summaries))
	for _, session := range summaries {
		if session.ID == excludedSessionID {
			continue
		}
		filtered = append(filtered, session)
	}
	return filtered
}
