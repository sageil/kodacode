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
	layout := m.buildTranscriptLayoutForViewport(state, contentWidth)
	m.applyVirtualTranscriptLayout(layout, m.messages.AtBottom())
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

func (m *Model) applyVirtualTranscriptLayout(layout transcriptLayout, followBottom bool) {
	if m == nil {
		return
	}
	rendered := virtualTranscriptRender(layout)
	m.messages.SyncVirtualChunks(rendered.chunks, followBottom)
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
		m.applyVirtualTranscriptLayout(layout, m.messages.AtBottom())
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
		rendered, cacheKey := cachedTurnTranscriptRenderWithKey(m, state, turnID, turn, contentWidth, options)
		if strings.TrimSpace(rendered.content) == "" {
			return transcriptLayout{}, false
		}
		layout.chunks[index].rendered = rendered
		layout.chunks[index].cacheKey = cacheKey
	}
	return layout, true
}

func (m Model) buildTranscriptLayoutForViewport(state events.SessionState, width int) transcriptLayout {
	previous := m.transcriptView.layout
	width = max(width, 1)
	wide := isWideShell(m)
	turnIDs := visibleTranscriptTurnIDs(m, state)
	if !previous.canReuseForViewportRender(width, wide, turnIDs) {
		return buildTranscriptLayout(m, state, width)
	}

	lineStarts := previous.turnLineStarts()
	layout := transcriptLayout{
		width:       width,
		wide:        wide,
		turnIndices: make(map[string]int),
	}
	if !layout.wide {
		layout.turnSeparator = transcriptRender{
			content:        renderTurnSeparator(m, width),
			selectionLines: []transcriptSelectionLine{{}},
		}
	}

	for i, turnID := range turnIDs {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		options := transcriptTurnRenderOptions{}
		if i+1 < len(turnIDs) && shouldSuppressHistoryCompactionBeforeContinuation(state, turnID, turnIDs[i+1]) {
			options.suppressHistoryCompaction = true
		}
		previousIndex := previous.turnIndices[turnID]
		previousChunk := previous.chunks[previousIndex]
		cacheKey := turnTranscriptChunkCacheKeyWithOptions(m, state, turnID, turn, width, options)
		rendered := previousChunk.rendered
		if previousChunk.cacheKey != cacheKey || transcriptTurnIntersectsViewport(previousChunk, lineStarts[turnID], m.messages.YOffset(), m.messages.Height()) {
			rendered, cacheKey = cachedTurnTranscriptRenderWithKey(m, state, turnID, turn, width, options)
		}
		layout.turnIndices[turnID] = len(layout.chunks)
		layout.chunks = append(layout.chunks, transcriptLayoutChunk{
			kind:     transcriptLayoutChunkTurn,
			turnID:   turnID,
			cacheKey: cacheKey,
			rendered: rendered,
		})
	}

	if draftSections := renderDraftTurnSections(m, state, width); len(draftSections) > 0 {
		layout.chunks = append(layout.chunks, transcriptLayoutChunk{
			kind:     transcriptLayoutChunkDraft,
			rendered: buildTranscriptChunk(draftSections),
		})
	}

	if handoff := m.pendingDelegatedPermission(); handoff != nil {
		row := newDelegatedPermissionSystemRow(handoff, width)
		layout.chunks = append(layout.chunks, transcriptLayoutChunk{
			kind:     transcriptLayoutChunkDelegatedPermission,
			rendered: row.render(m),
		})
	}
	return layout
}

func (layout transcriptLayout) canReuseForViewportRender(width int, wide bool, turnIDs []string) bool {
	if layout.width != max(width, 1) || layout.wide != wide || len(layout.chunks) == 0 {
		return false
	}
	if len(layout.turnIndices) != len(turnIDs) {
		return false
	}
	for _, turnID := range turnIDs {
		index, ok := layout.turnIndices[strings.TrimSpace(turnID)]
		if !ok || index < 0 || index >= len(layout.chunks) {
			return false
		}
		chunk := layout.chunks[index]
		if chunk.kind != transcriptLayoutChunkTurn || strings.TrimSpace(chunk.turnID) != strings.TrimSpace(turnID) {
			return false
		}
	}
	return true
}

func (layout transcriptLayout) turnLineStarts() map[string]int {
	starts := make(map[string]int, len(layout.turnIndices))
	line := 0
	index := 0
	visibleTurnSeen := false
	appendRendered := func(rendered transcriptRender) bool {
		content := strings.TrimRight(rendered.content, "\n")
		if strings.TrimSpace(content) == "" {
			return false
		}
		if index > 0 {
			line += 2
		}
		line += transcriptRenderedLineCount(content)
		index++
		return true
	}
	for _, chunk := range layout.chunks {
		rendered := chunk.rendered
		content := strings.TrimRight(rendered.content, "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}
		if chunk.kind == transcriptLayoutChunkTurn {
			if visibleTurnSeen && !layout.wide && strings.TrimSpace(layout.turnSeparator.content) != "" {
				appendRendered(layout.turnSeparator)
			}
			visibleTurnSeen = true
			if index > 0 {
				starts[strings.TrimSpace(chunk.turnID)] = line + 2
			} else {
				starts[strings.TrimSpace(chunk.turnID)] = line
			}
		}
		appendRendered(rendered)
	}
	return starts
}

func transcriptTurnIntersectsViewport(chunk transcriptLayoutChunk, start, offset, height int) bool {
	if height <= 0 {
		return false
	}
	content := strings.TrimRight(chunk.rendered.content, "\n")
	if strings.TrimSpace(content) == "" {
		return false
	}
	end := start + transcriptRenderedLineCount(content)
	viewportStart := max(offset, 0)
	viewportEnd := viewportStart + max(height, 1)
	return start < viewportEnd && end > viewportStart
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
