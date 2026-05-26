package tui

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

func (m *Model) syncTranscriptCursorToViewport() {
	if m == nil || m.transcriptView.visualActive {
		return
	}
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		m.transcriptView.selectionLines = nil
		m.transcriptView.cursorLine = 0
		m.transcriptView.cursorColumn = 0
		m.transcriptView.cursorGoalColumn = 0
		m.transcriptView.cursorInitialized = false
		return
	}
	line := clampTranscriptLineIndex(m.messages.YOffset(), lineCount-1)
	if selectable, ok := m.nearestSelectableTranscriptLine(line, 1); ok {
		line = selectable
	} else if selectable, ok := m.nearestSelectableTranscriptLine(line, -1); ok {
		line = selectable
	}
	m.transcriptView.cursorLine = line
	m.transcriptView.cursorColumn = clampTranscriptCursorColumn(m.transcriptView.cursorGoalColumn, m.transcriptLineGraphemeCount(line))
	m.transcriptView.cursorInitialized = true
}

func (m *Model) ensureTranscriptCursorVisible() {
	if m == nil || !m.transcriptView.cursorInitialized {
		return
	}
	height := max(m.messages.Height(), 1)
	offset := m.messages.YOffset()
	switch {
	case m.transcriptView.cursorLine < offset:
		m.messages.GotoLine(m.transcriptView.cursorLine)
	case m.transcriptView.cursorLine >= offset+height:
		m.messages.GotoLine(max(m.transcriptView.cursorLine-height+1, 0))
	}
}

func (m *Model) setTranscriptCursorPosition(pos transcriptCursorPosition) {
	if m == nil {
		return
	}
	m.syncTranscriptVisualState()
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		return
	}
	line := clampTranscriptLineIndex(pos.line, lineCount-1)
	if !m.transcriptLineSelectable(line) {
		if selectable, ok := m.nearestSelectableTranscriptLine(line, 1); ok {
			line = selectable
		} else if selectable, ok := m.nearestSelectableTranscriptLine(line, -1); ok {
			line = selectable
		} else {
			return
		}
	}
	graphemes := m.transcriptLineGraphemeCount(line)
	column := clampTranscriptCursorColumn(pos.column, graphemes)
	m.transcriptView.cursorLine = line
	m.transcriptView.cursorColumn = column
	m.transcriptView.cursorGoalColumn = column
	m.transcriptView.cursorInitialized = true
	m.ensureTranscriptCursorVisible()
}

func (m *Model) moveTranscriptCursor(delta int) {
	if m == nil {
		return
	}
	m.syncTranscriptVisualState()
	if delta == 0 || m.transcriptLineCount() <= 0 || !m.transcriptView.cursorInitialized {
		return
	}
	step := 1
	if delta < 0 {
		step = -1
		delta = -delta
	}
	line := m.transcriptView.cursorLine
	for moved := 0; moved < delta; moved++ {
		next, ok := m.nextSelectableTranscriptLine(line+step, step)
		if !ok {
			break
		}
		line = next
	}
	m.transcriptView.cursorLine = line
	m.transcriptView.cursorColumn = clampTranscriptCursorColumn(m.transcriptView.cursorGoalColumn, m.transcriptLineGraphemeCount(line))
	m.ensureTranscriptCursorVisible()
}

func (m *Model) moveTranscriptCursorHorizontal(delta int) {
	if m == nil {
		return
	}
	m.syncTranscriptVisualState()
	if !m.transcriptView.cursorInitialized {
		return
	}
	graphemes := m.transcriptLineGraphemeCount(m.transcriptView.cursorLine)
	if graphemes <= 0 {
		return
	}
	m.transcriptView.cursorColumn = clampTranscriptCursorColumn(m.transcriptView.cursorColumn+delta, graphemes)
	m.transcriptView.cursorGoalColumn = m.transcriptView.cursorColumn
	m.ensureTranscriptCursorVisible()
}

func (m *Model) pageTranscriptCursor(deltaPages int) {
	if m == nil {
		return
	}
	step := max(m.messages.Height()-1, 1)
	m.moveTranscriptCursor(deltaPages * step)
}

func (m *Model) moveTranscriptCursorToStart() {
	if m == nil {
		return
	}
	line, ok := m.nearestSelectableTranscriptLine(0, 1)
	if !ok {
		return
	}
	m.transcriptView.cursorLine = line
	m.transcriptView.cursorColumn = 0
	m.transcriptView.cursorGoalColumn = 0
	m.transcriptView.cursorInitialized = true
	m.ensureTranscriptCursorVisible()
}

func (m *Model) moveTranscriptCursorToEnd() {
	if m == nil {
		return
	}
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		return
	}
	line, ok := m.nearestSelectableTranscriptLine(lineCount-1, -1)
	if !ok {
		return
	}
	m.transcriptView.cursorLine = line
	graphemes := m.transcriptLineGraphemeCount(line)
	m.transcriptView.cursorColumn = clampTranscriptCursorColumn(graphemes-1, graphemes)
	m.transcriptView.cursorGoalColumn = m.transcriptView.cursorColumn
	m.transcriptView.cursorInitialized = true
	m.ensureTranscriptCursorVisible()
}

func (m *Model) moveTranscriptCursorToLineStart() {
	if m == nil {
		return
	}
	m.syncTranscriptVisualState()
	if !m.transcriptView.cursorInitialized || m.transcriptLineGraphemeCount(m.transcriptView.cursorLine) <= 0 {
		return
	}
	m.transcriptView.cursorColumn = 0
	m.transcriptView.cursorGoalColumn = 0
	m.ensureTranscriptCursorVisible()
}

func (m *Model) moveTranscriptCursorToLineEnd() {
	if m == nil {
		return
	}
	m.syncTranscriptVisualState()
	if !m.transcriptView.cursorInitialized {
		return
	}
	graphemes := m.transcriptLineGraphemeCount(m.transcriptView.cursorLine)
	if graphemes <= 0 {
		return
	}
	m.transcriptView.cursorColumn = graphemes - 1
	m.transcriptView.cursorGoalColumn = m.transcriptView.cursorColumn
	m.ensureTranscriptCursorVisible()
}

func clampTranscriptLineIndex(line, maxLine int) int {
	if maxLine < 0 {
		return 0
	}
	if line < 0 {
		return 0
	}
	if line > maxLine {
		return maxLine
	}
	return line
}

func clampTranscriptCursorColumn(column, graphemes int) int {
	if graphemes <= 0 {
		return 0
	}
	if column < 0 {
		return 0
	}
	if column > graphemes-1 {
		return graphemes - 1
	}
	return column
}

func transcriptRawColumnForDisplayX(text string, x int) int {
	if x <= 0 {
		return 0
	}
	graphemes := uniseg.NewGraphemes(text)
	cell := 0
	index := 0
	last := 0
	for graphemes.Next() {
		cluster := graphemes.Str()
		width := ansi.StringWidth(cluster)
		if width <= 0 {
			width = 1
		}
		if x < cell+width {
			return index
		}
		cell += width
		last = index
		index++
	}
	if index == 0 {
		return 0
	}
	return last
}

func (m Model) nearestSelectableTranscriptLine(start, step int) (int, bool) {
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		return 0, false
	}
	start = clampTranscriptLineIndex(start, lineCount-1)
	if step == 0 {
		step = 1
	}
	for line := start; line >= 0 && line < lineCount; line += step {
		if m.transcriptLineSelectable(line) {
			return line, true
		}
	}
	return 0, false
}

func (m Model) nextSelectableTranscriptLine(start, step int) (int, bool) {
	lineCount := m.transcriptLineCount()
	if lineCount <= 0 {
		return 0, false
	}
	if start < 0 || start >= lineCount {
		return 0, false
	}
	if step == 0 {
		step = 1
	}
	for line := start; line >= 0 && line < lineCount; line += step {
		if m.transcriptLineSelectable(line) {
			return line, true
		}
	}
	return 0, false
}
