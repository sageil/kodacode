package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/events"
)

func loadToolResultCmd(ctx context.Context, controller controller, sessionID string, ref sessionToolCallRef) tea.Cmd {
	return func() tea.Msg {
		result, err := controller.LoadToolResult(ctx, sessionID, ref.TurnID, ref.CallID)
		return toolResultLoadedMsg{sessionID: sessionID, ref: ref, result: result, err: err}
	}
}

func (m *Model) ensureSelectedToolResultLoadedCmd() tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID, ref, _, call, ok := selectedSessionToolCall(m.projector.Snapshot(), *m)
	if !ok || !toolResultNeedsHydration(call) {
		return nil
	}
	return m.ensureToolResultLoadedForSessionCmd(sessionID, ref, call)
}

func (m *Model) ensureToolResultLoadedForSessionCmd(sessionID string, ref sessionToolCallRef, call *events.ToolCallState) tea.Cmd {
	if m == nil || !toolResultNeedsHydration(call) {
		return nil
	}
	key := scopedToolKey(sessionID, ref)
	if _, ok := m.toolHydration.loadedResults[key]; ok {
		return nil
	}
	if m.toolHydration.loadingResults[key] {
		return nil
	}
	m.toolHydration.loadingResults[key] = true
	return loadToolResultCmd(m.ctx, m.controller, strings.TrimSpace(sessionID), ref)
}

func callHasOffloadedToolResult(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	return (call.OutputBlob != nil && call.OutputBlob.Ref != "") || (call.ErrorBlob != nil && call.ErrorBlob.Ref != "")
}

func toolResultNeedsHydration(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	if callHasOffloadedToolResult(call) {
		return true
	}
	if !call.Completed {
		return false
	}
	if strings.TrimSpace(call.Output) != "" || strings.TrimSpace(call.Error) != "" {
		return false
	}
	return !events.SnapshotRetainsToolBody(call)
}

func toolResultHydrationPlaceholder(m Model, ref *sessionToolCallRef, call *events.ToolCallState) string {
	return toolResultHydrationPlaceholderForSession(m, m.sessionID, ref, call)
}

func toolResultHydrationPlaceholderForSession(m Model, sessionID string, ref *sessionToolCallRef, call *events.ToolCallState) string {
	if call == nil || !toolResultNeedsHydration(call) || toolResultBodyLoadedForSession(m, sessionID, ref) {
		return ""
	}
	if ref != nil && m.toolHydration.loadingResults[scopedToolKey(sessionID, *ref)] {
		return "loading full output..."
	}
	return "full output unavailable in session snapshot"
}

func toolResultOutput(m Model, ref *sessionToolCallRef, call *events.ToolCallState) string {
	return toolResultOutputForSession(m, m.sessionID, ref, call)
}

func toolResultOutputForSession(m Model, sessionID string, ref *sessionToolCallRef, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if ref != nil {
		if loaded, ok := m.toolHydration.loadedResults[scopedToolKey(sessionID, *ref)]; ok {
			return loaded.Output
		}
	}
	return call.Output
}

func toolResultError(m Model, ref *sessionToolCallRef, call *events.ToolCallState) string {
	return toolResultErrorForSession(m, m.sessionID, ref, call)
}

func toolResultErrorForSession(m Model, sessionID string, ref *sessionToolCallRef, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if ref != nil {
		if loaded, ok := m.toolHydration.loadedResults[scopedToolKey(sessionID, *ref)]; ok {
			return loaded.Error
		}
	}
	return call.Error
}

func toolResultBodyLoaded(m Model, ref *sessionToolCallRef) bool {
	return toolResultBodyLoadedForSession(m, m.sessionID, ref)
}

func toolResultBodyLoadedForSession(m Model, sessionID string, ref *sessionToolCallRef) bool {
	if ref == nil {
		return false
	}
	_, ok := m.toolHydration.loadedResults[scopedToolKey(sessionID, *ref)]
	return ok
}

func toolCallTranscriptUsesLoadedResult(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	return !showMutationToolInTranscript(call) && !isCommandToolCall(call)
}

func (m Model) shouldSyncTranscriptForLoadedToolResult(state events.SessionState, ref sessionToolCallRef) bool {
	selectedSessionID, selectedRef, _, call, ok := selectedSessionToolCall(state, m)
	if !ok {
		return false
	}
	if strings.TrimSpace(selectedSessionID) != strings.TrimSpace(state.SessionID) {
		return false
	}
	if selectedRef != ref {
		return false
	}
	return toolCallTranscriptUsesLoadedResult(call)
}

func (m Model) transcriptRefreshPlanForLoadedToolResult(state events.SessionState, ref sessionToolCallRef) transcriptRefreshPlan {
	if !m.shouldSyncTranscriptForLoadedToolResult(state, ref) {
		return transcriptRefreshPlan{}
	}
	return transcriptTurnRefreshPlan(ref.TurnID)
}

func (m Model) selectedTranscriptLoadedToolResultRef(state events.SessionState, turnID string) (sessionToolCallRef, bool) {
	selectedSessionID, selectedRef, _, call, ok := selectedSessionToolCall(state, m)
	if !ok || strings.TrimSpace(selectedRef.TurnID) != strings.TrimSpace(turnID) {
		return sessionToolCallRef{}, false
	}
	if strings.TrimSpace(selectedSessionID) != strings.TrimSpace(state.SessionID) {
		return sessionToolCallRef{}, false
	}
	if !toolCallTranscriptUsesLoadedResult(call) {
		return sessionToolCallRef{}, false
	}
	if _, ok := m.toolHydration.loadedResults[scopedToolKey(selectedSessionID, selectedRef)]; !ok {
		return sessionToolCallRef{}, false
	}
	return selectedRef, true
}

func toolResultPreviewNotice(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	parts := make([]string, 0, 2)
	if call.OutputTruncated {
		parts = append(parts, "full output available")
	}
	if call.ErrorTruncated {
		parts = append(parts, "full error available")
	}
	return strings.Join(parts, "; ")
}
