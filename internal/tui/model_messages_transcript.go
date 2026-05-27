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
	layout := m.buildVisibleTranscriptLayout(state, contentWidth)
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

func (m *Model) syncVisibleTranscriptChunksIfNeeded() {
	if m == nil || len(m.transcriptView.layout.chunks) == 0 {
		return
	}
	state := m.projector.CurrentState()
	contentWidth := max(m.messages.Width(), 1)
	layout := m.buildVisibleTranscriptLayout(state, contentWidth)
	if transcriptLayoutsEquivalentForVirtualContent(m.transcriptView.layout, layout) {
		return
	}
	offset := m.messages.YOffset()
	m.applyVirtualTranscriptLayout(layout, false)
	m.messages.GotoLine(offset)
	m.syncTranscriptVisualState()
}

func transcriptLayoutsEquivalentForVirtualContent(left, right transcriptLayout) bool {
	if left.width != right.width || left.wide != right.wide || len(left.chunks) != len(right.chunks) {
		return false
	}
	if strings.TrimRight(left.turnSeparator.content, "\n") != strings.TrimRight(right.turnSeparator.content, "\n") {
		return false
	}
	for i := range left.chunks {
		a := left.chunks[i]
		b := right.chunks[i]
		if a.kind != b.kind ||
			strings.TrimSpace(a.turnID) != strings.TrimSpace(b.turnID) ||
			strings.TrimSpace(a.cacheKey) != strings.TrimSpace(b.cacheKey) ||
			transcriptLayoutChunkLineCount(a) != transcriptLayoutChunkLineCount(b) ||
			strings.TrimRight(a.rendered.content, "\n") != strings.TrimRight(b.rendered.content, "\n") {
			return false
		}
	}
	return true
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
	lineStarts := layout.turnLineStarts()
	window := transcriptViewportRenderWindow(m.messages.YOffset(), m.messages.Height())
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
		chunk := layout.chunks[index]
		cacheKey := turnTranscriptChunkCacheKeyWithOptions(m, state, turnID, turn, contentWidth, options)
		start := lineStarts[turnID]
		if transcriptTurnRequiresRender(m, turnID) || transcriptLineRangeIntersects(start, transcriptLayoutChunkLineCount(chunk), window.start, window.end) {
			rendered, renderedKey := cachedTurnTranscriptRenderWithKey(m, state, turnID, turn, contentWidth, options)
			if strings.TrimSpace(rendered.content) == "" {
				return transcriptLayout{}, false
			}
			chunk.rendered = rendered
			chunk.cacheKey = renderedKey
			chunk.lineCount = transcriptRenderLineCount(rendered)
		} else {
			chunk.cacheKey = cacheKey
			if chunk.lineCount <= 0 {
				chunk.lineCount = transcriptRenderLineCount(chunk.rendered)
			}
			chunk.rendered = stripTranscriptRenderContent(chunk.rendered)
		}
		layout.chunks[index] = chunk
	}
	return layout, true
}

func (m Model) buildTranscriptLayoutForViewport(state events.SessionState, width int) transcriptLayout {
	return m.buildVisibleTranscriptLayout(state, width)
}

func (m Model) buildVisibleTranscriptLayout(state events.SessionState, width int) transcriptLayout {
	previous := m.transcriptView.layout
	width = max(width, 1)
	wide := isWideShell(m)
	turnIDs := visibleTranscriptTurnIDs(m, state)
	previousCompatible := previous.width == width && previous.wide == wide && len(previous.chunks) > 0
	if !previousCompatible {
		return buildTranscriptLayout(m, state, width)
	}

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

	window := transcriptViewportRenderWindow(m.messages.YOffset(), m.messages.Height())
	line := 0
	index := 0
	visibleTurnSeen := false
	appendRenderedLineCount := func(rendered transcriptRender) {
		content := strings.TrimRight(rendered.content, "\n")
		if strings.TrimSpace(content) == "" {
			return
		}
		if index > 0 {
			line += 2
		}
		line += transcriptRenderedLineCount(content)
		index++
	}
	appendChunkLineCount := func(lineCount int) {
		if lineCount <= 0 {
			return
		}
		if index > 0 {
			line += 2
		}
		line += lineCount
		index++
	}

	for i, turnID := range turnIDs {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		if visibleTurnSeen && !layout.wide && strings.TrimSpace(layout.turnSeparator.content) != "" {
			appendRenderedLineCount(layout.turnSeparator)
		}
		visibleTurnSeen = true
		start := line
		if index > 0 {
			start += 2
		}
		options := transcriptTurnRenderOptions{}
		if i+1 < len(turnIDs) && shouldSuppressHistoryCompactionBeforeContinuation(state, turnID, turnIDs[i+1]) {
			options.suppressHistoryCompaction = true
		}
		cacheKey := turnTranscriptChunkCacheKeyWithOptions(m, state, turnID, turn, width, options)
		chunk := transcriptLayoutChunk{
			kind:     transcriptLayoutChunkTurn,
			turnID:   turnID,
			cacheKey: cacheKey,
		}
		if previousIndex, ok := previous.turnIndices[turnID]; ok && previousIndex >= 0 && previousIndex < len(previous.chunks) {
			previousChunk := previous.chunks[previousIndex]
			if previousChunk.kind == transcriptLayoutChunkTurn {
				chunk.rendered = previousChunk.rendered
				chunk.lineCount = transcriptLayoutChunkLineCount(previousChunk)
			}
		}
		if chunk.lineCount <= 0 || transcriptTurnRequiresRender(m, turnID) || transcriptLineRangeIntersects(start, chunk.lineCount, window.start, window.end) {
			rendered, renderedKey := cachedTurnTranscriptRenderWithKey(m, state, turnID, turn, width, options)
			chunk.rendered = rendered
			chunk.cacheKey = renderedKey
			chunk.lineCount = transcriptRenderLineCount(rendered)
		} else {
			chunk.rendered = stripTranscriptRenderContent(chunk.rendered)
		}
		if chunk.lineCount <= 0 {
			continue
		}
		layout.turnIndices[turnID] = len(layout.chunks)
		layout.chunks = append(layout.chunks, chunk)
		appendChunkLineCount(chunk.lineCount)
	}

	if draftSections := renderDraftTurnSections(m, state, width); len(draftSections) > 0 {
		rendered := buildTranscriptChunk(draftSections)
		layout.chunks = append(layout.chunks, transcriptLayoutChunk{
			kind:      transcriptLayoutChunkDraft,
			rendered:  rendered,
			lineCount: transcriptRenderLineCount(rendered),
		})
	}

	if handoff := m.pendingDelegatedPermission(); handoff != nil {
		row := newDelegatedPermissionSystemRow(handoff, width)
		rendered := row.render(m)
		layout.chunks = append(layout.chunks, transcriptLayoutChunk{
			kind:      transcriptLayoutChunkDelegatedPermission,
			rendered:  rendered,
			lineCount: transcriptRenderLineCount(rendered),
		})
	}
	return layout
}

func transcriptTurnRequiresRender(m Model, turnID string) bool {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return false
	}
	if strings.TrimSpace(m.selection.callTurnID) == turnID && strings.TrimSpace(m.selection.callID) != "" {
		return true
	}
	if strings.TrimSpace(m.selection.handoffID) != "" && strings.TrimSpace(m.turnID) == turnID {
		return true
	}
	return false
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
		lineCount := transcriptRenderedLineCount(content)
		if strings.TrimSpace(content) == "" {
			return false
		}
		if index > 0 {
			line += 2
		}
		line += lineCount
		index++
		return true
	}
	appendChunk := func(chunk transcriptLayoutChunk) bool {
		lineCount := transcriptLayoutChunkLineCount(chunk)
		if lineCount <= 0 {
			return false
		}
		if index > 0 {
			line += 2
		}
		line += lineCount
		index++
		return true
	}
	for _, chunk := range layout.chunks {
		rendered := chunk.rendered
		content := strings.TrimRight(rendered.content, "\n")
		if strings.TrimSpace(content) == "" && transcriptLayoutChunkLineCount(chunk) <= 0 {
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
		appendChunk(chunk)
	}
	return starts
}

func transcriptViewportRenderWindow(offset, height int) transcriptLineWindow {
	visibleHeight := max(height, 1)
	overscan := visibleHeight
	start := max(offset-overscan, 0)
	end := max(offset, 0) + visibleHeight + overscan
	return transcriptLineWindow{start: start, end: end}
}

type transcriptLineWindow struct {
	start int
	end   int
}

func transcriptLineRangeIntersects(start, lineCount, windowStart, windowEnd int) bool {
	if lineCount <= 0 || windowEnd <= windowStart {
		return false
	}
	end := start + lineCount
	return start < windowEnd && end > windowStart
}

func transcriptTurnIntersectsViewport(chunk transcriptLayoutChunk, start, offset, height int) bool {
	if height <= 0 {
		return false
	}
	lineCount := transcriptLayoutChunkLineCount(chunk)
	viewportStart := max(offset, 0)
	viewportEnd := viewportStart + max(height, 1)
	return transcriptLineRangeIntersects(start, lineCount, viewportStart, viewportEnd)
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
