package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

type animTickMsg struct{}
type transcriptRefreshTickMsg struct{}
type dialogRefreshTickMsg struct{}

func animTickCmd() tea.Cmd {
	return tea.Tick(60*time.Millisecond, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
}

func (m *Model) ensureAnimTicking() tea.Cmd {
	if m.animation.ticking || !m.shouldAnimateTranscriptActivity() {
		return nil
	}
	m.animation.ticking = true
	return animTickCmd()
}
