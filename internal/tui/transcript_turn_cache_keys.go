package tui

import (
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func cachedTurnTranscriptRenderWithKey(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int, options transcriptTurnRenderOptions) (transcriptRender, string) {
	if turn == nil {
		return transcriptRender{}, ""
	}
	key := turnTranscriptChunkCacheKeyWithOptions(m, state, turnID, turn, width, options)

	if cached, ok := cachedTurnTranscriptRenderForKey(key); ok {
		return cached, key
	}

	rendered := buildTranscriptChunk(renderTurnTranscriptSectionsWithOptions(m, state, turnID, turn, width, options))

	turnTranscriptChunkCache.mu.Lock()
	turnTranscriptChunkCache.cache.put(key, rendered)
	turnTranscriptChunkCache.mu.Unlock()
	return rendered, key
}

func cachedTurnTranscriptRenderForKey(key string) (transcriptRender, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return transcriptRender{}, false
	}
	turnTranscriptChunkCache.mu.Lock()
	cached, ok := turnTranscriptChunkCache.cache.get(key)
	turnTranscriptChunkCache.mu.Unlock()
	return cached, ok
}

func turnTranscriptChunkCacheKey(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) string {
	return turnTranscriptChunkCacheKeyWithOptions(m, state, turnID, turn, width, transcriptTurnRenderOptions{})
}

func turnTranscriptChunkCacheKeyWithOptions(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int, options transcriptTurnRenderOptions) string {
	var b strings.Builder
	b.Grow(512)
	b.WriteString("turn_chunk\x00")
	b.WriteString(strconv.Itoa(max(width, 1)))
	b.WriteString("\x00")
	b.WriteString(modelRenderCacheKey(m))
	b.WriteString("\x00")
	if isWideShell(m) {
		b.WriteString("wide")
	} else {
		b.WriteString("stacked")
	}
	b.WriteString("\x00")
	b.WriteString(state.WorkspaceRoot)
	b.WriteString("\x00")
	b.WriteString(turnID)
	b.WriteString("\x00")
	b.WriteString(string(turn.Status))
	b.WriteString("\x00")
	b.WriteString(m.transcriptTurnSourceKey(turnID, turn))
	b.WriteString("\x00")
	if options.suppressHistoryCompaction {
		b.WriteString("suppress_history_compaction")
	}
	b.WriteString("\x00")
	b.WriteString(buildTurnCompactionSignature(effectiveTurnHistoryContinuation(state, turnID, turn)))
	b.WriteString("\x00")
	if strings.TrimSpace(selectedToolSessionID(m)) == strings.TrimSpace(state.SessionID) &&
		strings.TrimSpace(m.selection.callTurnID) == turnID {
		b.WriteString(strings.TrimSpace(m.selection.callID))
	}
	b.WriteString("\x00")
	if explicitSelectedHandoff(turn, strings.TrimSpace(m.selection.handoffID)) != nil {
		b.WriteString(strings.TrimSpace(m.selection.handoffID))
	}
	b.WriteString("\x00")
	if pending := pendingQuestionFromState(state); pending != nil && strings.TrimSpace(pending.TurnID) == turnID {
		b.WriteString(strings.TrimSpace(pending.QuestionID))
		b.WriteString("\x00")
		b.WriteString(strings.TrimSpace(pending.ToolCallID))
	}
	if ref, ok := m.selectedTranscriptLoadedToolResultRef(state, turnID); ok {
		loaded := m.toolHydration.loadedResults[scopedToolKey(m.sessionID, ref)]
		b.WriteString("\x00loaded:")
		b.WriteString(ref.CallID)
		b.WriteString("\x00")
		b.WriteString(toolResultDetailSignature(loaded))
	}
	return b.String()
}

func (m Model) transcriptTurnSourceKey(turnID string, turn *events.TurnState) string {
	turnID = strings.TrimSpace(turnID)
	if turnID != "" && m.transcriptView.turnSourceKeys != nil {
		if key, ok := m.transcriptView.turnSourceKeys[turnID]; ok {
			return key
		}
	}
	return buildTurnTranscriptSourceKey(turn)
}

func buildTurnTranscriptSourceKey(turn *events.TurnState) string {
	if turn == nil {
		return ""
	}
	sum := buildTurnTranscriptSourceSignature(turn)
	return strconv.FormatUint(sum, 16)
}

func (m *Model) primeTranscriptTurnSourceKeys(state events.SessionState) {
	if m == nil {
		return
	}
	keys := make(map[string]string, len(state.Turns))
	for _, turnID := range orderedSessionTurnIDs(state) {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		keys[turnID] = buildTurnTranscriptSourceKey(turn)
	}
	m.transcriptView.turnSourceKeys = keys
}

func (m *Model) refreshTranscriptTurnSourceKeysForBatch(state events.SessionState, batch []events.Event) {
	if m == nil {
		return
	}
	if m.transcriptView.turnSourceKeys == nil {
		m.transcriptView.turnSourceKeys = make(map[string]string)
	}
	refreshAll := false
	turnIDs := make(map[string]struct{}, len(batch))
	for _, event := range batch {
		if event.Type == events.TypeSessionStateSnapshot {
			refreshAll = true
			break
		}
		if !shouldSyncTranscriptForEvent(event) {
			continue
		}
		turnID := strings.TrimSpace(event.TurnID)
		if turnID == "" {
			refreshAll = true
			break
		}
		turnIDs[turnID] = struct{}{}
	}
	if refreshAll {
		m.primeTranscriptTurnSourceKeys(state)
		return
	}
	for turnID := range turnIDs {
		turn := state.Turns[turnID]
		if turn == nil {
			delete(m.transcriptView.turnSourceKeys, turnID)
			continue
		}
		m.transcriptView.turnSourceKeys[turnID] = buildTurnTranscriptSourceKey(turn)
	}
}

func buildTranscriptChunk(sections []transcriptSection) transcriptRender {
	return buildTranscriptSections(sections, false)
}
