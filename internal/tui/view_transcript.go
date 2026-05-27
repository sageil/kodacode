package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
)

const (
	transcriptScrollbarWidth = 1
)

var transcriptScrollbarThumbGlyph = terminalIcon(terminalIconScrollThumb)

func transcriptViewportWidth(width int) int {
	return transcriptViewportContentWidth(width, true)
}

func transcriptViewportContentWidth(width int, showScrollbar bool) int {
	if !showScrollbar {
		return max(width, 1)
	}
	return max(width-transcriptScrollbarWidth, 1)
}

func renderTranscriptPane(m Model, state events.SessionState, width int) string {
	if m.renderCache.transcriptPane == nil {
		return renderTranscriptPaneUncached(m, state, width)
	}
	return m.renderCache.transcriptPane.renderedFor(transcriptPaneCacheKey(m, state, width), func() string {
		return renderTranscriptPaneUncached(m, state, width)
	})
}

func renderTranscriptPaneUncached(m Model, state events.SessionState, width int) string {
	body, activity := renderTranscriptPaneSections(m, state, width)
	if activity != "" {
		if body != "" {
			body += "\n"
		}
		body += activity
	}
	return body
}

func renderTranscriptPaneSections(m Model, state events.SessionState, width int) (string, string) {
	return renderTranscriptPaneSectionsWithOptions(m, state, width, transcriptPaneRenderOptions{showScrollbar: true})
}

type transcriptPaneRenderOptions struct {
	showScrollbar bool
}

func renderTranscriptPaneSectionsWithOptions(m Model, state events.SessionState, width int, opts transcriptPaneRenderOptions) (string, string) {
	opts.showScrollbar = transcriptPaneShowScrollbar(m, opts.showScrollbar)
	viewport := renderTranscriptViewportWithOptions(m, max(width, 1), opts)
	statusBar := strings.TrimSpace(renderTranscriptStatusBar(m, state, max(width, 1)))
	body := viewport
	if panel := renderInlinePermissionPrompt(m, state, max(width, 1)); panel != "" {
		body = panel + "\n\n" + body
	}
	if panel := renderInlineQuestionPrompt(m, max(width, 1)); panel != "" {
		body = panel + "\n\n" + body
	}
	return body, statusBar
}

func renderTranscriptViewport(m Model, width int) string {
	return renderTranscriptViewportWithOptions(m, width, transcriptPaneRenderOptions{showScrollbar: true})
}

func renderTranscriptViewportWithOptions(m Model, width int, opts transcriptPaneRenderOptions) string {
	opts.showScrollbar = transcriptPaneShowScrollbar(m, opts.showScrollbar)
	height := max(m.messages.Height(), 1)
	contentWidth := min(max(m.messages.Width(), 1), transcriptViewportContentWidth(width, opts.showScrollbar))
	lines := append([]string(nil), m.messages.VisibleLines()...)
	gutter := renderTranscriptScrollbar(m, height, opts.showScrollbar)
	padWidth := max(width-contentWidth-transcriptScrollbarRenderWidth(opts.showScrollbar), 0)
	baseLine := m.messages.YOffset()
	emptyLine := strings.Repeat(" ", contentWidth)
	pad := strings.Repeat(" ", padWidth)

	for len(lines) < height {
		lines = append(lines, emptyLine)
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	for i := range lines {
		absoluteLine := baseLine + i
		lines[i] = styleTranscriptViewportLine(m, absoluteLine, lines[i], contentWidth)
		if i < len(gutter) {
			lines[i] += renderTranscriptViewportGutter(m, absoluteLine, gutter[i])
		}
		if padWidth > 0 {
			lines[i] += pad
		}
	}
	return strings.Join(lines, "\n")
}

func styleTranscriptViewportLine(m Model, lineIndex int, line string, width int) string {
	if m.transcriptView.visualActive {
		if start, end, ok := m.transcriptLineSelectionBounds(lineIndex); ok {
			return highlightTranscriptViewportLine(m, lineIndex, line, width, start, end, true)
		}
		if m.transcriptLineSelected(lineIndex) {
			bg := transcriptVisualSelectionBG(m)
			if strings.TrimSpace(bg) != "" {
				return fillBackground(max(width, 1), bg, line)
			}
		}
		return line
	}
	return line
}

func transcriptPaneShowScrollbar(m Model, requested bool) bool {
	if !requested {
		return false
	}
	return !dialogHidesTranscriptScrollbar(m.dialog)
}

func dialogHidesTranscriptScrollbar(dialog dialogModel) bool {
	if dialog == nil {
		return false
	}
	switch dialog.ID() {
	case dialogIDToolDetail, dialogIDHandoffDetail, dialogIDTaskDetail:
		return true
	default:
		return false
	}
}

func highlightTranscriptViewportLine(m Model, lineIndex int, line string, width, start, end int, visual bool) string {
	selectionLine := m.transcriptSelectionLineAt(lineIndex)
	bg, fg := transcriptCursorColors(m)
	if visual {
		bg, fg = transcriptSelectionColors(m)
	}
	if strings.TrimSpace(bg) == "" {
		return line
	}
	rawStart := selectionLine.prefixGraphemes + start
	rawEnd := selectionLine.prefixGraphemes + end
	total := transcriptGraphemeCount(ansi.Strip(line))
	if rawStart < 0 {
		rawStart = 0
	}
	if rawStart > total {
		rawStart = total
	}
	if rawEnd < rawStart {
		rawEnd = rawStart
	}
	if rawEnd > total {
		rawEnd = total
	}
	if rawEnd <= rawStart {
		return fillBackground(max(width, 1), bg, line)
	}
	prefix := ansi.Cut(line, 0, rawStart)
	segment := ansi.Cut(line, rawStart, rawEnd)
	suffix := ansi.Cut(line, rawEnd, total)
	return prefix + highlightTranscriptViewportSegment(bg, fg, segment) + suffix
}

func highlightTranscriptViewportSegment(bg, fg, segment string) string {
	if segment == "" {
		return segment
	}
	style := lipgloss.NewStyle().Background(lipgloss.Color(bg))
	if strings.TrimSpace(fg) != "" {
		style = style.Foreground(lipgloss.Color(fg))
	}
	return style.Render(ansi.Strip(segment))
}

func transcriptSelectionColors(m Model) (string, string) {
	bg := colorFor(m.theme, "primary", "#7aa2f7")
	fg := colorFor(m.theme, "surface", "#10141c")
	if m.theme != nil && m.theme.Components != nil {
		style := m.theme.Resolve("selection")
		if style.BG != nil && strings.TrimSpace(*style.BG) != "" {
			bg = strings.TrimSpace(*style.BG)
		}
		if style.FG != nil && strings.TrimSpace(*style.FG) != "" {
			fg = strings.TrimSpace(*style.FG)
		}
	}
	return bg, fg
}

func transcriptVisualSelectionBG(m Model) string {
	bg, _ := transcriptSelectionColors(m)
	return bg
}

func transcriptCursorColors(m Model) (string, string) {
	bg := toneValue(m.theme, toneLineStrong)
	if strings.TrimSpace(bg) == "" {
		bg = colorFor(m.theme, "warning", "#ffd28f")
	}
	return bg, colorFor(m.theme, "text", "#ecf0ff")
}

func renderTranscriptViewportGutter(m Model, lineIndex int, fallback string) string {
	switch {
	case m.transcriptLineSelected(lineIndex):
		return lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7aa2f7"))).
			Render("█")
	default:
		return fallback
	}
}

func transcriptScrollbarRenderWidth(showScrollbar bool) int {
	if !showScrollbar {
		return 0
	}
	return transcriptScrollbarWidth
}

func renderTranscriptScrollbar(m Model, height int, showScrollbar bool) []string {
	if height <= 0 {
		return nil
	}
	if !showScrollbar {
		return nil
	}
	total := m.messages.TotalLineCount()
	visible := max(m.messages.Height(), 1)
	if total <= visible {
		return blankTranscriptScrollbar(height)
	}

	offset := m.messages.YOffset()
	thumbHeight := max(1, height*height/max(total, 1))
	if thumbHeight > height {
		thumbHeight = height
	}
	trackSpan := max(height-thumbHeight, 0)
	scrollSpan := max(total-visible, 1)
	thumbStart := 0
	if trackSpan > 0 {
		thumbStart = offset * trackSpan / scrollSpan
	}

	thumbStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7aa2f7")))

	lines := blankTranscriptScrollbar(height)
	for i := 0; i < height; i++ {
		if i >= thumbStart && i < thumbStart+thumbHeight {
			lines[i] = thumbStyle.Render(transcriptScrollbarThumbGlyph)
		}
	}
	return lines
}

func blankTranscriptScrollbar(height int) []string {
	lines := make([]string, height)
	for i := 0; i < height; i++ {
		lines[i] = " "
	}
	return lines
}

func transcriptAgentLabel(m Model, state events.SessionState, turnID string) string {
	if turn := currentTurn(state, turnID); turn != nil {
		if isLocalShellTurn(turn) {
			return "local shell"
		}
	}
	if agentID := strings.TrimSpace(m.agentID); agentID != "" {
		return agentID
	}
	if turn := currentTurn(state, turnID); turn != nil && turn.Config != nil {
		if agentID := strings.TrimSpace(turn.Config.AgentID); agentID != "" {
			return agentID
		}
	}
	return "builder"
}

func summarizeInlineValue(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	line := strings.ReplaceAll(trimmed, "\n", " ")
	if len(line) <= 90 {
		return line
	}
	return line[:87] + "..."
}
