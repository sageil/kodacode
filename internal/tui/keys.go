package tui

import (
	"charm.land/bubbles/v2/key"
)

// KeyMap holds the global keybindings for the TUI.
// Dialog shortcuts (agents, models, sessions, themes, etc.) are accessed via
// slash commands only. Ctrl keys are reserved for textarea navigation.
type KeyMap struct {
	Quit            key.Binding
	CancelStream    key.Binding
	ScrollUp        key.Binding
	ScrollDown      key.Binding
	CycleAgent      key.Binding
	PasteClipboard  key.Binding
	OpenPalette     key.Binding
	ToggleCollapse  key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		CancelStream: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("esc", "cancel streaming"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("pgup"),
			key.WithHelp("pgup", "scroll up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("pgdn"),
			key.WithHelp("pgdn", "scroll down"),
		),
		CycleAgent: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "cycle agent"),
		),
		PasteClipboard: key.NewBinding(
			key.WithKeys("ctrl+y"),
			key.WithHelp("ctrl+y", "paste clipboard"),
		),
		OpenPalette: key.NewBinding(
			key.WithKeys("ctrl+p"),
			key.WithHelp("ctrl+p", "command palette"),
		),
		ToggleCollapse: key.NewBinding(
			key.WithKeys("ctrl+e"),
			key.WithHelp("ctrl+e", "expand/collapse all"),
		),
	}
}
