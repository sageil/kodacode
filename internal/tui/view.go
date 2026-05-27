package tui

import (
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/cellbuf"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func renderModelView(m Model) string {
	content, _ := renderModelSurface(m)
	return content
}

func renderModelSurface(m Model) (string, *tea.Cursor) {
	if m.err != nil {
		return renderErrorView(m), nil
	}
	if m.width <= 0 || m.height <= 0 {
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render("starting kodacode…"), nil
	}

	state := m.projector.CurrentState()
	layout := resolveShellLayout(m, state)
	if m.dialog == nil && m.composerState.popupMode == composerPopupNone {
		if !hasFooterNotice(m, state) {
			if surface, ok := renderSplitWideAnimationSurface(m, state, layout); ok {
				return renderDialogSurface(surface), composerCursorForSurface(m, state, layout)
			}
			return renderModelRootContent(m, state, layout), composerCursorForSurface(m, state, layout)
		}
		base, baseRows := renderModelRootSurfaceBase(m, state, layout)
		surface := newOverlaySurface(base, baseRows)
		drawSplitWideAnimationOverlay(surface, m, state, layout)
		drawFooterNoticeOnSurface(surface, m, state, layout)
		return renderDialogSurface(surface), composerCursorForSurface(m, state, layout)
	}

	base, baseRows := renderModelRootSurfaceBase(m, state, layout)
	if hasFooterNotice(m, state) {
		surface := newOverlaySurface(base, baseRows)
		drawSplitWideAnimationOverlay(surface, m, state, layout)
		drawFooterNoticeOnSurface(surface, m, state, layout)
		if m.dialog != nil {
			area := resolveDialogRenderArea(m, state, layout)
			cursor := renderDialogOnBuffer(surface, m.dialog, area)
			return renderDialogSurface(surface), cursor
		}
		cursor := composerCursorForSurface(m, state, layout)
		if cursor != nil {
			width := composerPopupWidth(m, layout.totalWidth)
			popup := renderComposerPopup(m, width)
			if strings.TrimSpace(popup) != "" {
				drawRenderedComposerPopupOnSurface(surface, popup, cursor)
			}
		}
		return renderDialogSurface(surface), cursor
	}
	if m.dialog != nil {
		area := resolveDialogRenderArea(m, state, layout)
		if shouldOverlaySplitWideAnimation(m, state, layout) {
			surface := newOverlaySurface(base, baseRows)
			drawSplitWideAnimationOverlay(surface, m, state, layout)
			cursor := renderDialogOnBuffer(surface, m.dialog, area)
			return renderDialogSurface(surface), cursor
		}
		return renderDialogOverlaySurface(m, base, baseRows, area)
	}
	if shouldOverlaySplitWideAnimation(m, state, layout) {
		surface := newOverlaySurface(base, baseRows)
		drawSplitWideAnimationOverlay(surface, m, state, layout)
		cursor := composerCursorForSurface(m, state, layout)
		if cursor != nil {
			width := composerPopupWidth(m, layout.totalWidth)
			popup := renderComposerPopup(m, width)
			if strings.TrimSpace(popup) != "" {
				drawRenderedComposerPopupOnSurface(surface, popup, cursor)
			}
		}
		return renderDialogSurface(surface), cursor
	}
	return renderComposerOverlaySurface(m, state, layout, base, baseRows)
}

func renderModelRootSurfaceBuffer(m Model, state events.SessionState, layout shellLayout) *cellbuf.Buffer {
	base := renderModelRootSurfaceBaseBuffer(m, state, layout)
	if base == nil {
		return nil
	}
	return cloneCellBuffer(base)
}

func renderModelRootSurfaceBase(m Model, state events.SessionState, layout shellLayout) (*cellbuf.Buffer, []string) {
	width := max(m.width, 1)
	height := max(m.height, 1)
	rendered := renderModelRootContent(m, state, layout)
	if m.renderCache.rootSurface == nil {
		return newRenderedSurface(rendered, width, height)
	}
	return m.renderCache.rootSurface.surfaceFor(rendered, width, height)
}

func renderModelRootSurfaceBaseBuffer(m Model, state events.SessionState, layout shellLayout) *cellbuf.Buffer {
	base, _ := renderModelRootSurfaceBase(m, state, layout)
	return base
}

func renderModelRootContent(m Model, state events.SessionState, layout shellLayout) string {
	if shellLayoutEnabled(m) {
		return renderKodaShellView(m, state, layout)
	}
	if isWideShell(m) {
		return renderSplitWideView(m, state, layout)
	}

	header := renderHeaderBar(m, state, layout.totalWidth)
	body := renderMainShell(m, state, layout)
	footer := renderFooterBar(m, state, layout.totalWidth)
	return renderToneBlock(m.theme, toneBG, max(m.width, 1), max(m.height, 1), header+"\n"+body+"\n"+footer)
}

func renderSplitWideAnimationSurface(m Model, state events.SessionState, layout shellLayout) (*overlaySurface, bool) {
	if !shouldOverlaySplitWideAnimation(m, state, layout) {
		return nil, false
	}
	base, baseRows := renderModelRootSurfaceBase(m, state, layout)
	surface := newOverlaySurface(base, baseRows)
	drawSplitWideAnimationOverlay(surface, m, state, layout)
	return surface, true
}

func shouldOverlaySplitWideAnimation(m Model, state events.SessionState, layout shellLayout) bool {
	if shellLayoutEnabled(m) {
		return false
	}
	if !isWideShell(m) {
		return false
	}
	if m.width <= 0 || m.height <= 0 {
		return false
	}
	return m.shouldAnimateTranscriptActivityForState(state)
}

func drawSplitWideAnimationOverlay(surface dialogSurface, m Model, state events.SessionState, layout shellLayout) {
	if surface == nil || !shouldOverlaySplitWideAnimation(m, state, layout) {
		return
	}
	width := max(layout.totalWidth, 1)
	drawBlockOnSurface(surface, renderHeaderDividerForState(m, state, width), 0, splitWideHeaderHeight()-1)
	if activity, ok := composerActivityStripStateFor(m, state); ok && strings.TrimSpace(activity.Label) != "" {
		footerTop := splitWideHeaderHeight() + splitWidePanelHeight(layout)
		drawBlockOnSurface(surface, renderComposerActivityStrip(m, state, width, composerBorderColor(m, state)), 0, footerTop)
	}
}

func renderModelBaseSurface(surface dialogSurface, m Model, state events.SessionState, layout shellLayout) *tea.Cursor {
	cursor := composerCursorForSurface(m, state, layout)
	if m.composerState.popupMode == composerPopupNone {
		return cursor
	}
	if cursor == nil {
		return nil
	}

	drawComposerPopupOnSurface(surface, m, state, layout)
	return cursor
}

func resolveDialogRenderArea(m Model, state events.SessionState, layout shellLayout) dialogRenderArea {
	rect := resolveShellRects(m, state, layout).transcript
	return dialogRenderArea{
		x:      rect.x,
		y:      rect.y,
		width:  max(rect.width, 1),
		height: max(rect.height, 1),
	}
}

func renderErrorView(m Model) string {
	title := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "error", "#ff6b6b"))).
		Bold(true).
		Render("kodacode")
	body := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorFor(m.theme, "error", "#ff6b6b"))).
		Padding(0, 1).
		Render(m.err.Error())
	return title + "\n\n" + body
}

func currentTurn(state events.SessionState, turnID string) *events.TurnState {
	return state.Turns[turnID]
}

func effectiveLiveTurnID(m Model, state events.SessionState) string {
	if turnID := pendingInteractionTurnIDFromState(state); turnID != "" {
		return turnID
	}
	if trackedTurnID := strings.TrimSpace(m.turnID); trackedTurnID != "" {
		descendantTurnID := latestContinuationDescendantTurnID(state, trackedTurnID)
		if turn := currentTurn(state, descendantTurnID); turn != nil && !isTurnFinished(turn) {
			return descendantTurnID
		}
		if turn := currentTurn(state, trackedTurnID); turn != nil && !isTurnFinished(turn) {
			return trackedTurnID
		}
	}
	for idx := len(state.TurnOrder) - 1; idx >= 0; idx-- {
		turnID := strings.TrimSpace(state.TurnOrder[idx])
		turn := currentTurn(state, turnID)
		if turn != nil && !isTurnFinished(turn) {
			return turnID
		}
	}
	return ""
}

func effectiveDetailTurnID(m Model, state events.SessionState) string {
	if turnID := strings.TrimSpace(m.selection.detailTurnID); turnID != "" && state.Turns[turnID] != nil {
		return turnID
	}
	return m.turnID
}

func effectiveFooterTurnID(m Model, state events.SessionState) string {
	if turnID := effectiveLiveTurnID(m, state); turnID != "" && (m.busy || m.liveTurn.spinnerArmed || pendingInteractionTurnIDFromState(state) != "") {
		return turnID
	}
	if turnID := strings.TrimSpace(m.turnID); turnID != "" && state.Turns[turnID] != nil {
		return turnID
	}
	return effectiveDetailTurnID(m, state)
}

func effectiveStatusMetricsScope(m Model, state events.SessionState) (events.SessionState, string, bool) {
	if childState, handoff, ok := m.activeDelegatedSessionState(state); ok {
		if turnID := strings.TrimSpace(handoff.ChildTurnID); turnID != "" {
			return childState, turnID, true
		}
		return childState, effectiveFooterTurnID(m, childState), true
	}
	if childState, handoff, ok := m.selectedDelegatedSessionState(state); ok {
		if turnID := strings.TrimSpace(handoff.ChildTurnID); turnID != "" {
			return childState, turnID, true
		}
		return childState, effectiveFooterTurnID(m, childState), true
	}
	return state, effectiveFooterTurnID(m, state), false
}

func inspectorDetailTurn(state events.SessionState, m Model) *events.TurnState {
	return currentTurn(state, effectiveDetailTurnID(m, state))
}

func orderedHandoffIDs(turn *events.TurnState) []string {
	if turn == nil {
		return nil
	}
	handoffIDs := make([]string, 0, len(turn.HandoffOrder))
	for _, handoffID := range turn.HandoffOrder {
		if turn.Handoffs[handoffID] != nil {
			handoffIDs = append(handoffIDs, handoffID)
		}
	}
	return handoffIDs
}

func orderedToolCallIDs(turn *events.TurnState) []string {
	if turn == nil {
		return nil
	}
	callIDs := make([]string, 0, len(turn.ToolCallOrder))
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if !toolCallVisibleInRuntime(turn, call) {
			continue
		}
		callIDs = append(callIDs, callID)
	}
	return callIDs
}

func toolCallVisibleInRuntime(turn *events.TurnState, call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	if call.Declared || call.Executing || call.Completed {
		return true
	}
	return turn != nil && turn.Status == events.TurnStatusRunning
}

type sessionToolCallRef struct {
	TurnID string
	CallID string
}

func orderedSessionTurnIDs(state events.SessionState) []string {
	turnIDs := make([]string, 0, len(state.TurnOrder))
	for _, turnID := range state.TurnOrder {
		if state.Turns[turnID] != nil {
			turnIDs = append(turnIDs, turnID)
		}
	}
	return turnIDs
}

func orderedSessionToolCallRefs(state events.SessionState) []sessionToolCallRef {
	refs := make([]sessionToolCallRef, 0, len(state.TurnOrder)*2)
	for _, turnID := range orderedSessionTurnIDs(state) {
		turn := state.Turns[turnID]
		for _, callID := range orderedToolCallIDs(turn) {
			call := turn.ToolCalls[callID]
			if !showToolCallInToolsList(turn, callID, call) {
				continue
			}
			refs = append(refs, sessionToolCallRef{TurnID: turnID, CallID: callID})
		}
	}
	return refs
}

func showToolCallInToolsList(turn *events.TurnState, callID string, call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	if shouldHideSupersededMutationFailure(turn, callID, call) {
		return false
	}
	if shouldHideSupersededRetriedLogicalToolCall(turn, callID, call) {
		return false
	}
	if shouldHideSupersededDelegateAttempt(turn, callID, call) {
		return false
	}
	switch strings.TrimSpace(call.ToolName) {
	case "read":
		return false
	default:
		return true
	}
}

func sessionToolCall(state events.SessionState, ref sessionToolCallRef) (*events.TurnState, *events.ToolCallState) {
	turn := state.Turns[ref.TurnID]
	if turn == nil {
		return nil, nil
	}
	call := turn.ToolCalls[ref.CallID]
	if call == nil {
		return turn, nil
	}
	return turn, call
}

func selectedSessionToolCall(state events.SessionState, m Model) (string, sessionToolCallRef, *events.TurnState, *events.ToolCallState, bool) {
	ref := sessionToolCallRef{
		TurnID: strings.TrimSpace(m.selection.callTurnID),
		CallID: strings.TrimSpace(m.selection.callID),
	}
	if ref.TurnID == "" || ref.CallID == "" {
		return "", sessionToolCallRef{}, nil, nil, false
	}
	sessionID := selectedToolSessionID(m)
	if sessionID != "" && sessionID != strings.TrimSpace(state.SessionID) {
		childState, ok := m.delegatedSnapshot(sessionID)
		if !ok {
			return "", sessionToolCallRef{}, nil, nil, false
		}
		turn, call := sessionToolCall(childState, ref)
		if turn == nil || call == nil {
			return "", sessionToolCallRef{}, nil, nil, false
		}
		return sessionID, ref, turn, call, true
	}
	turn, call := sessionToolCall(state, ref)
	if turn == nil || call == nil {
		return "", sessionToolCallRef{}, nil, nil, false
	}
	return strings.TrimSpace(state.SessionID), ref, turn, call, true
}

func sessionToolTurnOrdinal(state events.SessionState, turnID string) int {
	for idx, candidate := range state.TurnOrder {
		if candidate == turnID {
			return idx + 1
		}
	}
	return 0
}

func effectiveHandoffID(turn *events.TurnState, selectedHandoffID string) string {
	if turn == nil {
		return ""
	}
	if selectedHandoffID != "" && turn.Handoffs[selectedHandoffID] != nil {
		return selectedHandoffID
	}
	if handoff := featuredHandoff(turn); handoff != nil {
		return handoff.HandoffID
	}
	return ""
}

func explicitSelectedHandoff(turn *events.TurnState, selectedHandoffID string) *events.AgentHandoffState {
	if turn == nil || selectedHandoffID == "" {
		return nil
	}
	return turn.Handoffs[selectedHandoffID]
}

func selectedHandoff(turn *events.TurnState, selectedHandoffID string) *events.AgentHandoffState {
	handoffID := effectiveHandoffID(turn, selectedHandoffID)
	if handoffID == "" {
		return nil
	}
	return turn.Handoffs[handoffID]
}

func toolStatusColor(th *theme.Theme, status string) color.Color {
	switch status {
	case "running", "building", "preparing", "declared", "waiting":
		return lipgloss.Color(colorFor(th, "warning", "#f9e2af"))
	case "partial":
		return lipgloss.Color(colorFor(th, "subtext", "#9da8ca"))
	case "done":
		return lipgloss.Color(colorFor(th, "success", "#a6e3a1"))
	default:
		return lipgloss.Color(colorFor(th, "error", "#f38ba8"))
	}
}

func taskStatusColor(th *theme.Theme, status string) color.Color {
	switch status {
	case "blocked":
		return lipgloss.Color(colorFor(th, "warning", "#f9e2af"))
	default:
		return toolStatusColor(th, status)
	}
}
