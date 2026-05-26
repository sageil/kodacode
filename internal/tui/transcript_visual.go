package tui

import (
	"errors"

	"github.com/rivo/uniseg"
)

var ErrTranscriptSelectionInactive = errors.New("transcript visual selection is not active")

type transcriptCursorPosition struct {
	line   int
	column int
}

func transcriptGraphemeCount(text string) int {
	return uniseg.GraphemeClusterCount(text)
}

func (m *Model) syncTranscriptVisualState() {
	if m == nil {
		return
	}
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		m.transcriptView.selectionLines = nil
		m.transcriptView.cursorLine = 0
		m.transcriptView.cursorColumn = 0
		m.transcriptView.cursorGoalColumn = 0
		m.transcriptView.mouseSelecting = false
		m.transcriptView.mouseAnchorLine = 0
		m.transcriptView.mouseAnchorColumn = 0
		m.transcriptView.visualAnchorLine = 0
		m.transcriptView.visualAnchorColumn = 0
		m.transcriptView.cursorInitialized = false
		m.transcriptView.visualActive = false
		return
	}
	if !m.transcriptView.cursorInitialized {
		m.syncTranscriptCursorToViewport()
	}
	maxLine := lineCount - 1
	m.transcriptView.cursorLine = clampTranscriptLineIndex(m.transcriptView.cursorLine, maxLine)
	if selectable, ok := m.nearestSelectableTranscriptLine(m.transcriptView.cursorLine, 1); ok && !m.transcriptLineSelectable(m.transcriptView.cursorLine) {
		m.transcriptView.cursorLine = selectable
	}
	if selectable, ok := m.nearestSelectableTranscriptLine(m.transcriptView.cursorLine, -1); ok && !m.transcriptLineSelectable(m.transcriptView.cursorLine) {
		m.transcriptView.cursorLine = selectable
	}
	m.transcriptView.cursorColumn = clampTranscriptCursorColumn(m.transcriptView.cursorColumn, m.transcriptLineGraphemeCount(m.transcriptView.cursorLine))
	m.transcriptView.mouseAnchorLine = clampTranscriptLineIndex(m.transcriptView.mouseAnchorLine, maxLine)
	m.transcriptView.mouseAnchorColumn = clampTranscriptCursorColumn(m.transcriptView.mouseAnchorColumn, m.transcriptLineGraphemeCount(m.transcriptView.mouseAnchorLine))
	m.transcriptView.visualAnchorLine = clampTranscriptLineIndex(m.transcriptView.visualAnchorLine, maxLine)
	m.transcriptView.visualAnchorColumn = clampTranscriptCursorColumn(m.transcriptView.visualAnchorColumn, m.transcriptLineGraphemeCount(m.transcriptView.visualAnchorLine))
	if m.transcriptView.visualActive {
		m.ensureTranscriptCursorVisible()
	}
}
