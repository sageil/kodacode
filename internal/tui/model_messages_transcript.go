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
	followBottom := m.messages.AtBottom()
	if isWideShell(*m) {
		rendered := renderTranscriptLayout(*m, state, contentWidth)
		m.applyTranscriptLayout(rendered.layout, rendered.rendered, followBottom)
		return
	}
	layout := m.buildVisibleTranscriptLayout(state, contentWidth)
	m.applyVirtualTranscriptLayout(layout, followBottom)
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
	if isWideShell(*m) {
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
		if isWideShell(*m) {
			m.syncTranscriptStructureWithState(state)
			return true
		}
		layout, ok := m.transcriptLayoutForTurnRefresh(state, plan.turnIDs...)
		if !ok {
			m.err = ErrTranscriptIncrementalRefreshInvariant
			return false
		}
		contentWidth := max(m.messages.Width(), 1)
		followBottom := m.messages.AtBottom()
		layout = m.renderLayoutVisibleTurnPlaceholders(state, contentWidth, layout, followBottom)
		m.applyVirtualTranscriptLayout(layout, followBottom)
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
		options.suppressCompletedWorkflowReviewEntries = shouldRenderCompletedWorkflowReport(state)
		ctx := transcriptTurnChunkLifecycle{
			state:   state,
			turnID:  turnID,
			turn:    turn,
			width:   contentWidth,
			options: options,
			start:   lineStarts[turnID],
			window:  window,
		}
		chunk, ok := layout.chunks[index].syncTurnLifecycle(m, ctx)
		if !ok {
			return transcriptLayout{}, false
		}
		layout.chunks[index] = chunk
	}
	return layout, true
}

func (m Model) buildVisibleTranscriptLayout(state events.SessionState, width int) transcriptLayout {
	previous := m.transcriptView.layout
	width = max(width, 1)
	wide := isWideShell(m)
	turnIDs := visibleTranscriptTurnIDs(m, state)
	previousCompatible := previous.width == width && previous.wide == wide && len(previous.chunks) > 0
	if !previousCompatible {
		return m.buildInitialVisibleTranscriptLayout(state, width, turnIDs, wide)
	}

	layout := newTranscriptLayoutShell(m, width, wide)

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
		options.suppressCompletedWorkflowReviewEntries = shouldRenderCompletedWorkflowReport(state)
		ctx := transcriptTurnChunkLifecycle{
			state:   state,
			turnID:  turnID,
			turn:    turn,
			width:   width,
			options: options,
			start:   start,
			window:  window,
		}
		chunk := newTranscriptTurnLayoutChunk(m, ctx)
		if previousIndex, ok := previous.turnIndices[turnID]; ok && previousIndex >= 0 && previousIndex < len(previous.chunks) {
			chunk = chunk.withPreviousLifecycle(previous.chunks[previousIndex])
		}
		var ok bool
		chunk, ok = chunk.syncTurnLifecycle(m, ctx)
		if !ok {
			continue
		}
		layout.turnIndices[turnID] = len(layout.chunks)
		layout.chunks = append(layout.chunks, chunk)
		appendChunkLineCount(chunk.lineCount)
	}

	layout = appendTranscriptTrailingChunks(m, state, width, layout)
	return m.renderLayoutVisibleTurnPlaceholders(state, width, layout, m.messages.AtBottom())
}

func (m Model) buildInitialVisibleTranscriptLayout(state events.SessionState, width int, turnIDs []string, wide bool) transcriptLayout {
	layout := newTranscriptLayoutShell(m, width, wide)
	line := 0
	index := 0
	visibleTurnSeen := false
	appendRenderedLineCount := func(rendered transcriptRender) {
		if strings.TrimSpace(rendered.content) == "" {
			return
		}
		if index > 0 {
			line += 2
		}
		line += transcriptRenderLineCount(rendered)
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

	type pendingTurnChunk struct {
		ctx   transcriptTurnChunkLifecycle
		chunk transcriptLayoutChunk
	}
	pending := make([]pendingTurnChunk, 0, len(turnIDs))
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
		options.suppressCompletedWorkflowReviewEntries = shouldRenderCompletedWorkflowReport(state)
		ctx := transcriptTurnChunkLifecycle{
			state:   state,
			turnID:  turnID,
			turn:    turn,
			width:   width,
			options: options,
			start:   start,
		}
		chunk := newTranscriptTurnLayoutChunk(m, ctx)
		chunk.lineCount = estimatedTurnTranscriptLineCount(m, ctx)
		if chunk.lineCount <= 0 && !transcriptTurnRetainsLifecycle(turn) {
			continue
		}
		layout.turnIndices[turnID] = len(pending)
		pending = append(pending, pendingTurnChunk{ctx: ctx, chunk: chunk})
		appendChunkLineCount(chunk.lineCount)
	}

	window := initialTranscriptViewportRenderWindow(m, line)
	layout.turnIndices = make(map[string]int, len(pending))
	layout.chunks = make([]transcriptLayoutChunk, 0, len(pending))
	for _, item := range pending {
		item.ctx.window = window
		chunk, ok := item.chunk.syncTurnLifecycle(m, item.ctx)
		if !ok {
			continue
		}
		layout.turnIndices[item.ctx.turnID] = len(layout.chunks)
		layout.chunks = append(layout.chunks, chunk)
	}

	layout = appendTranscriptTrailingChunks(m, state, width, layout)
	return m.renderLayoutVisibleTurnPlaceholders(state, width, layout, m.messages.AtBottom())
}

func (m Model) renderLayoutVisibleTurnPlaceholders(state events.SessionState, width int, layout transcriptLayout, followBottom bool) transcriptLayout {
	width = max(width, 1)
	for i := 0; i < 4; i++ {
		offset := max(m.messages.YOffset(), 0)
		if followBottom {
			offset = max(transcriptLayoutVirtualLineCount(layout)-max(m.messages.Height(), 1), 0)
		}
		window := transcriptViewportRenderWindow(offset, m.messages.Height())
		lineStarts := layout.turnLineStarts()
		changed := false
		for index, chunk := range layout.chunks {
			if chunk.kind != transcriptLayoutChunkTurn || strings.TrimSpace(chunk.rendered.content) != "" {
				continue
			}
			turnID := strings.TrimSpace(chunk.turnID)
			start, ok := lineStarts[turnID]
			if !ok || !transcriptLineRangeIntersects(start, transcriptLayoutChunkLineCount(chunk), window.start, window.end) {
				continue
			}
			turn := state.Turns[turnID]
			if turn == nil {
				continue
			}
			options := transcriptTurnRenderOptions{}
			if shouldSuppressHistoryCompactionBeforeContinuation(state, turnID, nextTranscriptLayoutTurnID(layout, turnID)) {
				options.suppressHistoryCompaction = true
			}
			options.suppressCompletedWorkflowReviewEntries = shouldRenderCompletedWorkflowReport(state)
			ctx := transcriptTurnChunkLifecycle{
				state:   state,
				turnID:  turnID,
				turn:    turn,
				width:   width,
				options: options,
				start:   start,
				window:  window,
			}
			rendered, ok := chunk.renderTurn(m, ctx)
			if !ok {
				continue
			}
			layout.chunks[index] = rendered
			changed = true
		}
		if !changed {
			return layout
		}
	}
	return layout
}

func transcriptLayoutVirtualLineCount(layout transcriptLayout) int {
	total := 0
	for _, chunk := range virtualTranscriptRender(layout).chunks {
		if chunk.blankLines > 0 {
			total += chunk.blankLines
			continue
		}
		total += virtualContentLineCount(chunk.content)
	}
	return total
}

func newTranscriptLayoutShell(m Model, width int, wide bool) transcriptLayout {
	layout := transcriptLayout{
		width:       max(width, 1),
		wide:        wide,
		turnIndices: make(map[string]int),
	}
	if !layout.wide {
		layout.turnSeparator = transcriptRender{
			content:        renderTurnSeparator(m, width),
			selectionLines: []transcriptSelectionLine{{}},
		}
	}
	return layout
}

func initialTranscriptViewportRenderWindow(m Model, contentLineCount int) transcriptLineWindow {
	height := max(m.messages.Height(), 1)
	offset := max(m.messages.YOffset(), 0)
	if m.messages.AtBottom() {
		offset = max(contentLineCount-height, 0)
	}
	return transcriptViewportRenderWindow(offset, height)
}

func estimatedTurnTranscriptLineCount(m Model, ctx transcriptTurnChunkLifecycle) int {
	if cached, ok := cachedTurnTranscriptRenderForKey(ctx.cacheKey(m)); ok {
		return transcriptRenderLineCount(cached)
	}
	turn := ctx.turn
	if turn == nil {
		return 0
	}
	lines := 0
	sections := 0
	addSection := func(lineCount int) {
		if lineCount <= 0 {
			return
		}
		if sections > 0 {
			lines += 2
		}
		lines += lineCount
		sections++
	}

	toolRenderer := transcriptToolEntryRendererForModel(m)
	for i := 0; i < len(turn.Transcript); i++ {
		entry := turn.Transcript[i]
		if suppressContextLimitContinuationTranscriptEntry(turn, entry) ||
			suppressQuestionAnswerContinuationTranscriptEntry(ctx.state, turn, entry) {
			continue
		}
		switch entry.Kind {
		case events.TranscriptEntryUser:
			addSection(estimatedUserTranscriptLineCount(m, entry.Text, ctx.width))
		case events.TranscriptEntryAssistant, events.TranscriptEntryWorklog:
			if entry.Kind == events.TranscriptEntryAssistant && suppressAssistantEntryForStructuredReview(turn, i) {
				continue
			}
			if !isTurnContinuationTranscriptEntry(turn, entry) {
				addSection(estimatedFallbackCompactionLineCount(m, ctx))
			}
			addSection(estimatedAssistantTranscriptLineCount(turn, entry.Text, ctx.width))
			if isTurnContinuationTranscriptEntry(turn, entry) {
				addSection(estimatedFallbackCompactionLineCount(m, ctx))
			}
		case events.TranscriptEntryCompaction:
			if !ctx.options.suppressHistoryCompaction {
				addSection(estimatedTranscriptBlockLineCount(entry.Text, ctx.width))
			}
		case events.TranscriptEntryReview:
			if ctx.options.suppressCompletedWorkflowReviewEntries && isWorkflowReviewTranscriptEntry(turn) {
				continue
			}
			addSection(estimatedFallbackCompactionLineCount(m, ctx))
			addSection(estimatedTranscriptBlockLineCount(entry.Text, ctx.width))
		case events.TranscriptEntryReasoning:
			addSection(estimatedFallbackCompactionLineCount(m, ctx))
			addSection(estimatedTranscriptBlockLineCount(entry.Text, ctx.width))
		case events.TranscriptEntryTool:
			addSection(estimatedFallbackCompactionLineCount(m, ctx))
			if toolRenderer.BatchConsecutive() {
				refs := make([]sessionToolCallRef, 0, 4)
				for i < len(turn.Transcript) && turn.Transcript[i].Kind == events.TranscriptEntryTool {
					callID := turn.Transcript[i].CallID
					call := turn.ToolCalls[callID]
					if toolRenderer.ShouldRenderCall(m, turn, callID, call) {
						refs = append(refs, sessionToolCallRef{TurnID: ctx.turnID, CallID: callID})
					}
					i++
				}
				i--
				addSection(estimatedToolRefsLineCount(m, ctx.state, refs))
				continue
			}
			call := turn.ToolCalls[entry.CallID]
			if toolRenderer.ShouldRenderCall(m, turn, entry.CallID, call) {
				addSection(estimatedToolRefsLineCount(m, ctx.state, []sessionToolCallRef{{TurnID: ctx.turnID, CallID: entry.CallID}}))
			}
		}
	}
	addSection(estimatedAssistantTranscriptLineCount(turn, turn.StreamingText, ctx.width))
	if turn.Status == events.TurnStatusRunning {
		refs := make([]sessionToolCallRef, 0, len(turn.ToolCallOrder))
		for _, callID := range orderedToolCallIDs(turn) {
			call := turn.ToolCalls[callID]
			if shouldRenderLiveToolCallPreview(turn, call) {
				refs = append(refs, sessionToolCallRef{TurnID: ctx.turnID, CallID: callID})
			}
		}
		addSection(estimatedToolRefsLineCount(m, ctx.state, filterPendingQuestionToolRefs(m, refs)))
	}
	if explicitSelectedHandoff(turn, strings.TrimSpace(m.selection.handoffID)) != nil {
		addSection(estimatedTranscriptBlockLineCount(strings.TrimSpace(m.selection.handoffID), ctx.width))
	}
	return lines
}

func estimatedUserTranscriptLineCount(m Model, text string, width int) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(transcriptRailSelectionLines(m, text, width))
}

func estimatedAssistantTranscriptLineCount(turn *events.TurnState, text string, width int) int {
	if isLocalShellTurn(turn) || strings.TrimSpace(text) == "" {
		return 0
	}
	contentWidth := max(max(width, 1)-4, 1)
	return len(wrapTranscriptText(strings.TrimRight(strings.TrimSpace(text), "\n"), contentWidth)) + 2
}

func estimatedTranscriptBlockLineCount(text string, width int) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(wrapTranscriptText(text, max(width, 1))) + 2
}

func estimatedFallbackCompactionLineCount(m Model, ctx transcriptTurnChunkLifecycle) int {
	if ctx.options.suppressHistoryCompaction || ctx.turn == nil || turnHasHistoryCompactionTranscriptEntry(ctx.turn) {
		return 0
	}
	if effectiveTurnHistoryContinuation(ctx.state, ctx.turnID, ctx.turn) == nil {
		return 0
	}
	return estimatedTranscriptBlockLineCount(historyCompactionSummaryText(effectiveTurnHistoryContinuation(ctx.state, ctx.turnID, ctx.turn)), ctx.width)
}

func estimatedToolRefsLineCount(m Model, state events.SessionState, refs []sessionToolCallRef) int {
	if len(refs) == 0 {
		return 0
	}
	if shellLayoutEnabled(m) || isWideShell(m) {
		return max(len(deriveTurnToolOutcomeRows(state, refs)), 0)
	}
	return max(len(refs)*3, 1)
}

type transcriptTurnChunkLifecycle struct {
	state   events.SessionState
	turnID  string
	turn    *events.TurnState
	width   int
	options transcriptTurnRenderOptions
	start   int
	window  transcriptLineWindow
}

func newTranscriptTurnLayoutChunk(m Model, ctx transcriptTurnChunkLifecycle) transcriptLayoutChunk {
	return transcriptLayoutChunk{
		kind:     transcriptLayoutChunkTurn,
		turnID:   strings.TrimSpace(ctx.turnID),
		cacheKey: ctx.cacheKey(m),
	}
}

func (ctx transcriptTurnChunkLifecycle) cacheKey(m Model) string {
	return turnTranscriptChunkCacheKeyWithOptions(m, ctx.state, ctx.turnID, ctx.turn, ctx.width, ctx.options)
}

func (chunk transcriptLayoutChunk) withPreviousLifecycle(previous transcriptLayoutChunk) transcriptLayoutChunk {
	if previous.kind != transcriptLayoutChunkTurn {
		return chunk
	}
	chunk.rendered = previous.rendered
	chunk.lineCount = transcriptLayoutChunkLineCount(previous)
	return chunk
}

func (chunk transcriptLayoutChunk) syncTurnLifecycle(m Model, ctx transcriptTurnChunkLifecycle) (transcriptLayoutChunk, bool) {
	chunk.kind = transcriptLayoutChunkTurn
	chunk.turnID = strings.TrimSpace(ctx.turnID)
	chunk.cacheKey = ctx.cacheKey(m)
	if chunk.shouldRenderTurn(m, ctx) {
		return chunk.renderTurn(m, ctx)
	}
	chunk.ensurePlaceholderLineCount()
	chunk.rendered = stripTranscriptRenderContent(chunk.rendered)
	return chunk, chunk.lineCount > 0 || transcriptTurnRetainsLifecycle(ctx.turn)
}

func (chunk transcriptLayoutChunk) shouldRenderTurn(m Model, ctx transcriptTurnChunkLifecycle) bool {
	return chunk.lineCount <= 0 ||
		transcriptTurnRequiresRender(m, ctx.turnID) ||
		transcriptLineRangeIntersects(ctx.start, chunk.lineCount, ctx.window.start, ctx.window.end)
}

func (chunk transcriptLayoutChunk) renderTurn(m Model, ctx transcriptTurnChunkLifecycle) (transcriptLayoutChunk, bool) {
	rendered, cacheKey := cachedTurnTranscriptRenderWithKey(m, ctx.state, ctx.turnID, ctx.turn, ctx.width, ctx.options)
	if strings.TrimSpace(rendered.content) == "" {
		chunk.rendered = transcriptRender{}
		chunk.lineCount = 0
		return chunk, transcriptTurnRetainsLifecycle(ctx.turn)
	}
	chunk.rendered = rendered
	chunk.cacheKey = cacheKey
	chunk.lineCount = transcriptRenderLineCount(rendered)
	return chunk, true
}

func transcriptTurnRetainsLifecycle(turn *events.TurnState) bool {
	if turn == nil {
		return false
	}
	return len(turn.Transcript) > 0 ||
		len(turn.ToolCallOrder) > 0 ||
		strings.TrimSpace(turn.StreamingText) != "" ||
		strings.TrimSpace(turn.UserText) != "" ||
		turn.Continuation != nil ||
		turn.ContinuationStart != nil ||
		turn.Review != nil
}

func (chunk *transcriptLayoutChunk) ensurePlaceholderLineCount() {
	if chunk == nil || chunk.lineCount > 0 {
		return
	}
	chunk.lineCount = transcriptRenderLineCount(chunk.rendered)
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
