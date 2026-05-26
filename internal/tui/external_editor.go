package tui

import (
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"
)

type externalEditorLauncher interface {
	Open(currentText string) tea.Cmd
}

type externalEditorLauncherFunc func(currentText string) tea.Cmd

func (fn externalEditorLauncherFunc) Open(currentText string) tea.Cmd {
	return fn(currentText)
}

type systemExternalEditorLauncher struct{}

func (systemExternalEditorLauncher) Open(currentText string) tea.Cmd {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vim"
	}

	tmpFile, err := os.CreateTemp("", "kodacode-*.md")
	if err != nil {
		return func() tea.Msg { return composerExternalEditorDoneMsg{err: err} }
	}
	tmpPath := tmpFile.Name()
	if currentText != "" {
		_, _ = tmpFile.WriteString(currentText)
	}
	_ = tmpFile.Close()

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		_ = os.Remove(tmpPath)
		return func() tea.Msg { return composerExternalEditorDoneMsg{err: os.ErrInvalid} }
	}
	args := append(parts[1:], tmpPath)
	cmd := exec.Command(parts[0], args...)

	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		defer func() { _ = os.Remove(tmpPath) }()
		if err != nil {
			return composerExternalEditorDoneMsg{err: err}
		}
		data, readErr := os.ReadFile(tmpPath)
		if readErr != nil {
			return composerExternalEditorDoneMsg{err: readErr}
		}
		return composerExternalEditorDoneMsg{text: strings.TrimRight(string(data), "\n")}
	})
}

func (m Model) openComposerExternalEditor() tea.Cmd {
	launcher := m.editor
	if launcher == nil {
		launcher = systemExternalEditorLauncher{}
	}
	return launcher.Open(m.expandComposerPastedText(m.composer.Value()))
}

func (m *Model) applyComposerExternalEditorText(text string) {
	m.resetComposerHistoryRecall()
	m.clearComposerError()
	m.clearComposerPastedText()
	m.dismissComposerPopup()
	m.composer.SetValue(text)
	m.filterPendingFocusPathsForComposerText(text)
	if len(m.composerState.pendingAttachments) == 0 {
		return
	}
	filtered := m.composerState.pendingAttachments[:0]
	for _, attachment := range m.composerState.pendingAttachments {
		tag := strings.TrimSpace(attachment.Tag)
		if tag == "" || !strings.Contains(text, tag) {
			continue
		}
		filtered = append(filtered, attachment)
	}
	if len(filtered) == 0 {
		m.composerState.pendingAttachments = nil
		m.composerState.nextAttachmentID = 0
		m.syncComposerPrompt()
		return
	}
	m.composerState.pendingAttachments = filtered
	m.syncComposerPrompt()
}
