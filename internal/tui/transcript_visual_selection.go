package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) startTranscriptVisualSelection() {
	if m == nil {
		return
	}
	m.syncTranscriptVisualState()
	if !m.transcriptView.cursorInitialized {
		return
	}
	m.transcriptView.visualActive = true
	m.transcriptView.visualAnchorLine = m.transcriptView.cursorLine
	m.transcriptView.visualAnchorColumn = m.transcriptView.cursorColumn
}

func (m *Model) clearTranscriptVisualSelection() {
	if m == nil {
		return
	}
	m.transcriptView.visualActive = false
}

func (m *Model) clearTranscriptMouseSelection() {
	if m == nil {
		return
	}
	m.transcriptView.mouseSelecting = false
	m.transcriptView.mouseAnchorLine = 0
	m.transcriptView.mouseAnchorColumn = 0
}

func (m Model) transcriptSelectionRange() (int, int, bool) {
	start, end, ok := m.orderedTranscriptSelectionBounds()
	if !ok {
		return 0, 0, false
	}
	return start.line, end.line, true
}

func (m Model) transcriptLineCount() int {
	if len(m.transcriptView.selectionLines) > 0 {
		return len(m.transcriptView.selectionLines)
	}
	return len(m.transcriptRawLines())
}

func (m Model) transcriptRawLines() []string {
	return m.messages.RawLines()
}

func (m Model) transcriptSelectionLineAt(line int) transcriptSelectionLine {
	if line < 0 {
		return transcriptSelectionLine{}
	}
	if line < len(m.transcriptView.selectionLines) {
		explicit := m.transcriptView.selectionLines[line]
		if derived, ok := m.transcriptSelectionLineFromRaw(line, explicit.prefixGraphemes); ok && derived.graphemeCount > explicit.graphemeCount {
			return derived
		}
		return explicit
	}
	if derived, ok := m.transcriptSelectionLineFromRaw(line, 0); ok {
		return derived
	}
	return transcriptSelectionLine{}
}

func (m Model) transcriptSelectionLineFromRaw(line, prefixGraphemes int) (transcriptSelectionLine, bool) {
	lines := m.transcriptRawLines()
	if line < 0 || line >= len(lines) {
		return transcriptSelectionLine{}, false
	}
	stripped := strings.TrimRight(ansi.Strip(lines[line]), " \t")
	if prefixGraphemes > 0 {
		total := transcriptGraphemeCount(stripped)
		if total <= prefixGraphemes {
			return transcriptSelectionLine{}, true
		}
		return newTranscriptSelectionLine(ansi.Cut(stripped, prefixGraphemes, total), prefixGraphemes), true
	}
	switch {
	case stripped == m.userPromptRailGlyph() || stripped == userPromptRailGlyph || stripped == asciiUserPromptRailGlyph:
		return transcriptSelectionLine{}, true
	case strings.HasPrefix(stripped, m.userPromptContentPrefix()):
		return newTranscriptSelectionLine(strings.TrimPrefix(stripped, m.userPromptContentPrefix()), m.userPromptContentPrefixGraphemeCount()), true
	case strings.HasPrefix(stripped, asciiUserPromptContentPrefixValue):
		return newTranscriptSelectionLine(strings.TrimPrefix(stripped, asciiUserPromptContentPrefixValue), transcriptGraphemeCount(asciiUserPromptContentPrefixValue)), true
	case strings.HasPrefix(stripped, userPromptContentPrefix()):
		return newTranscriptSelectionLine(strings.TrimPrefix(stripped, userPromptContentPrefix()), userPromptContentPrefixGraphemeCount), true
	default:
		return newTranscriptSelectionLine(stripped, 0), true
	}
}

func (m Model) visibleTranscriptLine(line int) (string, bool) {
	if line < 0 {
		return "", false
	}
	base := m.messages.YOffset()
	index := line - base
	if index < 0 {
		return "", false
	}
	lines := m.messages.VisibleLines()
	if index >= len(lines) {
		return "", false
	}
	return lines[index], true
}

func (m Model) transcriptLineGraphemeCount(line int) int {
	return m.transcriptSelectionLineAt(line).graphemeCount
}

func (m Model) transcriptLineSelectable(line int) bool {
	return m.transcriptLineGraphemeCount(line) > 0
}

func (m Model) transcriptLineSelected(line int) bool {
	if start, end, ok := m.transcriptSelectionRange(); ok {
		return line >= start && line <= end
	}
	return false
}

func (m Model) transcriptLineSelectionBounds(line int) (int, int, bool) {
	start, end, ok := m.orderedTranscriptSelectionBounds()
	if !ok {
		return 0, 0, false
	}
	graphemes := m.transcriptLineGraphemeCount(line)
	switch {
	case line < start.line || line > end.line:
		return 0, 0, false
	case start.line == end.line:
		return clampTranscriptSelectionSpan(start.column, end.column+1, graphemes)
	case line == start.line:
		return clampTranscriptSelectionSpan(start.column, graphemes, graphemes)
	case line == end.line:
		return clampTranscriptSelectionSpan(0, end.column+1, graphemes)
	default:
		return clampTranscriptSelectionSpan(0, graphemes, graphemes)
	}
}

func (m Model) orderedTranscriptSelectionBounds() (transcriptCursorPosition, transcriptCursorPosition, bool) {
	if !m.transcriptView.visualActive || !m.transcriptView.cursorInitialized || m.transcriptLineCount() <= 0 {
		return transcriptCursorPosition{}, transcriptCursorPosition{}, false
	}
	start := transcriptCursorPosition{
		line:   m.transcriptView.visualAnchorLine,
		column: m.transcriptView.visualAnchorColumn,
	}
	end := transcriptCursorPosition{
		line:   m.transcriptView.cursorLine,
		column: m.transcriptView.cursorColumn,
	}
	if compareTranscriptCursorPosition(start, end) > 0 {
		start, end = end, start
	}
	return start, end, true
}

func compareTranscriptCursorPosition(left, right transcriptCursorPosition) int {
	switch {
	case left.line < right.line:
		return -1
	case left.line > right.line:
		return 1
	case left.column < right.column:
		return -1
	case left.column > right.column:
		return 1
	default:
		return 0
	}
}

func clampTranscriptSelectionSpan(start, end, graphemes int) (int, int, bool) {
	if graphemes <= 0 {
		return 0, 0, false
	}
	if start < 0 {
		start = 0
	}
	if start > graphemes-1 {
		start = graphemes - 1
	}
	if end < start+1 {
		end = start + 1
	}
	if end > graphemes {
		end = graphemes
	}
	return start, end, end > start
}

func (m Model) copyTranscriptVisualSelectionCmd() tea.Cmd {
	if !m.transcriptView.visualActive {
		return func() tea.Msg {
			return transcriptCopiedMsg{err: ErrTranscriptSelectionInactive}
		}
	}
	text, err := m.transcriptVisualSelectionText()
	if err != nil {
		return func() tea.Msg {
			return transcriptCopiedMsg{err: err}
		}
	}
	writer := m.clipboard
	if writer == nil {
		writer = systemClipboardWriter{}
	}
	return func() tea.Msg {
		return transcriptCopiedMsg{
			label: "Copied transcript selection",
			err:   writer.WriteText(m.ctx, text),
		}
	}
}

func (m Model) transcriptVisualSelectionText() (string, error) {
	start, end, ok := m.orderedTranscriptSelectionBounds()
	if !ok {
		return "", ErrTranscriptSelectionInactive
	}
	lines := make([]string, 0, end.line-start.line+1)
	for line := start.line; line <= end.line; line++ {
		selectionLine := m.transcriptSelectionLineAt(line)
		graphemes := selectionLine.graphemeCount
		switch {
		case start.line == end.line:
			endExclusive := min(end.column+1, graphemes)
			lines = append(lines, ansi.Cut(selectionLine.text, start.column, endExclusive))
		case line == start.line:
			lines = append(lines, ansi.Cut(selectionLine.text, start.column, graphemes))
		case line == end.line:
			endExclusive := min(end.column+1, graphemes)
			lines = append(lines, ansi.Cut(selectionLine.text, 0, endExclusive))
		default:
			lines = append(lines, selectionLine.text)
		}
	}
	text := normalizeTranscriptCopyText(strings.Join(lines, "\n"))
	if text == "" {
		return "", ErrTranscriptCopyUnavailable
	}
	return text, nil
}
