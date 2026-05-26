package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type skillsDialogResult struct {
	SkillIDs []string
}

type skillsDialog struct {
	id          string
	frameWidth  int
	frameHeight int
	theme       *theme.Theme

	paletteListState

	filter textinput.Model

	skillItems     []skillItem
	filteredSkills []skillItem
	selectedSkills map[string]bool
	initialSkills  map[string]bool
	skillScope     string
}

func newSkillsDialog(items []skillItem, current []string, th *theme.Theme) *skillsDialog {
	filter := newDialogTextInput(th, 128)
	filter.Focus()
	dialog := &skillsDialog{
		id:          dialogIDSkills,
		frameWidth:  96,
		frameHeight: 32,
		theme:       th,
		paletteListState: newPaletteListState(
			commandPaletteMaxVisible,
		),
		filter:         filter,
		skillItems:     append([]skillItem(nil), items...),
		filteredSkills: append([]skillItem(nil), items...),
		selectedSkills: make(map[string]bool, len(current)),
		initialSkills:  make(map[string]bool, len(current)),
	}
	for _, id := range current {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		dialog.selectedSkills[id] = true
		dialog.initialSkills[id] = true
	}
	dialog.configureInputs()
	dialog.syncFocus()
	return dialog
}

func (d *skillsDialog) ID() string { return d.id }

func (d *skillsDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	applyDialogInputTheme(&d.filter, th)
}

func (d *skillsDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.resizeInputs()
}

func (d *skillsDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := d.dialogWidth()
	content := renderPaletteDialogContentSized(d.theme, max(width-dialogFrameInset*2, 1), dialogPaletteFrame{
		Prompt: d.headerPrompt(),
		Body:   d.bodyView(),
		Hint:   d.hintView(),
	}, d.dialogContentMinBodyHeight())
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *skillsDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		return d.updateKey(typed)
	default:
		return d.updateInputs(msg)
	}
}

func (d *skillsDialog) updateKey(msg tea.KeyPressMsg) (dialogModel, tea.Cmd) {
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
	case " ":
		if d.focusedButtonIndex() < 0 {
			d.toggleSelectedSkill()
			return d, nil
		}
	case "[":
		if d.focusedButtonIndex() < 0 {
			d.cycleSkillScope(-1)
			return d, nil
		}
	case "]":
		if d.focusedButtonIndex() < 0 {
			d.cycleSkillScope(1)
			return d, nil
		}
	}
	return d.updateInputs(msg)
}

func (d *skillsDialog) updateInputs(msg tea.Msg) (dialogModel, tea.Cmd) {
	cmds := make([]tea.Cmd, 1)
	prevFilter := d.filter.Value()
	d.filter, cmds[0] = d.filter.Update(msg)
	if d.filter.Value() != prevFilter {
		d.refilter()
	}
	return d, tea.Batch(cmds...)
}

func (d *skillsDialog) handleEnter() (dialogModel, tea.Cmd) {
	if buttonIdx := d.focusedButtonIndex(); buttonIdx >= 0 {
		return d.activateButton(buttonIdx)
	}
	d.toggleSelectedSkill()
	return d, nil
}

func (d *skillsDialog) handleUp() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(-1)
	}
	d.moveCursor(-1, len(d.filteredSkills))
	return d, nil
}

func (d *skillsDialog) handleDown() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(1)
	}
	d.moveCursor(1, len(d.filteredSkills))
	return d, nil
}

func (d *skillsDialog) activateButton(buttonIdx int) (dialogModel, tea.Cmd) {
	buttons := d.activeButtons()
	if buttonIdx < 0 || buttonIdx >= len(buttons) {
		return d, nil
	}
	switch buttons[buttonIdx].id {
	case "apply":
		return d, closeDialog(d.id, skillsDialogResult{SkillIDs: d.selectedSkillIDs()})
	case "clear":
		return d, closeDialog(d.id, skillsDialogResult{})
	case "cancel":
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d *skillsDialog) headerPrompt() string {
	return d.filter.View()
}

func (d *skillsDialog) bodyView() string {
	parts := []string{d.renderSkillRows()}
	if buttons := d.renderButtons(); buttons != "" {
		parts = append(parts, buttons)
	}
	return strings.Join(parts, "\n\n")
}

func (d *skillsDialog) hintView() string {
	return "↑/↓ navigate • space/enter toggle • [ ] scope • tab buttons • esc close"
}

func (d *skillsDialog) dialogWidth() int {
	return desiredDialogWidth(d.frameWidth, 68, 120)
}

func (d *skillsDialog) dialogContentMinBodyHeight() int {
	contentHeight := min(max(d.frameHeight-2, 1), commandPaletteDefaultModalHeight-2)
	reserved := 3
	return max(contentHeight-reserved, 1)
}

func (d *skillsDialog) refilter() {
	query := strings.TrimSpace(d.filter.Value())
	d.filteredSkills = d.filteredSkills[:0]
	for _, item := range d.skillItems {
		if d.skillScope != "" && item.Source != d.skillScope {
			continue
		}
		if query == "" {
			d.filteredSkills = append(d.filteredSkills, item)
			continue
		}
		terms := item.ID + " " + item.Description + " " + item.Source
		if ok, _ := fuzzyScore(query, terms); ok {
			d.filteredSkills = append(d.filteredSkills, item)
		}
	}
	d.resetWindow(len(d.filteredSkills))
}

func (d *skillsDialog) activeButtons() []paletteButton {
	return []paletteButton{{id: "apply", label: "Apply"}, {id: "clear", label: "Clear"}, {id: "cancel", label: "Cancel"}}
}

func (d *skillsDialog) focusedButtonIndex() int {
	return d.paletteListState.focusedButtonIndex(1, len(d.activeButtons()))
}

func (d *skillsDialog) moveFocus(delta int) tea.Cmd {
	total := 1 + len(d.activeButtons())
	d.paletteListState.moveFocus(delta, total)
	return d.syncFocus()
}

func (d *skillsDialog) syncFocus() tea.Cmd {
	d.resizeInputs()
	if d.focusIndex == 0 {
		return d.filter.Focus()
	}
	d.filter.Blur()
	return nil
}

func (d *skillsDialog) resizeInputs() {
	d.filter.SetWidth(max(d.dialogWidth()-8, 18))
}

func (d *skillsDialog) configureInputs() {
	d.filter.Placeholder = "filter skills"
}

func (d *skillsDialog) countBySource(source string) int {
	count := 0
	for _, item := range d.skillItems {
		if item.Source == source {
			count++
		}
	}
	return count
}

func (d *skillsDialog) cycleSkillScope(delta int) {
	scopes := []string{"", "project", "global"}
	for i, s := range scopes {
		if s == d.skillScope {
			d.skillScope = scopes[(i+delta+len(scopes))%len(scopes)]
			d.refilter()
			return
		}
	}
	d.skillScope = ""
	d.refilter()
}

func (d *skillsDialog) toggleSelectedSkill() {
	if d.cursor < 0 || d.cursor >= len(d.filteredSkills) {
		return
	}
	id := strings.TrimSpace(d.filteredSkills[d.cursor].ID)
	if id == "" {
		return
	}
	if d.selectedSkills[id] {
		delete(d.selectedSkills, id)
		return
	}
	d.selectedSkills[id] = true
}

func (d *skillsDialog) skillSelected(id string) bool {
	return d.selectedSkills[strings.TrimSpace(id)]
}

func (d *skillsDialog) selectedSkillIDs() []string {
	if len(d.selectedSkills) == 0 {
		return nil
	}
	selected := make([]string, 0, len(d.selectedSkills))
	for _, item := range d.skillItems {
		id := strings.TrimSpace(item.ID)
		if id == "" || !d.selectedSkills[id] {
			continue
		}
		selected = append(selected, id)
	}
	return selected
}

func (d *skillsDialog) visibleSkills() []skillItem {
	start, end := d.visibleRange(len(d.filteredSkills))
	return d.filteredSkills[start:end]
}

func (d *skillsDialog) renderButtons() string {
	buttons := d.activeButtons()
	rendered := make([]string, 0, len(buttons))
	buttonFocus := d.focusedButtonIndex()
	for idx, button := range buttons {
		rendered = append(rendered, renderPaletteButton(d.theme, button.label, idx == buttonFocus))
	}
	return strings.Join(rendered, "  ")
}
