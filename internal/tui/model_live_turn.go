package tui

import (
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func (m *Model) armLiveTurn() {
	m.liveTurn.spinnerArmed = true
	if m.liveTurn.startedAt.IsZero() {
		m.liveTurn.startedAt = time.Now()
	}
}

func (m *Model) disarmLiveTurn() {
	m.liveTurn.spinnerArmed = false
	m.liveTurn.startedAt = time.Time{}
	m.transcriptRefresh.pending = false
	m.transcriptRefresh.ticking = false
}

func (m *Model) syncLiveTurnWithState(state events.SessionState) {
	m.disarmLiveTurn()
	m.liveTurn.cancelRequested = false
	if turnID := effectiveLiveTurnID(*m, state); turnID != "" {
		if turn := currentTurn(state, turnID); turn != nil && turn.Status == events.TurnStatusRunning {
			m.armLiveTurn()
		}
	}
}
