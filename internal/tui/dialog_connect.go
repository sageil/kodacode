package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type connectDialogMode int

const (
	connectDialogList connectDialogMode = iota
	connectDialogEdit
	connectDialogRemove
)

type connectDialogResult struct {
	Save   *app.ProviderConnectionInput
	Remove string
}

type openAIAuthDialogRequest struct{}

type gitHubCopilotAuthDialogRequest struct {
	BaseURL string
}

type connectDialog struct {
	id          string
	mode        connectDialogMode
	frameWidth  int
	frameHeight int
	theme       *theme.Theme

	paletteListState

	filter   textinput.Model
	provider textinput.Model
	apiKey   textinput.Model
	baseURL  textinput.Model

	connectItems    []connectDialogEntry
	filteredConnect []connectDialogEntry
	currentConnect  connectDialogEntry
}

func newConnectDialog(items []connectDialogEntry, th *theme.Theme) *connectDialog {
	filter := newDialogTextInput(th, 128)
	filter.Focus()
	provider := newDialogTextInput(th, 64)
	apiKey := newDialogTextInput(th, 256)
	baseURL := newDialogTextInput(th, 256)
	dialog := &connectDialog{
		id:          dialogIDConnect,
		mode:        connectDialogList,
		frameWidth:  96,
		frameHeight: 32,
		theme:       th,
		paletteListState: newPaletteListState(
			commandPaletteMaxVisible,
		),
		filter:          filter,
		provider:        provider,
		apiKey:          apiKey,
		baseURL:         baseURL,
		connectItems:    append([]connectDialogEntry(nil), items...),
		filteredConnect: append([]connectDialogEntry(nil), items...),
	}
	dialog.configureInputs()
	dialog.syncFocus()
	return dialog
}

func (d *connectDialog) ID() string { return d.id }

func (d *connectDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	applyDialogInputTheme(&d.filter, th)
	applyDialogInputTheme(&d.provider, th)
	applyDialogInputTheme(&d.apiKey, th)
	applyDialogInputTheme(&d.baseURL, th)
}

func (d *connectDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.resizeInputs()
}

func (d *connectDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := desiredDialogWidth(d.frameWidth, 48, 112)
	content := renderPaletteDialogContentSized(d.theme, max(width-dialogFrameInset*2, 1), dialogPaletteFrame{
		Prompt: d.headerPrompt(),
		Body:   d.bodyView(),
		Hint:   d.hintView(),
	}, d.dialogContentMinBodyHeight())
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *connectDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		return d.updateKey(typed)
	default:
		return d.updateInputs(msg)
	}
}

func (d *connectDialog) updateKey(msg tea.KeyPressMsg) (dialogModel, tea.Cmd) {
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
	}
	return d.updateInputs(msg)
}

func (d *connectDialog) updateInputs(msg tea.Msg) (dialogModel, tea.Cmd) {
	inputs := d.activeInputs()
	if len(inputs) == 0 {
		return d, nil
	}
	cmds := make([]tea.Cmd, len(inputs))
	prevFilter := d.filter.Value()
	for idx, input := range inputs {
		*input, cmds[idx] = input.Update(msg)
	}
	if d.mode == connectDialogList && d.filter.Value() != prevFilter {
		d.refilter()
	}
	return d, tea.Batch(cmds...)
}

func (d *connectDialog) handleEscape() (dialogModel, tea.Cmd) {
	switch d.mode {
	case connectDialogEdit, connectDialogRemove:
		d.mode = connectDialogList
		d.focusIndex = 0
		return d, d.syncFocus()
	default:
		return d, closeDialog(d.id, nil)
	}
}

func (d *connectDialog) handleEnter() (dialogModel, tea.Cmd) {
	if buttonIdx := d.focusedButtonIndex(); buttonIdx >= 0 {
		return d.activateButton(buttonIdx)
	}
	switch d.mode {
	case connectDialogList:
		return d.activateConnectEdit()
	case connectDialogEdit:
		return d, d.moveFocus(1)
	default:
		return d, nil
	}
}

func (d *connectDialog) handleUp() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(-1)
	}
	if d.mode == connectDialogList && d.cursor > 0 {
		d.moveCursor(-1, len(d.filteredConnect))
	}
	return d, nil
}

func (d *connectDialog) handleDown() (dialogModel, tea.Cmd) {
	if d.focusedButtonIndex() >= 0 {
		return d, d.moveFocus(1)
	}
	if d.mode == connectDialogList && d.cursor < len(d.filteredConnect)-1 {
		d.moveCursor(1, len(d.filteredConnect))
	}
	return d, nil
}

func (d *connectDialog) handleShortcutDelete() (dialogModel, tea.Cmd) {
	if d.mode == connectDialogList && len(d.filteredConnect) > 0 {
		current := d.filteredConnect[d.cursor]
		if !current.connected {
			return d, nil
		}
		d.currentConnect = current
		d.mode = connectDialogRemove
		d.focusIndex = 0
		return d, d.syncFocus()
	}
	return d, nil
}

func (d *connectDialog) activateButton(buttonIdx int) (dialogModel, tea.Cmd) {
	buttons := d.activeButtons()
	if buttonIdx < 0 || buttonIdx >= len(buttons) {
		return d, nil
	}
	switch buttons[buttonIdx].id {
	case "edit":
		return d.activateConnectEdit()
	case "oauth":
		if d.mode == connectDialogEdit && d.currentConnect.preset.ID == "openai" && !d.currentConnect.preset.Custom {
			return d, closeDialog(d.id, openAIAuthDialogRequest{})
		}
	case "save":
		return d.activateConnectSave()
	case "remove":
		if d.mode == connectDialogList {
			if len(d.filteredConnect) == 0 || !d.filteredConnect[d.cursor].connected {
				return d, nil
			}
			d.currentConnect = d.filteredConnect[d.cursor]
			d.mode = connectDialogRemove
			d.focusIndex = 0
			return d, d.syncFocus()
		}
		if d.mode == connectDialogRemove {
			return d, closeDialog(d.id, connectDialogResult{Remove: d.currentConnect.preset.ID})
		}
	case "back":
		d.mode = connectDialogList
		d.focusIndex = 0
		return d, d.syncFocus()
	case "cancel":
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d *connectDialog) activateConnectEdit() (dialogModel, tea.Cmd) {
	if len(d.filteredConnect) == 0 {
		return d, nil
	}
	d.currentConnect = d.filteredConnect[d.cursor]
	if d.currentConnect.preset.ID == "github-copilot" && !d.currentConnect.preset.Custom {
		return d, closeDialog(d.id, gitHubCopilotAuthDialogRequest{
			BaseURL: pickFirstNonBlank(d.currentConnect.baseURL, d.currentConnect.preset.BaseURL),
		})
	}
	d.provider.SetValue(d.currentConnect.preset.ID)
	d.baseURL.SetValue(pickFirstNonBlank(d.currentConnect.baseURL, d.currentConnect.preset.BaseURL))
	d.apiKey.SetValue("")
	d.mode = connectDialogEdit
	d.focusIndex = 0
	return d, d.syncFocus()
}

func (d *connectDialog) activateConnectSave() (dialogModel, tea.Cmd) {
	input := &app.ProviderConnectionInput{
		ProviderID: strings.TrimSpace(d.provider.Value()),
		APIKey:     strings.TrimSpace(d.apiKey.Value()),
		BaseURL:    strings.TrimSpace(d.baseURL.Value()),
	}
	if !d.currentConnect.preset.Custom {
		input.ProviderID = d.currentConnect.preset.ID
	}
	if input.ProviderID == "openai" && input.APIKey != "" &&
		input.BaseURL == provider.DefaultOpenAIOAuthBaseURL() &&
		strings.TrimSpace(d.currentConnect.baseURL) == provider.DefaultOpenAIOAuthBaseURL() {
		input.BaseURL = provider.DefaultOpenAIBaseURL()
	}
	return d, closeDialog(d.id, connectDialogResult{Save: input})
}

func (d *connectDialog) applyDialogStateRefresh(state app.DialogState) {
	d.connectItems = append(d.connectItems[:0], buildConnectEntries(state)...)
	currentID := strings.TrimSpace(d.currentConnect.preset.ID)
	if currentID != "" {
		for _, entry := range d.connectItems {
			if entry.preset.ID == currentID {
				d.currentConnect = entry
				break
			}
		}
	}
	d.refilter()
}

func (d *connectDialog) headerPrompt() string {
	switch d.mode {
	case connectDialogEdit:
		return "Provider Settings"
	case connectDialogRemove:
		return "Remove Provider"
	default:
		return d.filter.View()
	}
}

func (d *connectDialog) bodyView() string {
	switch d.mode {
	case connectDialogEdit:
		return strings.Join([]string{d.renderConnectInputs(), "", d.renderButtons()}, "\n")
	case connectDialogRemove:
		return strings.Join([]string{"Remove provider “" + d.currentConnect.preset.Name + "”?", "", d.renderButtons()}, "\n")
	default:
		body := []string{
			dialogSectionStyle(d.theme).Render("PROVIDERS"),
			d.renderConnectRows(),
		}
		if buttons := d.renderButtons(); buttons != "" {
			body = append(body, "", buttons)
		}
		return strings.Join(body, "\n")
	}
}

func (d *connectDialog) hintView() string {
	switch d.mode {
	case connectDialogEdit:
		return "tab next field • enter confirm • esc back"
	case connectDialogRemove:
		return "tab move • enter confirm • esc back"
	default:
		return "↑/↓ select • tab buttons • enter confirm • esc back"
	}
}

func (d *connectDialog) dialogContentMinBodyHeight() int {
	if d.mode != connectDialogList {
		return 0
	}
	contentHeight := min(max(d.frameHeight-2, 1), commandPaletteDefaultModalHeight-2)
	reserved := 3
	return max(contentHeight-reserved, 1)
}

func (d *connectDialog) refilter() {
	query := strings.TrimSpace(d.filter.Value())
	if query == "" {
		d.filteredConnect = append(d.filteredConnect[:0], d.connectItems...)
	} else {
		d.filteredConnect = d.filteredConnect[:0]
		for _, item := range d.connectItems {
			if ok, _ := fuzzyScore(query, item.preset.Name+" "+item.preset.ID); ok {
				d.filteredConnect = append(d.filteredConnect, item)
			}
		}
	}
	d.resetWindow(len(d.filteredConnect))
}

func (d *connectDialog) activeInputs() []*textinput.Model {
	switch d.mode {
	case connectDialogList:
		return []*textinput.Model{&d.filter}
	case connectDialogEdit:
		inputs := make([]*textinput.Model, 0, 3)
		if d.currentConnect.preset.Custom {
			inputs = append(inputs, &d.provider)
		}
		inputs = append(inputs, &d.apiKey, &d.baseURL)
		return inputs
	default:
		return nil
	}
}

func (d *connectDialog) activeButtons() []paletteButton {
	switch d.mode {
	case connectDialogEdit:
		if d.currentConnect.preset.ID == "openai" && !d.currentConnect.preset.Custom {
			return []paletteButton{{id: "oauth", label: "OAuth"}, {id: "save", label: "Save API Key"}, {id: "back", label: "Back"}}
		}
		return []paletteButton{{id: "save", label: "Save"}, {id: "back", label: "Back"}}
	case connectDialogRemove:
		return []paletteButton{{id: "remove", label: "Remove"}, {id: "back", label: "Back"}}
	default:
		return []paletteButton{{id: "edit", label: "Edit"}, {id: "remove", label: "Remove"}, {id: "cancel", label: "Cancel"}}
	}
}

func (d *connectDialog) focusedButtonIndex() int {
	return d.paletteListState.focusedButtonIndex(len(d.activeInputs()), len(d.activeButtons()))
}

func (d *connectDialog) moveFocus(delta int) tea.Cmd {
	total := len(d.activeInputs()) + len(d.activeButtons())
	if total == 0 {
		return nil
	}
	d.paletteListState.moveFocus(delta, total)
	return d.syncFocus()
}

func (d *connectDialog) syncFocus() tea.Cmd {
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

func (d *connectDialog) resizeInputs() {
	fieldWidth := max(desiredDialogWidth(d.frameWidth, 48, 112)-8, 18)
	for _, input := range []*textinput.Model{&d.filter, &d.provider, &d.apiKey, &d.baseURL} {
		input.SetWidth(fieldWidth)
	}
}

func (d *connectDialog) configureInputs() {
	d.filter.Placeholder = "filter providers"
	d.provider.Placeholder = "provider id"
	d.apiKey.Placeholder = "leave blank to keep existing key"
	d.baseURL.Placeholder = "base url"
}

func (d *connectDialog) visibleConnect() []connectDialogEntry {
	start, end := d.visibleRange(len(d.filteredConnect))
	return d.filteredConnect[start:end]
}

func (d *connectDialog) renderConnectRows() string {
	selected := dialogSelectedItemStyle(d.theme)
	normal := dialogItemStyle(d.theme)
	muted := dialogHintStyle(d.theme)
	var rows []string
	for idx, item := range d.visibleConnect() {
		label := item.preset.Name
		if item.connected {
			label += "  " + muted.Render("(connected)")
		}
		if d.offset+idx == d.cursor {
			rows = append(rows, selected.Render("> "+label))
		} else {
			rows = append(rows, normal.Render("  "+label))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, muted.Render("  no providers"))
	}
	return strings.Join(rows, "\n")
}

func (d *connectDialog) renderConnectInputs() string {
	lines := make([]string, 0, 8)
	if d.currentConnect.preset.Custom {
		lines = append(lines, dialogHintStyle(d.theme).Render("Provider ID"), d.provider.View(), "")
	}
	lines = append(lines,
		dialogHintStyle(d.theme).Render(pickFirstNonBlank(d.currentConnect.preset.APIKeyLabel, "API key")),
		d.apiKey.View(),
		"",
		dialogHintStyle(d.theme).Render("Base URL"),
		d.baseURL.View(),
	)
	return strings.Join(lines, "\n")
}

func (d *connectDialog) renderButtons() string {
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
