package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (d *commandPaletteDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := d.dialogWidth()
	content := renderPaletteDialogContentSized(d.theme, max(width-dialogFrameInset*2, 1), dialogPaletteFrame{
		Prompt: d.headerPrompt(),
		Body:   d.bodyView(),
		Hint:   d.hintView(),
	}, d.dialogContentMinBodyHeight())
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *commandPaletteDialog) headerPrompt() string {
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteWorkflow, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return d.filter.View()
	}
	return ""
}

func (d *commandPaletteDialog) bodyView() string {
	parts := []string{}
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteWorkflow, commandPaletteUtilityModel, commandPaletteReviewerModel:
		parts = append(parts, d.renderListRows())
	}
	if buttons := d.renderButtons(); buttons != "" {
		parts = append(parts, buttons)
	}
	return strings.Join(parts, "\n\n")
}

func (d *commandPaletteDialog) hintView() string {
	switch d.kind {
	case commandPaletteModel:
		if d.mutableSelectionLocked() {
			return "↑/↓ navigate • ctrl+r refresh • enter select enabled model • esc close"
		}
		return "↑/↓ navigate • ctrl+r refresh • enter select • esc close"
	case commandPaletteAgent:
		if d.mutableSelectionLocked() {
			return "↑/↓ navigate • enter select enabled agent • esc close"
		}
		return "↑/↓ navigate • enter select • esc close"
	case commandPaletteWorkflow:
		if d.mutableSelectionLocked() {
			return "↑/↓ navigate • enter select enabled workflow • esc close"
		}
		return "↑/↓ navigate • enter select • esc close"
	case commandPaletteUtilityModel:
		if d.mutableSelectionLocked() {
			return "↑/↓ navigate • ctrl+r refresh • enter select enabled model • esc close"
		}
		return "↑/↓ navigate • ctrl+r refresh • enter select • esc close"
	case commandPaletteReviewerModel:
		if d.mutableSelectionLocked() {
			return "↑/↓ navigate • ctrl+r refresh • enter select enabled model • esc close"
		}
		return "↑/↓ navigate • ctrl+r refresh • enter select • esc close"
	case commandPaletteActions:
		return "↑/↓ navigate • enter select • esc close"
	}
	return ""
}
