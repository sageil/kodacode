package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
)

func (d *commandPaletteDialog) selectedModelRefreshRef() provider.ModelRef {
	if d.kind != commandPaletteModel && d.kind != commandPaletteUtilityModel && d.kind != commandPaletteReviewerModel {
		return provider.ModelRef{}
	}
	options := d.listOptions()
	if d.cursor >= 0 && d.cursor < len(options) {
		return options[d.cursor].Model.Ref
	}
	if exact, err := provider.ParseModelRef(strings.TrimSpace(d.currentModel)); err == nil {
		return exact
	}
	return provider.ModelRef{}
}

func (d *commandPaletteDialog) applyDialogStateRefresh(state app.DialogState, query, selected string) {
	ref, _ := provider.ParseModelRef(strings.TrimSpace(selected))
	d.modelItems = append(d.modelItems[:0], buildModelItems(state, ref)...)
	d.currentModel = selected
	d.filter.SetValue(strings.TrimSpace(query))
	d.refilter()
}

func (d *commandPaletteDialog) refilter() {
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		d.resetWindow(len(d.listOptions()))
	}
}

func (d *commandPaletteDialog) activeInputs() []*textinput.Model {
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return []*textinput.Model{&d.filter}
	}
	return nil
}

func (d *commandPaletteDialog) activeButtons() []paletteButton {
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		return nil
	}
	return nil
}

func (d *commandPaletteDialog) focusedButtonIndex() int {
	return d.paletteListState.focusedButtonIndex(len(d.activeInputs()), len(d.activeButtons()))
}

func (d *commandPaletteDialog) moveFocus(delta int) tea.Cmd {
	total := len(d.activeInputs()) + len(d.activeButtons())
	if total == 0 {
		return nil
	}
	d.paletteListState.moveFocus(delta, total)
	return d.syncFocus()
}

func (d *commandPaletteDialog) syncFocus() tea.Cmd {
	d.resizeInputs()
	inputs := d.activeInputs()
	cmds := make([]tea.Cmd, 0, len(inputs))
	for idx, input := range inputs {
		if idx == d.focusIndex {
			cmds = append(cmds, input.Focus())
		} else {
			input.Blur()
		}
	}
	return tea.Batch(cmds...)
}

func (d *commandPaletteDialog) resizeInputs() {
	d.filter.SetWidth(max(d.dialogWidth()-8, 18))
}

func (d *commandPaletteDialog) configureInputs() {
	switch d.kind {
	case commandPaletteActions, commandPaletteModel, commandPaletteAgent, commandPaletteUtilityModel, commandPaletteReviewerModel:
		switch d.kind {
		case commandPaletteModel:
			d.filter.Placeholder = "search models"
		case commandPaletteAgent:
			d.filter.Placeholder = "search agents"
		case commandPaletteUtilityModel:
			d.filter.Placeholder = "search utility models"
		case commandPaletteReviewerModel:
			d.filter.Placeholder = "search reviewer models"
		default:
			d.filter.Placeholder = "search commands"
		}
	}
}

func (d *commandPaletteDialog) moveToActions() {
	d.kind = commandPaletteActions
	d.focusIndex = 0
	d.cursor = 0
	d.offset = 0
	d.configureInputs()
	d.filter.SetValue("")
	d.refilter()
}
