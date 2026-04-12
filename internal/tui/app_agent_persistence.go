package tui

import (
	"context"
	"log"
	"time"

	tea "charm.land/bubbletea/v2"
)

var agentPersistDebounce = 500 * time.Millisecond

type agentPersistTickMsg struct {
	agentID string
	seq     int
}

type agentPersistResultMsg struct {
	agentID string
	seq     int
	err     error
}

func (a *App) scheduleAgentPersistence(agentID string) tea.Cmd {
	if agentID == "" {
		return nil
	}
	a.agentPersistDirty = true
	a.agentPersistSeq++
	seq := a.agentPersistSeq
	return tea.Tick(agentPersistDebounce, func(time.Time) tea.Msg {
		return agentPersistTickMsg{agentID: agentID, seq: seq}
	})
}

func (a App) persistLastAgentCmd(agentID string, seq int) tea.Cmd {
	if agentID == "" {
		return nil
	}
	api := a.api
	ctx := a.ctx
	return func() tea.Msg {
		err := api.SetSetting(ctx, "last_agent", agentID)
		return agentPersistResultMsg{
			agentID: agentID,
			seq:     seq,
			err:     err,
		}
	}
}

func (a App) handleAgentPersistTick(msg agentPersistTickMsg) (tea.Model, tea.Cmd) {
	if !a.agentPersistDirty || msg.seq != a.agentPersistSeq || msg.agentID == "" || msg.agentID != a.cfg.Agent {
		return a, nil
	}
	return a, a.persistLastAgentCmd(msg.agentID, msg.seq)
}

func (a App) handleAgentPersistResult(msg agentPersistResultMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		log.Printf("tui: persist last_agent failed: %v", msg.err)
		return a, nil
	}
	if msg.seq == a.agentPersistSeq && msg.agentID == a.cfg.Agent {
		a.agentPersistDirty = false
	}
	return a, nil
}

func (a *App) flushPendingAgentSelection() {
	if !a.agentPersistDirty || a.cfg.Agent == "" {
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, time.Second)
	defer cancel()
	if err := a.api.SetSetting(ctx, "last_agent", a.cfg.Agent); err != nil {
		log.Printf("tui: flush last_agent failed: %v", err)
		return
	}
	a.agentPersistDirty = false
}
