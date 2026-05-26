package tui

import tea "charm.land/bubbletea/v2"

func (m Model) handlePaste(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	if m.dialog != nil {
		updated, cmd := m.dialog.Update(msg)
		m.dialog = updated
		return m, cmd
	}

	if m.chrome.focus == focusComposer {
		m.clearComposerError()
		return m, m.handleComposerPaste(msg.String())
	}

	return m, nil
}
