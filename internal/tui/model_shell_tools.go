package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) openShellToolsDialog() tea.Cmd {
	if m == nil {
		return nil
	}
	state := m.projector.Snapshot()
	dialog := newShellToolsDialog(*m, state)
	width, height := dialogRenderSize(*m, state)
	dialog.SetFrame(width, height)
	m.dialog = dialog
	return nil
}

func (m *Model) syncShellToolsDialog() {
	if m == nil {
		return
	}
	dialog, ok := m.dialog.(*shellToolsDialog)
	if !ok {
		return
	}
	dialog.ApplyTheme(m.theme)
	dialog.SetFrame(dialogRenderSize(*m, m.projector.Snapshot()))
	dialog.Sync(*m, m.projector.Snapshot())
}

func (m *Model) openShellToolsDialogResult(result shellToolsDialogResult) tea.Cmd {
	if m == nil {
		return nil
	}
	ref := sessionToolCallRef{
		TurnID: strings.TrimSpace(result.Ref.TurnID),
		CallID: strings.TrimSpace(result.Ref.CallID),
	}
	if ref.TurnID == "" || ref.CallID == "" {
		return nil
	}
	return tea.Batch(m.selectToolCall(ref), m.openToolCallDialog(ref))
}
