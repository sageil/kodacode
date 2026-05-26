package tui

import tea "charm.land/bubbletea/v2"

func (d *commandPaletteDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		return d.updateKey(typed)
	default:
		return d.updateInputs(msg)
	}
}

func (d *commandPaletteDialog) updateKey(msg tea.KeyPressMsg) (dialogModel, tea.Cmd) {
	if (d.kind == commandPaletteModel || d.kind == commandPaletteUtilityModel || d.kind == commandPaletteReviewerModel) && msg.String() == "ctrl+r" {
		return d, emitDialogMsg(modelCatalogRefreshRequestedMsg{
			query:    d.filter.Value(),
			selected: d.selectedModelRefreshRef(),
		})
	}

	switch msg.String() {
	case "esc":
		return d.handleEscape()
	case "tab":
		return d, d.moveFocus(1)
	case "shift+tab", "backtab":
		return d, d.moveFocus(-1)
	case "left":
		if d.focusedButtonIndex() >= 0 {
			return d, d.moveFocus(-1)
		}
	case "right":
		if d.focusedButtonIndex() >= 0 {
			return d, d.moveFocus(1)
		}
	case "enter":
		return d.handleEnter()
	case "up":
		return d.handleUp()
	case "down":
		return d.handleDown()
	case "ctrl+d":
		return d.handleShortcutDelete()
	case "ctrl+p":
		return d.handleShortcutPurge()
	}
	return d.updateInputs(msg)
}

func (d *commandPaletteDialog) updateInputs(msg tea.Msg) (dialogModel, tea.Cmd) {
	inputs := d.activeInputs()
	if len(inputs) == 0 {
		return d, nil
	}
	cmds := make([]tea.Cmd, len(inputs))
	prevFilter := d.filter.Value()
	for idx, input := range inputs {
		*input, cmds[idx] = input.Update(msg)
	}
	if d.filter.Value() != prevFilter {
		d.refilter()
	}
	return d, tea.Batch(cmds...)
}

func (d *commandPaletteDialog) handleEscape() (dialogModel, tea.Cmd) {
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return d, closeDialog(d.id, nil)
	}
	return d, closeDialog(d.id, nil)
}

func (d *commandPaletteDialog) handleEnter() (dialogModel, tea.Cmd) {
	if buttonIdx := d.focusedButtonIndex(); buttonIdx >= 0 {
		return d.activateButton(buttonIdx)
	}
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return d.activateListSelection()
	}
	return d, nil
}

func (d *commandPaletteDialog) handleUp() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(-1)
	}
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		d.moveCursor(-1, len(d.listOptions()))
	}
	return d, nil
}

func (d *commandPaletteDialog) handleDown() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(1)
	}
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		d.moveCursor(1, len(d.listOptions()))
	}
	return d, nil
}

func (d *commandPaletteDialog) handleShortcutDelete() (dialogModel, tea.Cmd) {
	return d, nil
}

func (d *commandPaletteDialog) handleShortcutPurge() (dialogModel, tea.Cmd) {
	return d, nil
}

func (d *commandPaletteDialog) activateButton(buttonIdx int) (dialogModel, tea.Cmd) {
	buttons := d.activeButtons()
	if buttonIdx < 0 || buttonIdx >= len(buttons) {
		return d, nil
	}
	switch buttons[buttonIdx].id {
	case "open":
		return d.activateListSelection()
	case "refresh":
		return d, emitDialogMsg(modelCatalogRefreshRequestedMsg{
			query:    d.filter.Value(),
			selected: d.selectedModelRefreshRef(),
		})
	case "cancel":
		return d, closeDialog(d.id, nil)
	case "back":
		if d.returnToActions {
			d.moveToActions()
		}
		d.focusIndex = 0
		return d, d.syncFocus()
	}
	return d, nil
}

func (d *commandPaletteDialog) activateListSelection() (dialogModel, tea.Cmd) {
	options := d.listOptions()
	if len(options) == 0 || d.cursor >= len(options) {
		return d, nil
	}
	selected := options[d.cursor]
	if selected.Disabled {
		return d, nil
	}
	switch d.kind {
	case commandPaletteReviewerModel:
		return d, closeDialog(d.id, reviewerModelSelectionResult{Ref: selected.Model.Ref})
	case commandPaletteUtilityModel:
		return d, closeDialog(d.id, utilityModelSelectionResult{Ref: selected.Model.Ref})
	case commandPaletteModel:
		return d, closeDialog(d.id, selected.Model.Ref)
	case commandPaletteAgent:
		return d, closeDialog(d.id, selected.Agent)
	case commandPaletteActions:
		switch selected.Action.ID {
		case "select-model", "select-agent", "select-theme", "manage-sessions", "manage-trust", "new-session", "connect-provider", "select-utility-model", "unset-utility-model", "select-reviewer-model", "unset-reviewer-model":
			return d, closeDialog(d.id, commandPaletteActionResult{ActionID: selected.Action.ID})
		}
	}
	return d, nil
}

func emitDialogMsg(msg tea.Msg) tea.Cmd {
	return func() tea.Msg { return msg }
}
