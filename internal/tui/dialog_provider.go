package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type ProviderConnectResult struct {
	ProviderID string
	AuthType   string
	APIKey     string
	BaseURL    string
	Remove     bool
}

type providerStep int

const (
	stepSelectProvider providerStep = iota
	stepSelectAuth
	stepProviderIDInput
	stepAPIKeyInput
	stepBaseURLInput
	stepCustomInput
	stepConfirmRemove
)

type authMethod struct {
	Type  string // "oauth" or "api"
	Label string
}

type providerDef struct {
	ID        string
	Name      string
	Auths     []authMethod
	Connected bool
}

var knownProviders = []providerDef{
	{ID: "anthropic", Name: "Anthropic", Auths: []authMethod{
		{Type: "api", Label: "API Key"},
	}},
	{ID: "openai", Name: "OpenAI", Auths: []authMethod{
		{Type: "oauth", Label: "OpenAI (ChatGPT Plus/Pro)"},
		{Type: "api", Label: "API Key"},
	}},
	{ID: "google", Name: "Google", Auths: []authMethod{
		{Type: "api", Label: "API Key"},
	}},
	{ID: "github-copilot", Name: "GitHub Copilot", Auths: []authMethod{
		{Type: "copilot-neovim", Label: "Import from Neovim/Vim"},
		{Type: "copilot-opencode", Label: "Import from opencode"},
		{Type: "copilot-manual", Label: "Paste token manually (gho_…)"},
	}},
	{ID: "custom", Name: "Custom (OpenAI-compatible)", Auths: []authMethod{
		{Type: "api", Label: "API Key + Base URL"},
	}},
}

// ProviderConnectDialog is a multi-step modal that walks the user through
// connecting or removing LLM providers. The first screen shows two sections:
// connected providers (removable) and available providers (connectable).
type ProviderConnectDialog struct {
	id        string
	step      providerStep
	cursor    int
	items     []providerListItem // flat list: headers + providers
	selected  providerDef
	selAuth   authMethod
	input     textinput.Model
	customID  string
	apiKey    string
	width     int
	theme     *theme.Theme
	keys      dialogKeys

	customFields [3]textinput.Model // ID, API Key, Base URL
	customFocus  int                // 0=ID, 1=Key, 2=URL
}

type providerListItem struct {
	header   string     // non-empty for section headers (not selectable)
	provider *providerDef
}

func (d *ProviderConnectDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

func NewProviderConnectDialog(id string, configuredProviders []string, th *theme.Theme) ProviderConnectDialog {
	ti := textinput.New()
	ti.Placeholder = "paste key here..."
	ti.CharLimit = 256

	configured := make(map[string]bool, len(configuredProviders))
	for _, pid := range configuredProviders {
		configured[pid] = true
	}

	var connected, available []providerDef
	knownIDs := make(map[string]bool, len(knownProviders))
	for _, p := range knownProviders {
		knownIDs[p.ID] = true
		cp := p
		if configured[p.ID] {
			cp.Connected = true
			connected = append(connected, cp)
		} else if p.ID != "custom" {
			available = append(available, cp)
		}
	}
	// Custom entry is always in available.
	for _, p := range knownProviders {
		if p.ID == "custom" {
			available = append(available, p)
			break
		}
	}
	// Add configured providers not in the known list.
	for _, pid := range configuredProviders {
		if !knownIDs[pid] {
			connected = append(connected, providerDef{
				ID:        pid,
				Name:      pid,
				Connected: true,
				Auths:     []authMethod{{Type: "api", Label: "API Key + Base URL"}},
			})
		}
	}

	var items []providerListItem
	if len(connected) > 0 {
		items = append(items, providerListItem{header: "Connected"})
		for i := range connected {
			items = append(items, providerListItem{provider: &connected[i]})
		}
	}
	if len(available) > 0 {
		items = append(items, providerListItem{header: "Available"})
		for i := range available {
			items = append(items, providerListItem{provider: &available[i]})
		}
	}

	cursor := 0
	for i, item := range items {
		if item.provider != nil {
			cursor = i
			break
		}
	}

	w := computeProviderDialogWidth(items)
	ti.SetWidth(w - 10) // padding + border

	fieldWidth := w - 10
	idField := textinput.New()
	idField.Placeholder = "e.g. zai-coding-plan"
	idField.CharLimit = 64
	idField.SetWidth(fieldWidth)

	keyField := textinput.New()
	keyField.Placeholder = "paste key here..."
	keyField.CharLimit = 256
	keyField.SetWidth(fieldWidth)

	urlField := textinput.New()
	urlField.Placeholder = "https://api.example.com/v1"
	urlField.CharLimit = 256
	urlField.SetWidth(fieldWidth)

	return ProviderConnectDialog{
		id:           id,
		step:         stepSelectProvider,
		items:        items,
		input:        ti,
		keys:         defaultDialogKeys(),
		width:        w,
		theme:        th,
		cursor:       cursor,
		customFields: [3]textinput.Model{idField, keyField, urlField},
	}
}

func computeProviderDialogWidth(items []providerListItem) int {
	const chrome = 8
	w := 80 // wide enough for a typical API key without wrapping
	for _, item := range items {
		var label string
		if item.header != "" {
			label = item.header
		} else if item.provider != nil {
			label = "  * " + item.provider.Name
		}
		if cw := len(label) + chrome; cw > w {
			w = cw
		}
	}
	return w
}

func (d *ProviderConnectDialog) SetWidth(w int) { d.width = w }
func (d ProviderConnectDialog) Width() int               { return d.width }

func (d ProviderConnectDialog) Init() tea.Cmd { return nil }

func (d ProviderConnectDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch d.step {
	case stepSelectProvider:
		return d.updateSelectProvider(msg)
	case stepSelectAuth:
		return d.updateSelectAuth(msg)
	case stepProviderIDInput:
		return d.updateProviderIDInput(msg)
	case stepAPIKeyInput:
		return d.updateAPIKeyInput(msg)
	case stepBaseURLInput:
		return d.updateBaseURLInput(msg)
	case stepCustomInput:
		return d.updateCustomInput(msg)
	case stepConfirmRemove:
		return d.updateConfirmRemove(msg)
	}
	return d, nil
}

func (d ProviderConnectDialog) updateSelectProvider(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch {
	case key.Matches(kp, d.keys.Up):
		d.cursor = d.prevSelectable(d.cursor)
	case key.Matches(kp, d.keys.Down):
		d.cursor = d.nextSelectable(d.cursor)
	case key.Matches(kp, d.keys.Select):
		item := d.items[d.cursor]
		if item.provider == nil {
			return d, nil
		}
		d.selected = *item.provider
		if len(d.selected.Auths) == 1 {
			d.selAuth = d.selected.Auths[0]
			return d.advanceFromAuth()
		}
		d.step = stepSelectAuth
		d.cursor = 0
	case kp.String() == "ctrl+d":
		item := d.items[d.cursor]
		if item.provider != nil && item.provider.Connected {
			d.selected = *item.provider
			d.step = stepConfirmRemove
		}
	case key.Matches(kp, d.keys.Cancel):
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d ProviderConnectDialog) nextSelectable(from int) int {
	for i := from + 1; i < len(d.items); i++ {
		if d.items[i].provider != nil {
			return i
		}
	}
	return from
}

func (d ProviderConnectDialog) prevSelectable(from int) int {
	for i := from - 1; i >= 0; i-- {
		if d.items[i].provider != nil {
			return i
		}
	}
	return from
}

func (d ProviderConnectDialog) updateSelectAuth(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch {
	case key.Matches(kp, d.keys.Up):
		if d.cursor > 0 {
			d.cursor--
		}
	case key.Matches(kp, d.keys.Down):
		if d.cursor < len(d.selected.Auths)-1 {
			d.cursor++
		}
	case key.Matches(kp, d.keys.Select):
		d.selAuth = d.selected.Auths[d.cursor]
		return d.advanceFromAuth()
	case key.Matches(kp, d.keys.Cancel):
		d.step = stepSelectProvider
		d.cursor = 0
	}
	return d, nil
}

func (d ProviderConnectDialog) advanceFromAuth() (tea.Model, tea.Cmd) {
	if d.selAuth.Type == "oauth" {
		return d, closeDialog(d.id, ProviderConnectResult{
			ProviderID: d.selected.ID,
			AuthType:   "oauth",
		})
	}
	// Copilot import: close dialog and let the handler read the token file.
	if d.selAuth.Type == "copilot-neovim" || d.selAuth.Type == "copilot-opencode" {
		return d, closeDialog(d.id, ProviderConnectResult{
			ProviderID: d.selected.ID,
			AuthType:   d.selAuth.Type,
		})
	}
	// Copilot manual: go to API key input for the gho_ token.
	if d.selAuth.Type == "copilot-manual" {
		d.step = stepAPIKeyInput
		d.cursor = 0
		d.input.Placeholder = "gho_..."
		d.input.SetValue("")
		return d, d.input.Focus()
	}
	if d.selected.ID == "custom" {
		d.step = stepCustomInput
		d.customFocus = 0
		for i := range d.customFields {
			d.customFields[i].SetValue("")
			d.customFields[i].Blur()
		}
		return d, d.customFields[0].Focus()
	}
	d.step = stepAPIKeyInput
	d.cursor = 0
	d.input.Placeholder = "paste key here..."
	d.input.SetValue("")
	return d, d.input.Focus()
}

func (d ProviderConnectDialog) updateProviderIDInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kp, d.keys.Cancel):
			d.input.Blur()
			if len(d.selected.Auths) == 1 {
				d.step = stepSelectProvider
			} else {
				d.step = stepSelectAuth
			}
			d.cursor = 0
			return d, nil
		case key.Matches(kp, d.keys.Confirm):
			val := strings.TrimSpace(d.input.Value())
			if val == "" {
				return d, nil
			}
			d.customID = val
			d.step = stepAPIKeyInput
			d.input.Placeholder = "paste key here..."
			d.input.SetValue("")
			return d, d.input.Focus()
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return d, cmd
}

func (d ProviderConnectDialog) updateAPIKeyInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kp, d.keys.Cancel):
			d.input.Blur()
			if len(d.selected.Auths) == 1 {
				d.step = stepSelectProvider
			} else {
				d.step = stepSelectAuth
			}
			d.cursor = 0
			return d, nil
		case key.Matches(kp, d.keys.Confirm):
			val := strings.TrimSpace(d.input.Value())
			if val == "" {
				return d, nil
			}
			d.apiKey = val
			if d.selected.ID == "custom" {
				d.step = stepBaseURLInput
				d.input.Placeholder = "https://api.example.com/v1"
				d.input.SetValue("")
				return d, d.input.Focus()
			}
			return d, closeDialog(d.id, ProviderConnectResult{
				ProviderID: d.selected.ID,
				AuthType:   "api",
				APIKey:     d.apiKey,
			})
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return d, cmd
}

func (d ProviderConnectDialog) updateBaseURLInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kp, d.keys.Cancel):
			d.step = stepAPIKeyInput
			d.input.Placeholder = "paste key here..."
			d.input.SetValue("")
			return d, d.input.Focus()
		case key.Matches(kp, d.keys.Confirm):
			val := strings.TrimSpace(d.input.Value())
			if val == "" {
				return d, nil
			}
			providerID := d.selected.ID
			if d.customID != "" {
				providerID = d.customID
			}
			return d, closeDialog(d.id, ProviderConnectResult{
				ProviderID: providerID,
				AuthType:   "api",
				APIKey:     d.apiKey,
				BaseURL:    val,
			})
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return d, cmd
}

func (d ProviderConnectDialog) updateCustomInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kp, d.keys.Cancel):
			d.customFields[d.customFocus].Blur()
			if len(d.selected.Auths) == 1 {
				d.step = stepSelectProvider
			} else {
				d.step = stepSelectAuth
			}
			d.cursor = 0
			return d, nil
		case kp.String() == "tab", kp.String() == "down":
			if d.customFocus < 2 {
				d.customFields[d.customFocus].Blur()
				d.customFocus++
				return d, d.customFields[d.customFocus].Focus()
			}
			return d, nil
		case kp.String() == "shift+tab", kp.String() == "up":
			if d.customFocus > 0 {
				d.customFields[d.customFocus].Blur()
				d.customFocus--
				return d, d.customFields[d.customFocus].Focus()
			}
			return d, nil
		case key.Matches(kp, d.keys.Confirm):
			id := strings.TrimSpace(d.customFields[0].Value())
			apiKey := strings.TrimSpace(d.customFields[1].Value())
			baseURL := strings.TrimSpace(d.customFields[2].Value())
			if id == "" || apiKey == "" || baseURL == "" {
				// Move focus to first empty field.
				if id == "" {
					d.customFields[d.customFocus].Blur()
					d.customFocus = 0
					return d, d.customFields[0].Focus()
				}
				if apiKey == "" {
					d.customFields[d.customFocus].Blur()
					d.customFocus = 1
					return d, d.customFields[1].Focus()
				}
				d.customFields[d.customFocus].Blur()
				d.customFocus = 2
				return d, d.customFields[2].Focus()
			}
			return d, closeDialog(d.id, ProviderConnectResult{
				ProviderID: id,
				AuthType:   "api",
				APIKey:     apiKey,
				BaseURL:    baseURL,
			})
		}
	}
	var cmd tea.Cmd
	d.customFields[d.customFocus], cmd = d.customFields[d.customFocus].Update(msg)
	return d, cmd
}

func (d ProviderConnectDialog) updateConfirmRemove(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch kp.String() {
	case "y", "enter":
		return d, closeDialog(d.id, ProviderConnectResult{
			ProviderID: d.selected.ID,
			Remove:     true,
		})
	case "n", "esc":
		d.step = stepSelectProvider
		d.cursor = 0
	}
	return d, nil
}

func (d ProviderConnectDialog) View() tea.View {
	var title, body, hint string

	switch d.step {
	case stepSelectProvider:
		title = titleStyle(d.theme).Render("Providers")
		body = d.viewProviderList()
		hint = hintStyle(d.theme).Render("enter connect • ^D remove • esc cancel")

	case stepSelectAuth:
		title = titleStyle(d.theme).Render("Authentication for " + d.selected.Name)
		body = d.viewAuthList()
		hint = hintStyle(d.theme).Render("enter select • esc back")

	case stepProviderIDInput:
		title = titleStyle(d.theme).Render("Enter provider ID")
		body = d.input.View()
		hint = hintStyle(d.theme).Render("enter confirm • esc back")

	case stepAPIKeyInput:
		name := d.selected.Name
		if d.customID != "" {
			name = d.customID
		}
		title = titleStyle(d.theme).Render("Enter API key for " + name)
		body = d.input.View()
		hint = hintStyle(d.theme).Render("enter confirm • esc back")

	case stepBaseURLInput:
		title = titleStyle(d.theme).Render("Enter base URL for " + d.customID)
		body = d.input.View()
		hint = hintStyle(d.theme).Render("enter confirm • esc back")

	case stepCustomInput:
		title = titleStyle(d.theme).Render("Add OpenAI-compatible provider")
		body = d.viewCustomFields()
		hint = hintStyle(d.theme).Render("tab/↑↓ navigate • enter submit • esc back")

	case stepConfirmRemove:
		title = titleStyle(d.theme).Render("Remove Provider")
		body = dangerTextStyle(d.theme).Render(
			fmt.Sprintf("Remove %q? This will delete its config and credentials.", d.selected.Name))
		hint = hintStyle(d.theme).Render("y/enter confirm • n/esc cancel")
	}

	content := title + "\n\n" + body + "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(content)
	return tea.NewView(dropShadow(box, d.theme))
}

func (d ProviderConnectDialog) viewProviderList() string {
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	check := checkedItemStyle(d.theme)
	section := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorFrom(d.theme, "subtext", lipgloss.Color("245"))).
		MarginBottom(0)

	var rows []string
	for i, item := range d.items {
		if item.header != "" {
			if len(rows) > 0 {
				rows = append(rows, "")
			}
			rows = append(rows, section.Render(item.header))
			continue
		}
		p := item.provider
		if p.Connected {
			prefix := "  "
			if i == d.cursor {
				prefix = "> "
			}
			label := prefix + "✓ " + p.Name
			if i == d.cursor {
				rows = append(rows, sel.Render(label))
			} else {
				rows = append(rows, check.Render(label))
			}
		} else {
			if i == d.cursor {
				rows = append(rows, sel.Render("> "+p.Name))
			} else {
				rows = append(rows, norm.Render("  "+p.Name))
			}
		}
	}
	return strings.Join(rows, "\n")
}

func (d ProviderConnectDialog) viewAuthList() string {
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)

	var rows []string
	for i, a := range d.selected.Auths {
		if i == d.cursor {
			rows = append(rows, sel.Render("> "+a.Label))
		} else {
			rows = append(rows, norm.Render("  "+a.Label))
		}
	}
	return strings.Join(rows, "\n")
}

func (d ProviderConnectDialog) viewCustomFields() string {
	labels := [3]string{"Provider ID", "API Key", "Base URL"}
	dim := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "subtext", lipgloss.Color("241")))
	accent := lipgloss.NewStyle().Foreground(colorFrom(d.theme, "primary", lipgloss.Color("62")))

	var rows []string
	for i, label := range labels {
		style := dim
		if i == d.customFocus {
			style = accent
		}
		rows = append(rows, style.Render(label))
		rows = append(rows, d.customFields[i].View())
		if i < 2 {
			rows = append(rows, "")
		}
	}
	return strings.Join(rows, "\n")
}
