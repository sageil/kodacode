package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type transcriptSection struct {
	content        string
	toolRefs       []sessionToolCallRef
	toolLineRefs   map[sessionToolCallRef]int
	selectionLines []transcriptSelectionLine
}

type transcriptRender struct {
	content        string
	toolLines      map[sessionToolCallRef]int
	selectionLines []transcriptSelectionLine
}

type transcriptSelectionLine struct {
	text            string
	prefixGraphemes int
	graphemeCount   int
}

type transcriptLayoutChunkKind string

const (
	transcriptLayoutChunkTurn                transcriptLayoutChunkKind = "turn"
	transcriptLayoutChunkDraft               transcriptLayoutChunkKind = "draft"
	transcriptLayoutChunkDelegatedPermission transcriptLayoutChunkKind = "delegated_permission"
	transcriptLayoutChunkWorkflowReport      transcriptLayoutChunkKind = "workflow_report"
)

type transcriptLayoutChunk struct {
	kind      transcriptLayoutChunkKind
	turnID    string
	cacheKey  string
	rendered  transcriptRender
	lineCount int
}

type transcriptLayout struct {
	width         int
	wide          bool
	turnSeparator transcriptRender
	chunks        []transcriptLayoutChunk
	turnIndices   map[string]int
}

type transcriptLayoutRender struct {
	layout   transcriptLayout
	rendered transcriptRender
}

const transcriptBottomPadding = "\n\n"
const userPromptInnerPadding = "  "

var userPromptRailGlyph = terminalIcon(terminalIconPromptRail)
var asciiUserPromptRailGlyph = asciiTerminalIconProfile.Icon(terminalIconPromptRail)

func renderTranscriptMessages(m Model, state events.SessionState, width int) transcriptRender {
	return renderTranscriptLayout(m, state, width).rendered
}

func renderTranscriptLayout(m Model, state events.SessionState, width int) transcriptLayoutRender {
	layout := buildTranscriptLayout(m, state, width)
	return transcriptLayoutRender{layout: layout, rendered: layout.rendered()}
}

func buildTranscriptLayout(m Model, state events.SessionState, width int) transcriptLayout {
	layout := transcriptLayout{
		width:       max(width, 1),
		wide:        isWideShell(m),
		turnIndices: make(map[string]int),
	}
	if !layout.wide {
		layout.turnSeparator = transcriptRender{
			content:        renderTurnSeparator(m, width),
			selectionLines: []transcriptSelectionLine{{}},
		}
	}

	turnIDs := visibleTranscriptTurnIDs(m, state)
	for i, turnID := range turnIDs {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		options := transcriptTurnRenderOptions{}
		if i+1 < len(turnIDs) && shouldSuppressHistoryCompactionBeforeContinuation(state, turnID, turnIDs[i+1]) {
			options.suppressHistoryCompaction = true
		}
		options.suppressCompletedWorkflowReviewEntries = shouldRenderCompletedWorkflowReport(state)
		rendered, cacheKey := cachedTurnTranscriptRenderWithKey(m, state, turnID, turn, width, options)
		layout.turnIndices[turnID] = len(layout.chunks)
		layout.chunks = append(layout.chunks, transcriptLayoutChunk{
			kind:      transcriptLayoutChunkTurn,
			turnID:    turnID,
			cacheKey:  cacheKey,
			rendered:  rendered,
			lineCount: transcriptRenderLineCount(rendered),
		})
	}

	return appendTranscriptTrailingChunks(m, state, width, layout)
}

func appendTranscriptTrailingChunks(m Model, state events.SessionState, width int, layout transcriptLayout) transcriptLayout {
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
	if reportSections := renderCompletedWorkflowReportSections(m, state, width); len(reportSections) > 0 {
		rendered := buildTranscriptChunk(reportSections)
		layout.chunks = append(layout.chunks, transcriptLayoutChunk{
			kind:      transcriptLayoutChunkWorkflowReport,
			rendered:  rendered,
			lineCount: transcriptRenderLineCount(rendered),
		})
	}
	return layout
}

func shouldSuppressHistoryCompactionBeforeContinuation(state events.SessionState, turnID, nextTurnID string) bool {
	turnID = strings.TrimSpace(turnID)
	nextTurnID = strings.TrimSpace(nextTurnID)
	if turnID == "" || nextTurnID == "" {
		return false
	}
	turn := state.Turns[turnID]
	next := state.Turns[nextTurnID]
	if turn == nil || next == nil || next.ContinuationStart == nil || next.Continuation == nil {
		return false
	}
	if strings.TrimSpace(next.ContinuationStart.PreviousTurnID) != turnID {
		return false
	}
	if strings.TrimSpace(historyCompactionSummaryText(next.Continuation)) == "" {
		return false
	}
	return turnHasHistoryCompactionTranscriptEntry(turn) || effectiveTurnHistoryContinuation(state, turnID, turn) != nil
}
