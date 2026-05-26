package tui

import (
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

func (m *Model) syncTranscriptStructureWithState(state events.SessionState) {
	if m == nil {
		return
	}
	contentWidth := max(m.messages.Width(), 1)
	layout := renderTranscriptLayout(*m, state, contentWidth)
	m.applyTranscriptLayout(layout.layout, layout.rendered, m.messages.AtBottom())
}

func (m *Model) applyTranscriptLayout(layout transcriptLayout, rendered transcriptRender, followBottom bool) {
	if m == nil {
		return
	}
	m.messages.Sync(rendered.content, followBottom)
	m.syncTranscriptVisualState()
	m.transcriptRefresh.deferred = false
	m.transcriptRefresh.pending = false
	m.transcriptRefresh.plan = transcriptRefreshPlan{}
	m.transcriptRefresh.lastAt = time.Now()
	m.transcriptView.toolLines = rendered.toolLines
	m.transcriptView.selectionLines = rendered.selectionLines
	m.transcriptView.layout = layout
}

func (m *Model) applyTranscriptRefreshPlan(plan transcriptRefreshPlan) bool {
	if m == nil {
		return false
	}
	return m.applyTranscriptRefreshPlanWithState(m.projector.CurrentState(), plan)
}

func transcriptTurnRefreshPlan(turnIDs ...string) transcriptRefreshPlan {
	turnIDs = dedupeTranscriptTurnIDs(turnIDs)
	if len(turnIDs) == 0 {
		return transcriptRefreshPlan{}
	}
	return transcriptRefreshPlan{
		kind:    transcriptRefreshTurns,
		turnIDs: turnIDs,
	}
}

func (m Model) canSyncTranscriptTurns(state events.SessionState, turnIDs ...string) bool {
	_, ok := m.transcriptLayoutForTurnRefresh(state, turnIDs...)
	return ok
}

func (m *Model) applyTranscriptRefreshPlanWithState(state events.SessionState, plan transcriptRefreshPlan) bool {
	if m == nil {
		return false
	}
	switch plan.kind {
	case transcriptRefreshNone:
		return true
	case transcriptRefreshStructure:
		m.syncTranscriptStructureWithState(state)
		return true
	case transcriptRefreshTurns:
		layout, ok := m.transcriptLayoutForTurnRefresh(state, plan.turnIDs...)
		if !ok {
			m.err = ErrTranscriptIncrementalRefreshInvariant
			return false
		}
		m.applyTranscriptLayout(layout, layout.rendered(), m.messages.AtBottom())
		return true
	default:
		m.err = ErrTranscriptIncrementalRefreshInvariant
		return false
	}
}

func (m Model) transcriptLayoutForTurnRefresh(state events.SessionState, turnIDs ...string) (transcriptLayout, bool) {
	contentWidth := max(m.messages.Width(), 1)
	wide := isWideShell(m)
	if m.transcriptView.layout.width != contentWidth || m.transcriptView.layout.wide != wide || len(m.transcriptView.layout.chunks) == 0 {
		return transcriptLayout{}, false
	}
	uniqueTurnIDs := dedupeTranscriptTurnIDs(turnIDs)
	visibleTurnIDs := make([]string, 0, len(uniqueTurnIDs))
	for _, turnID := range uniqueTurnIDs {
		if _, ok := m.transcriptView.layout.turnIndices[turnID]; ok {
			visibleTurnIDs = append(visibleTurnIDs, turnID)
		}
	}
	if len(visibleTurnIDs) == 0 {
		return m.transcriptView.layout, true
	}
	if !m.transcriptView.layout.canRefreshTurns(contentWidth, wide, visibleTurnIDs) {
		return transcriptLayout{}, false
	}
	layout := m.transcriptView.layout
	layout.chunks = append([]transcriptLayoutChunk(nil), layout.chunks...)
	for _, turnID := range visibleTurnIDs {
		index := layout.turnIndices[turnID]
		turn := state.Turns[turnID]
		if turn == nil {
			return transcriptLayout{}, false
		}
		options := transcriptTurnRenderOptions{}
		if shouldSuppressHistoryCompactionBeforeContinuation(state, turnID, nextTranscriptLayoutTurnID(layout, turnID)) {
			options.suppressHistoryCompaction = true
		}
		rendered := cachedTurnTranscriptRender(m, state, turnID, turn, contentWidth, options)
		if strings.TrimSpace(rendered.content) == "" {
			return transcriptLayout{}, false
		}
		layout.chunks[index].rendered = rendered
	}
	return layout, true
}

func nextTranscriptLayoutTurnID(layout transcriptLayout, turnID string) string {
	index, ok := layout.turnIndices[strings.TrimSpace(turnID)]
	if !ok {
		return ""
	}
	for i := index + 1; i < len(layout.chunks); i++ {
		if layout.chunks[i].kind == transcriptLayoutChunkTurn {
			return strings.TrimSpace(layout.chunks[i].turnID)
		}
	}
	return ""
}
