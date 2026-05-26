package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type themeDialog struct {
	id          string
	frameWidth  int
	frameHeight int
	theme       *theme.Theme

	paletteListState

	filter textinput.Model

	themeItems     []themeItem
	filteredThemes []themeItem
}

func newThemeDialog(items []themeItem, current string, th *theme.Theme) *themeDialog {
	filter := newDialogTextInput(th, 128)
	filter.Focus()
	dialog := &themeDialog{
		id:          dialogIDTheme,
		frameWidth:  96,
		frameHeight: 32,
		theme:       th,
		paletteListState: newPaletteListState(
			commandPaletteMaxVisible,
		),
		filter:         filter,
		themeItems:     append([]themeItem(nil), items...),
		filteredThemes: append([]themeItem(nil), items...),
	}
	for idx, item := range dialog.filteredThemes {
		if item.Name == current {
			dialog.cursor = idx
			break
		}
	}
	dialog.ensureVisible(len(dialog.filteredThemes))
	dialog.configureInputs()
	dialog.syncFocus()
	return dialog
}

func (d *themeDialog) ID() string { return d.id }

func (d *themeDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	applyDialogInputTheme(&d.filter, th)
}

func (d *themeDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.resizeInputs()
}

func (d *themeDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := d.dialogWidth()
	content := renderPaletteDialogContentSized(d.theme, max(width-dialogFrameInset*2, 1), dialogPaletteFrame{
		Prompt: d.headerPrompt(),
		Body:   d.bodyView(),
		Hint:   d.hintView(),
	}, d.dialogContentMinBodyHeight())
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *themeDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		return d.updateKey(typed)
	default:
		return d.updateInputs(msg)
	}
}

func (d *themeDialog) updateKey(msg tea.KeyPressMsg) (dialogModel, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return d, closeDialog(d.id, nil)
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
	}
	return d.updateInputs(msg)
}

func (d *themeDialog) updateInputs(msg tea.Msg) (dialogModel, tea.Cmd) {
	cmds := make([]tea.Cmd, 1)
	prevFilter := d.filter.Value()
	d.filter, cmds[0] = d.filter.Update(msg)
	if d.filter.Value() != prevFilter {
		d.refilter()
	}
	return d, tea.Batch(cmds...)
}

func (d *themeDialog) handleEnter() (dialogModel, tea.Cmd) {
	if buttonIdx := d.focusedButtonIndex(); buttonIdx >= 0 {
		return d.activateButton(buttonIdx)
	}
	return d.activateThemeSelect()
}

func (d *themeDialog) handleUp() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(-1)
	}
	d.moveCursor(-1, len(d.filteredThemes))
	return d, nil
}

func (d *themeDialog) handleDown() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(1)
	}
	d.moveCursor(1, len(d.filteredThemes))
	return d, nil
}

func (d *themeDialog) activateButton(buttonIdx int) (dialogModel, tea.Cmd) {
	buttons := d.activeButtons()
	if buttonIdx < 0 || buttonIdx >= len(buttons) {
		return d, nil
	}
	switch buttons[buttonIdx].id {
	case "select":
		return d.activateThemeSelect()
	case "cancel":
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d *themeDialog) activateThemeSelect() (dialogModel, tea.Cmd) {
	if len(d.filteredThemes) == 0 {
		return d, nil
	}
	return d, closeDialog(d.id, d.filteredThemes[d.cursor])
}

func (d *themeDialog) headerPrompt() string {
	return d.filter.View()
}

func (d *themeDialog) bodyView() string {
	parts := []string{
		dialogSectionStyle(d.theme).Render("THEMES"),
		d.renderThemeRows(),
	}
	if buttons := d.renderButtons(); buttons != "" {
		parts = append(parts, "", buttons)
	}
	return strings.Join(parts, "\n")
}

func (d *themeDialog) hintView() string {
	return "↑/↓ select • tab buttons • enter apply • esc close"
}

func (d *themeDialog) dialogWidth() int {
	return desiredDialogWidth(d.frameWidth, 32, 72)
}

func (d *themeDialog) dialogContentMinBodyHeight() int {
	contentHeight := min(max(d.frameHeight-2, 1), commandPaletteDefaultModalHeight-2)
	reserved := 3
	return max(contentHeight-reserved, 1)
}

func (d *themeDialog) refilter() {
	query := strings.TrimSpace(d.filter.Value())
	if query == "" {
		d.filteredThemes = append(d.filteredThemes[:0], d.themeItems...)
	} else {
		d.filteredThemes = d.filteredThemes[:0]
		for _, item := range d.themeItems {
			if ok, _ := fuzzyScore(query, item.DisplayName); ok {
				d.filteredThemes = append(d.filteredThemes, item)
			}
		}
	}
	d.resetWindow(len(d.filteredThemes))
}

func (d *themeDialog) activeButtons() []paletteButton {
	return []paletteButton{{id: "select", label: "Select"}, {id: "cancel", label: "Cancel"}}
}

func (d *themeDialog) focusedButtonIndex() int {
	return d.paletteListState.focusedButtonIndex(1, len(d.activeButtons()))
}

func (d *themeDialog) moveFocus(delta int) tea.Cmd {
	total := 1 + len(d.activeButtons())
	d.paletteListState.moveFocus(delta, total)
	return d.syncFocus()
}

func (d *themeDialog) syncFocus() tea.Cmd {
	d.resizeInputs()
	if d.focusIndex == 0 {
		return d.filter.Focus()
	}
	d.filter.Blur()
	return nil
}

func (d *themeDialog) resizeInputs() {
	d.filter.SetWidth(max(d.dialogWidth()-8, 18))
}

func (d *themeDialog) configureInputs() {
	d.filter.Placeholder = "filter themes"
}

func (d *themeDialog) visibleThemes() []themeItem {
	start, end := d.visibleRange(len(d.filteredThemes))
	return d.filteredThemes[start:end]
}

func (d *themeDialog) renderButtons() string {
	buttons := d.activeButtons()
	rendered := make([]string, 0, len(buttons))
	buttonFocus := d.focusedButtonIndex()
	for idx, button := range buttons {
		rendered = append(rendered, renderPaletteButton(d.theme, button.label, idx == buttonFocus))
	}
	return strings.Join(rendered, "  ")
}
