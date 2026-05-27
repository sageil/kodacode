package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func (layout transcriptLayout) rendered() transcriptRender {
	chunks := make([]transcriptRender, 0, len(layout.chunks))
	visibleTurnSeen := false
	for _, chunk := range layout.chunks {
		rendered := chunk.rendered
		content := strings.TrimRight(rendered.content, "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}
		rendered.content = content
		if chunk.kind == transcriptLayoutChunkTurn {
			if visibleTurnSeen && !layout.wide && strings.TrimSpace(layout.turnSeparator.content) != "" {
				chunks = append(chunks, layout.turnSeparator)
			}
			visibleTurnSeen = true
		}
		chunks = append(chunks, rendered)
	}
	return buildTranscriptFromChunks(chunks)
}

func newTranscriptSelectionLine(text string, prefixGraphemes int) transcriptSelectionLine {
	text = strings.TrimRight(text, " \t")
	if prefixGraphemes < 0 {
		prefixGraphemes = 0
	}
	return transcriptSelectionLine{
		text:            text,
		prefixGraphemes: prefixGraphemes,
		graphemeCount:   transcriptGraphemeCount(text),
	}
}

func transcriptSelectionLinesForContent(content string) []transcriptSelectionLine {
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	selection := make([]transcriptSelectionLine, 0, len(lines))
	for _, line := range lines {
		stripped := ansi.Strip(line)
		switch {
		case stripped == userPromptRailGlyph || stripped == asciiUserPromptRailGlyph:
			selection = append(selection, transcriptSelectionLine{})
		case strings.HasPrefix(stripped, asciiUserPromptContentPrefixValue):
			selection = append(selection, newTranscriptSelectionLine(strings.TrimPrefix(stripped, asciiUserPromptContentPrefixValue), transcriptGraphemeCount(asciiUserPromptContentPrefixValue)))
		case strings.HasPrefix(stripped, userPromptContentPrefix()):
			selection = append(selection, newTranscriptSelectionLine(strings.TrimPrefix(stripped, userPromptContentPrefix()), userPromptContentPrefixGraphemeCount))
		default:
			selection = append(selection, newTranscriptSelectionLine(stripped, 0))
		}
	}
	return selection
}

func normalizedTranscriptSelectionLines(content string, explicit []transcriptSelectionLine) []transcriptSelectionLine {
	content = strings.TrimRight(content, "\n")
	if strings.TrimSpace(content) == "" {
		return nil
	}
	expected := strings.Count(content, "\n") + 1
	if explicit == nil {
		return transcriptSelectionLinesForContent(content)
	}
	if len(explicit) != expected {
		return transcriptSelectionLinesForContent(content)
	}
	return explicit
}

func (layout transcriptLayout) canRefreshTurns(width int, wide bool, turnIDs []string) bool {
	if layout.width != max(width, 1) || layout.wide != wide || len(layout.chunks) == 0 {
		return false
	}
	seenTurn := false
	for _, turnID := range turnIDs {
		turnID = strings.TrimSpace(turnID)
		if turnID == "" {
			continue
		}
		seenTurn = true
		index, ok := layout.turnIndices[turnID]
		if !ok || index < 0 || index >= len(layout.chunks) {
			return false
		}
		if layout.chunks[index].kind != transcriptLayoutChunkTurn {
			return false
		}
	}
	return seenTurn
}

func buildTranscriptRender(sections []transcriptSection) transcriptRender {
	return buildTranscriptSections(sections, true)
}

func buildTranscriptSections(sections []transcriptSection, addBottomPadding bool) transcriptRender {
	parts := make([]string, 0, len(sections))
	selectionLines := make([]transcriptSelectionLine, 0, len(sections)*4)
	separator := "\n\n"
	for _, section := range sections {
		if strings.TrimSpace(section.content) == "" {
			continue
		}
		parts = append(parts, section.content)
	}
	if len(parts) == 0 {
		return transcriptRender{}
	}

	content := strings.Join(parts, separator)
	toolLines := make(map[sessionToolCallRef]int)
	line := 0
	index := 0
	for _, section := range sections {
		if strings.TrimSpace(section.content) == "" {
			continue
		}
		if index > 0 {
			line += strings.Count(separator, "\n")
			selectionLines = append(selectionLines, transcriptSelectionLine{}, transcriptSelectionLine{})
		}
		if len(section.toolLineRefs) > 0 {
			for ref, sectionLine := range section.toolLineRefs {
				toolLines[ref] = line + max(sectionLine, 0)
			}
		} else {
			for _, ref := range section.toolRefs {
				toolLines[ref] = line
			}
		}
		selectionLines = append(selectionLines, normalizedTranscriptSelectionLines(section.content, section.selectionLines)...)
		line += transcriptRenderedLineCount(section.content)
		index++
	}
	return transcriptRender{
		content:        contentWithBottomPadding(content, addBottomPadding),
		toolLines:      toolLines,
		selectionLines: selectionLines,
	}
}

func buildTranscriptFromChunks(chunks []transcriptRender) transcriptRender {
	parts := make([]string, 0, len(chunks))
	separator := "\n\n"
	toolLines := make(map[sessionToolCallRef]int)
	selectionLines := make([]transcriptSelectionLine, 0, len(chunks)*4)
	line := 0
	index := 0
	for _, chunk := range chunks {
		content := strings.TrimRight(chunk.content, "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, content)
		if index > 0 {
			line += strings.Count(separator, "\n")
			selectionLines = append(selectionLines, transcriptSelectionLine{}, transcriptSelectionLine{})
		}
		for ref, chunkLine := range chunk.toolLines {
			toolLines[ref] = line + chunkLine
		}
		selectionLines = append(selectionLines, normalizedTranscriptSelectionLines(content, chunk.selectionLines)...)
		line += transcriptRenderedLineCount(content)
		index++
	}
	if len(parts) == 0 {
		return transcriptRender{}
	}
	return transcriptRender{
		content:        contentWithBottomPadding(strings.Join(parts, separator), true),
		toolLines:      toolLines,
		selectionLines: selectionLines,
	}
}

type virtualTranscriptContent struct {
	chunks         []messagesVirtualChunk
	toolLines      map[sessionToolCallRef]int
	selectionLines []transcriptSelectionLine
}

func virtualTranscriptRender(layout transcriptLayout) virtualTranscriptContent {
	out := virtualTranscriptContent{
		toolLines:      make(map[sessionToolCallRef]int),
		selectionLines: make([]transcriptSelectionLine, 0, len(layout.chunks)*4),
	}
	visibleTurnSeen := false
	line := 0
	index := 0
	appendSeparator := func() {
		out.chunks = append(out.chunks, messagesVirtualChunk{blankLines: 2})
		out.selectionLines = append(out.selectionLines, transcriptSelectionLine{}, transcriptSelectionLine{})
		line += 2
	}
	appendChunk := func(rendered transcriptRender) {
		content := strings.TrimRight(rendered.content, "\n")
		if strings.TrimSpace(content) == "" {
			return
		}
		if index > 0 {
			appendSeparator()
		}
		out.chunks = append(out.chunks, messagesVirtualChunk{content: content})
		for ref, chunkLine := range rendered.toolLines {
			out.toolLines[ref] = line + chunkLine
		}
		out.selectionLines = append(out.selectionLines, normalizedTranscriptSelectionLines(content, rendered.selectionLines)...)
		line += transcriptRenderedLineCount(content)
		index++
	}
	for _, chunk := range layout.chunks {
		rendered := chunk.rendered
		content := strings.TrimRight(rendered.content, "\n")
		if strings.TrimSpace(content) == "" {
			continue
		}
		rendered.content = content
		if chunk.kind == transcriptLayoutChunkTurn {
			if visibleTurnSeen && !layout.wide && strings.TrimSpace(layout.turnSeparator.content) != "" {
				appendChunk(layout.turnSeparator)
			}
			visibleTurnSeen = true
		}
		appendChunk(rendered)
	}
	if index > 0 {
		out.chunks = append(out.chunks, messagesVirtualChunk{blankLines: strings.Count(transcriptBottomPadding, "\n")})
		out.selectionLines = append(out.selectionLines, transcriptSelectionLine{}, transcriptSelectionLine{})
	}
	return out
}

func contentWithBottomPadding(content string, addBottomPadding bool) string {
	if !addBottomPadding {
		return content
	}
	return content + transcriptBottomPadding
}

func transcriptRenderedLineCount(content string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	return strings.Count(content, "\n") + 1
}
