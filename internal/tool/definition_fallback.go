package tool

import (
	"os"
	"strings"
	"unicode"
	"unicode/utf16"

	"github.com/sageil/kodacode/internal/lsp"
)

type utf16Span struct {
	start int
	end   int
}

func bestEffortPositionCandidates(filePath string, line, character int) []int {
	candidates := []int{character}
	if character < 0 {
		return candidates
	}
	span, ok := nearestSymbolSpanForFile(filePath, line, character)
	if !ok {
		return candidates
	}
	candidates = appendUniqueInt(candidates, clampCharacterToSpan(character, span))
	candidates = appendUniqueInt(candidates, span.start)
	if span.end > span.start+1 {
		candidates = appendUniqueInt(candidates, span.end-1)
	}
	return candidates
}

func nearestSymbolSpanForFile(filePath string, line, character int) (utf16Span, bool) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return utf16Span{}, false
	}
	return nearestSymbolSpanInContent(string(content), line, character)
}

func nearestSymbolSpanInContent(content string, line, character int) (utf16Span, bool) {
	text, ok := lineTextAt(content, line)
	if !ok {
		return utf16Span{}, false
	}
	spans := collectUTF16Spans(text, isSymbolRune)
	if len(spans) == 0 {
		spans = collectUTF16Spans(text, func(r rune) bool { return !unicode.IsSpace(r) })
	}
	if len(spans) == 0 {
		return utf16Span{}, false
	}
	best := spans[0]
	for _, span := range spans[1:] {
		if preferSpan(span, best, character) {
			best = span
		}
	}
	return best, true
}

func lineTextAt(content string, line int) (string, bool) {
	offsets := strings.Split(content, "\n")
	if line < 1 || line > len(offsets) {
		return "", false
	}
	return offsets[line-1], true
}

func collectUTF16Spans(text string, include func(rune) bool) []utf16Span {
	var spans []utf16Span
	start := -1
	position := 0
	for _, r := range text {
		width := utf16WidthForRune(r)
		if include(r) {
			if start < 0 {
				start = position
			}
		} else if start >= 0 {
			spans = append(spans, utf16Span{start: start, end: position})
			start = -1
		}
		position += width
	}
	if start >= 0 {
		spans = append(spans, utf16Span{start: start, end: position})
	}
	return spans
}

func isSymbolRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_' || r == '$'
}

func utf16WidthForRune(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	width := utf16.RuneLen(r)
	if width < 0 {
		return 1
	}
	return width
}

func preferSpan(candidate, current utf16Span, character int) bool {
	candidateDistance := spanDistance(candidate, character)
	currentDistance := spanDistance(current, character)
	if candidateDistance != currentDistance {
		return candidateDistance < currentDistance
	}
	candidateContains := character >= candidate.start && character < candidate.end
	currentContains := character >= current.start && character < current.end
	if candidateContains != currentContains {
		return candidateContains
	}
	if candidate.start != current.start {
		return candidate.start < current.start
	}
	return candidate.end-candidate.start > current.end-current.start
}

func spanDistance(span utf16Span, character int) int {
	if character < span.start {
		return span.start - character
	}
	if character >= span.end {
		return character - span.end + 1
	}
	return 0
}

func clampCharacterToSpan(character int, span utf16Span) int {
	if character < span.start {
		return span.start
	}
	if character >= span.end {
		return span.end - 1
	}
	return character
}

func appendUniqueInt(dst []int, value int) []int {
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}

func BestEffortPositionCandidates(filePath string, line, character int) []int {
	return bestEffortPositionCandidates(filePath, line, character)
}

func BestEffortCodeActionRanges(filePath string, startLine, startCharacter, endLine, endCharacter int) []lsp.Range {
	if startLine < 1 || endLine < 1 || startLine != endLine || startCharacter < 0 || endCharacter < 0 {
		return nil
	}
	span, ok := nearestSymbolSpanForFile(filePath, startLine, startCharacter)
	if !ok {
		return nil
	}
	line := startLine - 1
	cursor := clampCharacterToSpan(startCharacter, span)
	var ranges []lsp.Range
	ranges = appendUniqueRange(ranges, lsp.Range{
		Start: lsp.Position{Line: line, Character: cursor},
		End:   lsp.Position{Line: line, Character: cursor},
	})
	ranges = appendUniqueRange(ranges, lsp.Range{
		Start: lsp.Position{Line: line, Character: span.start},
		End:   lsp.Position{Line: line, Character: span.end},
	})
	return ranges
}

func AppendUniqueRange(dst []lsp.Range, value lsp.Range) []lsp.Range {
	return appendUniqueRange(dst, value)
}

func WorkspaceEditHasChanges(edit *lsp.WorkspaceEdit) bool {
	return edit != nil && (len(edit.Changes) > 0 || len(edit.DocumentChanges) > 0)
}

func appendUniqueRange(dst []lsp.Range, value lsp.Range) []lsp.Range {
	for _, existing := range dst {
		if existing == value {
			return dst
		}
	}
	return append(dst, value)
}
