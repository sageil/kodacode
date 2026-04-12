package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type dialogClosedMsg struct {
	id     string
	result any // nil = cancelled
}

type dialogKeys struct {
	Up      key.Binding
	Down    key.Binding
	Select  key.Binding
	Toggle  key.Binding
	Confirm key.Binding
	Cancel  key.Binding
}

func defaultDialogKeys() dialogKeys {
	return dialogKeys{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space", " "),
			key.WithHelp("space", "toggle"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

// filterDialogKeys returns keybindings for dialogs that have a text filter
// input. The vim-motion aliases "j" and "k" are intentionally omitted so that
// those characters are forwarded to the textinput instead of consumed as
// navigation shortcuts.
func filterDialogKeys() dialogKeys {
	return dialogKeys{
		Up: key.NewBinding(
			key.WithKeys("up"),
			key.WithHelp("↑", "up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down"),
			key.WithHelp("↓", "down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select"),
		),
		Toggle: key.NewBinding(
			key.WithKeys("space", " "),
			key.WithHelp("space", "toggle"),
		),
		Confirm: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "confirm"),
		),
		Cancel: key.NewBinding(
			key.WithKeys("esc", "ctrl+c"),
			key.WithHelp("esc", "cancel"),
		),
	}
}

func dialogStyle(th *theme.Theme, width int) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorFrom(th, "primary", lipgloss.Color("62"))).
		Padding(1, 3).
		Width(width)
}

func dropShadow(box string, th *theme.Theme) string {
	lines := strings.Split(box, "\n")
	if len(lines) == 0 {
		return box
	}
	// Find the width of the widest line.
	maxW := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > maxW {
			maxW = w
		}
	}
	shadowColor := colorFrom(th, "subtext", lipgloss.Color("241"))
	shadow := " " + lipgloss.NewStyle().Foreground(shadowColor).Render(strings.Repeat("░", maxW))
	return box + "\n" + shadow
}

func titleStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(colorFrom(th, "primary", lipgloss.Color("62")))
}

func itemStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorFrom(th, "subtext", lipgloss.Color("250")))
}

func selectedItemStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().Bold(true).Foreground(colorFrom(th, "primary", lipgloss.Color("212")))
}

func checkedItemStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorFrom(th, "success", lipgloss.Color("76")))
}

func hintStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorFrom(th, "subtext", lipgloss.Color("241")))
}

func closeDialog(id string, result any) tea.Cmd {
	return func() tea.Msg {
		return dialogClosedMsg{id: id, result: result}
	}
}
