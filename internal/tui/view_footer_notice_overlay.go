package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func hasFooterNotice(m Model, state events.SessionState) bool {
	text, _ := footerNoticeText(m, state)
	return strings.TrimSpace(text) != ""
}

func drawFooterNoticeOnSurface(surface dialogSurface, m Model, state events.SessionState, layout shellLayout) {
	if surface == nil || !hasFooterNotice(m, state) {
		return
	}
	width := max(layout.totalWidth, surface.Width())
	notice := renderFooterNoticeBlock(m, state, width)
	if strings.TrimSpace(notice) == "" {
		return
	}
	height := lipgloss.Height(notice)
	if height <= 0 {
		return
	}
	y := max(surface.Height()-height, 0)
	drawBlockOnSurface(surface, notice, 0, y)
}
