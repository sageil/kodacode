package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type sessionsDialogMode int

const (
	sessionsDialogList sessionsDialogMode = iota
	sessionsDialogCreate
	sessionsDialogDelete
	sessionsDialogPurge
)

type sessionsDialogResult struct {
	OpenSessionID string
	Create        bool
	NewPrompt     string
	DeleteID      string
	PurgeIDs      []string
}

type sessionsDialog struct {
	id          string
	mode        sessionsDialogMode
	frameWidth  int
	frameHeight int
	theme       *theme.Theme

	paletteListState
	purgeCursor int
	purgeAges   []time.Duration

	filter textinput.Model
	prompt textinput.Model

	sessionItems     []sessionItem
	filteredSessions []sessionItem
}

func newSessionsDialog(items []sessionItem, th *theme.Theme) *sessionsDialog {
	filter := newDialogTextInput(th, 128)
	filter.Focus()
	prompt := newDialogTextInput(th, 256)
	dialog := &sessionsDialog{
		id:               dialogIDSessions,
		mode:             sessionsDialogList,
		frameWidth:       96,
		frameHeight:      32,
		theme:            th,
		paletteListState: newPaletteListState(commandPaletteMaxVisible),
		filter:           filter,
		prompt:           prompt,
		sessionItems:     append([]sessionItem(nil), items...),
		filteredSessions: append([]sessionItem(nil), items...),
		purgeAges:        []time.Duration{7 * 24 * time.Hour, 30 * 24 * time.Hour, 180 * 24 * time.Hour, 0},
	}
	dialog.configureInputs()
	dialog.syncFocus()
	return dialog
}

func (d *sessionsDialog) ID() string { return d.id }

func (d *sessionsDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	applyDialogInputTheme(&d.filter, th)
	applyDialogInputTheme(&d.prompt, th)
}

func (d *sessionsDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.resizeInputs()
}

func (d *sessionsDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := desiredDialogWidth(d.frameWidth, 42, 112)
	content := renderPaletteDialogContentSized(d.theme, max(width-dialogFrameInset*2, 1), dialogPaletteFrame{
		Prompt: d.headerPrompt(),
		Body:   d.bodyView(),
		Hint:   d.hintView(),
	}, d.dialogContentMinBodyHeight())
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *sessionsDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		return d.updateKey(typed)
	default:
		return d.updateInputs(msg)
	}
}

func (d *sessionsDialog) updateKey(msg tea.KeyPressMsg) (dialogModel, tea.Cmd) {
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

func (d *sessionsDialog) updateInputs(msg tea.Msg) (dialogModel, tea.Cmd) {
	inputs := d.activeInputs()
	if len(inputs) == 0 {
		return d, nil
	}
	cmds := make([]tea.Cmd, len(inputs))
	prevFilter := d.filter.Value()
	for idx, input := range inputs {
		*input, cmds[idx] = input.Update(msg)
	}
	if d.mode == sessionsDialogList && d.filter.Value() != prevFilter {
		d.refilter()
	}
	return d, tea.Batch(cmds...)
}

func (d *sessionsDialog) handleEscape() (dialogModel, tea.Cmd) {
	switch d.mode {
	case sessionsDialogCreate:
		d.prompt.SetValue("")
		d.mode = sessionsDialogList
		d.focusIndex = 0
		return d, d.syncFocus()
	case sessionsDialogDelete, sessionsDialogPurge:
		d.mode = sessionsDialogList
		d.focusIndex = 0
		return d, d.syncFocus()
	default:
		return d, closeDialog(d.id, nil)
	}
}

func (d *sessionsDialog) handleEnter() (dialogModel, tea.Cmd) {
	if buttonIdx := d.focusedButtonIndex(); buttonIdx >= 0 {
		return d.activateButton(buttonIdx)
	}
	switch d.mode {
	case sessionsDialogList:
		return d.activateSessionOpen()
	case sessionsDialogCreate, sessionsDialogPurge:
		return d, d.moveFocus(1)
	default:
		return d, nil
	}
}

func (d *sessionsDialog) handleUp() (dialogModel, tea.Cmd) {
	if d.mode == sessionsDialogPurge {
		d.focusIndex = 0
		if d.purgeCursor > 0 {
			d.purgeCursor--
		}
		return d, nil
	}
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(-1)
	}
	if d.mode == sessionsDialogList && d.cursor > 0 {
		d.moveCursor(-1, d.totalSessionRows())
	}
	return d, nil
}

func (d *sessionsDialog) handleDown() (dialogModel, tea.Cmd) {
	if d.mode == sessionsDialogPurge {
		d.focusIndex = 0
		if d.purgeCursor < len(d.purgeAges)-1 {
			d.purgeCursor++
		}
		return d, nil
	}
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(1)
	}
	if d.mode == sessionsDialogList && d.cursor < d.totalSessionRows()-1 {
		d.moveCursor(1, d.totalSessionRows())
	}
	return d, nil
}

func (d *sessionsDialog) handleShortcutDelete() (dialogModel, tea.Cmd) {
	if d.mode == sessionsDialogList && d.cursor < len(d.filteredSessions) {
		d.mode = sessionsDialogDelete
		d.focusIndex = 0
		return d, d.syncFocus()
	}
	return d, nil
}

func (d *sessionsDialog) handleShortcutPurge() (dialogModel, tea.Cmd) {
	if d.mode == sessionsDialogList {
		d.mode = sessionsDialogPurge
		d.focusIndex = 0
		d.purgeCursor = 0
		return d, d.syncFocus()
	}
	return d, nil
}

func (d *sessionsDialog) activateButton(buttonIdx int) (dialogModel, tea.Cmd) {
	buttons := d.activeButtons()
	if buttonIdx < 0 || buttonIdx >= len(buttons) {
		return d, nil
	}
	switch buttons[buttonIdx].id {
	case "open-session":
		return d.activateSessionOpen()
	case "new":
		d.mode = sessionsDialogCreate
		d.focusIndex = 0
		return d, d.syncFocus()
	case "delete":
		if d.mode == sessionsDialogList {
			if d.cursor >= len(d.filteredSessions) {
				return d, nil
			}
			d.mode = sessionsDialogDelete
			d.focusIndex = 0
			return d, d.syncFocus()
		}
		if d.mode == sessionsDialogDelete && d.cursor < len(d.filteredSessions) {
			return d, closeDialog(d.id, sessionsDialogResult{DeleteID: d.filteredSessions[d.cursor].ID})
		}
	case "purge":
		if d.mode == sessionsDialogList {
			d.mode = sessionsDialogPurge
			d.focusIndex = 0
			d.purgeCursor = 0
			return d, d.syncFocus()
		}
		if d.mode == sessionsDialogPurge {
			return d, closeDialog(d.id, sessionsDialogResult{PurgeIDs: d.purgeIDs(d.purgeAges[d.purgeCursor])})
		}
	case "create":
		return d, closeDialog(d.id, sessionsDialogResult{Create: true, NewPrompt: strings.TrimSpace(d.prompt.Value())})
	case "back":
		d.prompt.SetValue("")
		d.mode = sessionsDialogList
		d.focusIndex = 0
		return d, d.syncFocus()
	case "cancel":
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d *sessionsDialog) activateSessionOpen() (dialogModel, tea.Cmd) {
	if d.cursor == len(d.filteredSessions) {
		d.mode = sessionsDialogCreate
		d.focusIndex = 0
		return d, d.syncFocus()
	}
	if len(d.filteredSessions) == 0 || d.cursor >= len(d.filteredSessions) {
		return d, nil
	}
	return d, closeDialog(d.id, sessionsDialogResult{OpenSessionID: d.filteredSessions[d.cursor].ID})
}

func (d *sessionsDialog) headerPrompt() string {
	switch d.mode {
	case sessionsDialogCreate:
		return "New Session"
	case sessionsDialogDelete:
		return "Delete Session"
	case sessionsDialogPurge:
		return "Purge Sessions"
	default:
		return d.filter.View()
	}
}

func (d *sessionsDialog) bodyView() string {
	switch d.mode {
	case sessionsDialogCreate:
		return strings.Join([]string{
			dialogHintStyle(d.theme).Render("Optional opening prompt"),
			d.prompt.View(),
			"",
			d.renderButtons(),
		}, "\n")
	case sessionsDialogDelete:
		label := "Delete this session?"
		if d.cursor < len(d.filteredSessions) {
			title := strings.TrimSpace(d.filteredSessions[d.cursor].Title)
			if title == "" {
				title = "Workspace session"
			}
			label = "Delete “" + title + "”?"
		}
		return strings.Join([]string{label, "", d.renderButtons()}, "\n")
	case sessionsDialogPurge:
		return strings.Join([]string{
			dialogSectionStyle(d.theme).Render("PURGE"),
			d.renderPurgeRows(),
			"",
			d.renderButtons(),
		}, "\n")
	default:
		body := []string{
			dialogSectionStyle(d.theme).Render("SESSIONS"),
			d.renderSessionRows(),
		}
		if buttons := d.renderButtons(); buttons != "" {
			body = append(body, "", buttons)
		}
		return strings.Join(body, "\n")
	}
}

func (d *sessionsDialog) hintView() string {
	switch d.mode {
	case sessionsDialogCreate:
		return "tab move • enter confirm • esc back"
	case sessionsDialogDelete:
		return "tab move • enter confirm • esc back"
	case sessionsDialogPurge:
		return "↑/↓ choose • tab buttons • enter confirm • esc back"
	default:
		return "↑/↓ select • tab buttons • enter confirm • esc back"
	}
}

func (d *sessionsDialog) dialogContentMinBodyHeight() int {
	if d.mode != sessionsDialogList {
		return 0
	}
	contentHeight := min(max(d.frameHeight-2, 1), commandPaletteDefaultModalHeight-2)
	reserved := 3
	return max(contentHeight-reserved, 1)
}

func (d *sessionsDialog) refilter() {
	query := strings.TrimSpace(d.filter.Value())
	if query == "" {
		d.filteredSessions = append(d.filteredSessions[:0], d.sessionItems...)
	} else {
		d.filteredSessions = d.filteredSessions[:0]
		for _, item := range d.sessionItems {
			if ok, _ := fuzzyScore(query, item.Title); ok {
				d.filteredSessions = append(d.filteredSessions, item)
			}
		}
	}
	d.resetWindow(d.totalSessionRows())
}

func (d *sessionsDialog) activeInputs() []*textinput.Model {
	switch d.mode {
	case sessionsDialogList:
		return []*textinput.Model{&d.filter}
	case sessionsDialogCreate:
		return []*textinput.Model{&d.prompt}
	default:
		return nil
	}
}

func (d *sessionsDialog) activeButtons() []paletteButton {
	switch d.mode {
	case sessionsDialogCreate:
		return []paletteButton{{id: "create", label: "Create"}, {id: "back", label: "Back"}}
	case sessionsDialogDelete:
		return []paletteButton{{id: "delete", label: "Delete"}, {id: "back", label: "Back"}}
	case sessionsDialogPurge:
		return []paletteButton{{id: "purge", label: "Purge"}, {id: "back", label: "Back"}}
	default:
		return []paletteButton{
			{id: "open-session", label: "Open"},
			{id: "new", label: "New"},
			{id: "delete", label: "Delete"},
			{id: "purge", label: "Purge"},
			{id: "cancel", label: "Cancel"},
		}
	}
}

func (d *sessionsDialog) focusedButtonIndex() int {
	return d.paletteListState.focusedButtonIndex(len(d.activeInputs())+d.virtualFocusSlots(), len(d.activeButtons()))
}

func (d *sessionsDialog) moveFocus(delta int) tea.Cmd {
	total := len(d.activeInputs()) + d.virtualFocusSlots() + len(d.activeButtons())
	if total == 0 {
		return nil
	}
	d.paletteListState.moveFocus(delta, total)
	return d.syncFocus()
}

func (d *sessionsDialog) syncFocus() tea.Cmd {
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

func (d *sessionsDialog) virtualFocusSlots() int {
	if d.mode == sessionsDialogPurge {
		return 1
	}
	return 0
}

func (d *sessionsDialog) resizeInputs() {
	fieldWidth := max(desiredDialogWidth(d.frameWidth, 42, 112)-8, 18)
	d.filter.SetWidth(fieldWidth)
	d.prompt.SetWidth(fieldWidth)
}

func (d *sessionsDialog) configureInputs() {
	d.filter.Placeholder = "filter sessions"
	d.prompt.Placeholder = "optional opening prompt"
}

func (d *sessionsDialog) visibleSessions() []sessionItem {
	start, end := d.visibleRange(len(d.filteredSessions))
	return d.filteredSessions[start:end]
}

func (d *sessionsDialog) totalSessionRows() int {
	return len(d.filteredSessions) + 1
}

func (d *sessionsDialog) purgeIDs(age time.Duration) []string {
	if age == 0 {
		ids := make([]string, 0, len(d.sessionItems))
		for _, item := range d.sessionItems {
			ids = append(ids, item.ID)
		}
		return ids
	}
	var ids []string
	now := time.Now()
	for _, item := range d.sessionItems {
		if updatedAt := time.Unix(item.UpdatedAt, 0); !updatedAt.IsZero() && now.Sub(updatedAt) > age {
			ids = append(ids, item.ID)
		}
	}
	return ids
}

func (d *sessionsDialog) renderSessionRows() string {
	selected := dialogSelectedItemStyle(d.theme)
	normal := dialogItemStyle(d.theme)
	muted := dialogHintStyle(d.theme)
	rows := make([]string, 0, len(d.visibleSessions())+2)
	for idx, item := range d.visibleSessions() {
		titleText := strings.TrimSpace(item.Title)
		if titleText == "" {
			titleText = "Workspace session"
		}
		age := relativeTimeUnix(item.UpdatedAt)
		line := joinBar(titleText, age, max(desiredDialogWidth(d.frameWidth, 42, 112)-8, 12))
		if d.offset+idx == d.cursor {
			rows = append(rows, selected.Render("> "+line))
		} else {
			rows = append(rows, normal.Render("  "+line))
		}
	}
	if len(d.filteredSessions) == 0 {
		rows = append(rows, muted.Render("  no sessions"))
	}
	if d.cursor == len(d.filteredSessions) {
		rows = append(rows, selected.Render("> + New session"))
	} else {
		rows = append(rows, normal.Render("  + New session"))
	}
	return strings.Join(rows, "\n")
}

func (d *sessionsDialog) renderPurgeRows() string {
	selected := dialogSelectedItemStyle(d.theme)
	normal := dialogItemStyle(d.theme)
	var rows []string
	for idx, age := range d.purgeAges {
		label := purgeLabel(age)
		count := len(d.purgeIDs(age))
		line := fmt.Sprintf("%s (%d)", label, count)
		if idx == d.purgeCursor {
			rows = append(rows, selected.Render("> "+line))
		} else {
			rows = append(rows, normal.Render("  "+line))
		}
	}
	return strings.Join(rows, "\n")
}

func (d *sessionsDialog) renderButtons() string {
	buttons := d.activeButtons()
	if len(buttons) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(buttons))
	buttonFocus := d.focusedButtonIndex()
	for idx, button := range buttons {
		rendered = append(rendered, renderPaletteButton(d.theme, button.label, idx == buttonFocus))
	}
	return strings.Join(rendered, "  ")
}
