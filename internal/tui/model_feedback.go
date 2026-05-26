package tui

import "strings"

func (m *Model) setComposerError(message string) {
	if m == nil {
		return
	}
	next := strings.TrimSpace(message)
	if m.composerState.err == next {
		return
	}
	m.composerState.err = next
	m.syncViewportLayout()
}

func (m *Model) clearComposerError() {
	if m == nil {
		return
	}
	if m.composerState.err == "" {
		return
	}
	m.composerState.err = ""
	m.syncViewportLayout()
}

func (m *Model) setFooterError(message string) {
	if m == nil {
		return
	}
	m.footerNotice.err = strings.TrimSpace(message)
	m.syncViewportLayout()
}

func (m *Model) clearFooterError() {
	if m == nil {
		return
	}
	m.footerNotice.err = ""
	m.syncViewportLayout()
}
