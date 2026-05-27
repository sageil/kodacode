package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

const (
	composerPopupMinWidth = 24
	composerPopupMaxWidth = 72
)

type composerOverlayLayout struct {
	textX int
	textY int
}

func composerCursorForSurface(m Model, state events.SessionState, layout shellLayout) *tea.Cursor {
	liveActive, _ := m.liveTurnSpinnerState(state)
	if m.chrome.focus != focusComposer || liveActive || !m.composerInputEnabledForState(state) || hasPendingInteractionInState(state, m.turnID) {
		return nil
	}
	cursor := m.composer.Cursor()
	if cursor == nil {
		return nil
	}
	offset, ok := resolveComposerOverlayLayout(m, state, layout)
	if !ok {
		return nil
	}
	result := *cursor
	result.X += offset.textX
	result.Y += offset.textY
	return &result
}

func drawComposerPopupOnSurface(surface dialogSurface, m Model, state events.SessionState, layout shellLayout) {
	if surface == nil || m.composerState.popupMode == composerPopupNone {
		return
	}
	cursor := composerCursorForSurface(m, state, layout)
	if cursor == nil {
		return
	}

	width := composerPopupWidth(m, layout.totalWidth)
	popup := renderComposerPopup(m, width)
	drawRenderedComposerPopupOnSurface(surface, popup, cursor)
}

func drawRenderedComposerPopupOnSurface(surface dialogSurface, popup string, cursor *tea.Cursor) {
	if surface == nil || cursor == nil {
		return
	}
	popupWidth := blockWidth(popup)
	popupHeight := lipgloss.Height(popup)
	if popupWidth <= 0 || popupHeight <= 0 {
		return
	}

	x := min(max(cursor.X, 0), max(surface.Width()-popupWidth, 0))
	y := cursor.Y - popupHeight
	if y < 0 {
		y = 0
	}
	if y+popupHeight > surface.Height() {
		y = max(surface.Height()-popupHeight, 0)
	}
	drawBlockOnSurface(surface, popup, x, y)
}

func composerPopupWidth(m Model, available int) int {
	available = max(available, composerPopupMinWidth)
	maxWidth := min(available, composerPopupMaxWidth)
	if maxWidth < composerPopupMinWidth {
		maxWidth = composerPopupMinWidth
	}

	title, hint := composerPopupHeader(m)
	contentWidth := max(ansiWidth(title), ansiWidth(hint))
	switch {
	case m.composerState.popupMode == composerPopupHistory && m.composerState.promptHistoryBusy:
		contentWidth = max(contentWidth, ansiWidth("Loading recent prompts…"))
	case m.composerState.popupMode == composerPopupSkills && m.composerState.skillsBusy:
		contentWidth = max(contentWidth, ansiWidth("Loading skills…"))
	case m.composerState.popupMode == composerPopupPaths && m.composerState.workspacePathsBusy:
		contentWidth = max(contentWidth, ansiWidth("Loading workspace paths…"))
	case len(m.composerPopupItems()) == 0:
		empty := "No matches."
		switch m.composerState.popupMode {
		case composerPopupHistory:
			empty = "No saved prompts."
		case composerPopupSkills:
			empty = "No skills."
		case composerPopupPaths:
			empty = "No paths."
		}
		contentWidth = max(contentWidth, ansiWidth(empty))
	default:
		for _, item := range m.composerPopupItems() {
			rowWidth := ansiWidth(item.Title) + 2
			if meta := strings.TrimSpace(item.Meta); meta != "" {
				rowWidth += 1 + ansiWidth(meta)
			}
			contentWidth = max(contentWidth, rowWidth)
		}
	}

	width := contentWidth + 4
	width = max(width, composerPopupMinWidth)
	return min(width, maxWidth)
}

func resolveComposerOverlayLayout(m Model, state events.SessionState, layout shellLayout) (composerOverlayLayout, bool) {
	if m.height <= 0 {
		return composerOverlayLayout{}, false
	}

	if shellLayoutEnabled(m) {
		y := max(m.height-kodaShellFooterHeight(m, state, layout.totalWidth), 0)
		if notice := renderFooterNoticeBlock(m, state, layout.totalWidth); strings.TrimSpace(notice) != "" {
			y += lipgloss.Height(notice)
		}
		if divider := renderComposerActivityStrip(m, state, layout.totalWidth, composerBorderColor(m, state)); strings.TrimSpace(divider) != "" {
			y += lipgloss.Height(divider)
		}
		return composerOverlayLayout{textY: y}, true
	}

	footerTop := composerFooterTop(m, state, layout)
	composerTop := footerTop

	if isWideShell(m) {
		y := composerTop
		if divider := strings.TrimSpace(renderComposerActivityStrip(m, state, layout.totalWidth, composerBorderColor(m, state))); divider != "" {
			y += lipgloss.Height(divider)
		}
		return composerOverlayLayout{textY: y}, true
	}

	y := composerTop
	if divider := strings.TrimSpace(renderComposerActivityStrip(m, state, layout.totalWidth, composerBorderColor(m, state))); divider != "" {
		y += lipgloss.Height(divider)
	}
	y += 1 // header row
	return composerOverlayLayout{textY: y}, true
}

func composerFooterTop(m Model, state events.SessionState, layout shellLayout) int {
	return max(resolveShellRects(m, state, layout).composerFocus.y, 0)
}

func composerDisabled(m Model, state events.SessionState) bool {
	return strings.TrimSpace(m.composerDisabledMessage(state)) != "" && !m.hasPendingInteraction()
}

func composerBorderColor(m Model, state events.SessionState) string {
	border := lineTone(m)
	if m.chrome.focus == focusComposer {
		border = colorFor(m.theme, "primary", "#7cc7ff")
	}
	if m.hasPendingInteraction() {
		return colorFor(m.theme, "warning", "#ffd28f")
	}
	if composerDisabled(m, state) {
		return colorFor(m.theme, "warning", "#ffd28f")
	}
	if strings.TrimSpace(m.composerState.err) != "" {
		return colorFor(m.theme, "error", "#ff9aa6")
	}
	return border
}
