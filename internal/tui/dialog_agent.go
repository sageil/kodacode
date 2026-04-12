package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type AgentItem struct {
	ID          string
	Name        string
	Description string
}

// AgentPickerDialog lets the user choose an agent from a list.
//
// Result type on confirmation: AgentItem.
// Result is nil on cancellation.
type AgentPickerDialog struct {
	id     string
	title  string
	items  []AgentItem
	cursor int
	keys   dialogKeys
	width  int
	theme  *theme.Theme
}

func (d *AgentPickerDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

// NewAgentPickerDialog creates a new agent-picker dialog.
// id is a caller-supplied identifier echoed back in dialogClosedMsg.
// selectedID pre-selects the agent with that ID (cursor starts there).
func NewAgentPickerDialog(id string, items []AgentItem, selectedID string, th *theme.Theme) AgentPickerDialog {
	cursor := 0
	for i, item := range items {
		if item.ID == selectedID {
			cursor = i
			break
		}
	}
	return AgentPickerDialog{
		id:    id,
		title: "Select Agent",
		items: items,
		cursor: cursor,
		keys:  defaultDialogKeys(),
		width: 50,
		theme: th,
	}
}

func (d *AgentPickerDialog) SetWidth(w int) { d.width = w }
func (d AgentPickerDialog) Width() int               { return d.width }

func (d AgentPickerDialog) Init() tea.Cmd { return nil }

func (d AgentPickerDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		if d.cursor < len(d.items)-1 {
			d.cursor++
		}
	case key.Matches(kp, d.keys.Select):
		if len(d.items) > 0 {
			return d, closeDialog(d.id, d.items[d.cursor])
		}
	case key.Matches(kp, d.keys.Cancel):
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d AgentPickerDialog) View() tea.View {
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	title := titleStyle(d.theme).Render(d.title)
	hint := hintStyle(d.theme).Render("↑/↓ navigate • enter select • esc cancel")

	var rows []string
	for i, item := range d.items {
		line := item.Name
		if item.Description != "" {
			line += "  " + hintStyle(d.theme).Render(truncate(item.Description, 30))
		}
		if i == d.cursor {
			rows = append(rows, sel.Render("> "+line))
		} else {
			rows = append(rows, norm.Render("  "+line))
		}
	}

	body := title + "\n\n" + strings.Join(rows, "\n") + "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(box)
}
