package tool

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type replaceFailure struct {
	code    string
	message string
}

type replacementSpan struct {
	start int
	end   int
	text  string
	index int
}

func (e *replaceFailure) Error() string { return e.message }

func (e *replaceFailure) retryable() bool {
	return e.code == "ambiguous" || e.code == "not_found"
}

type lineScope struct {
	Start int
	End   int
}

type textRange struct {
	StartLine      int `json:"startLine"`
	StartCharacter int `json:"startCharacter"`
	EndLine        int `json:"endLine"`
	EndCharacter   int `json:"endCharacter"`
}

func (s lineScope) active() bool {
	return s.Start > 0 || s.End > 0
}

func (r textRange) active() bool {
	return r.StartLine > 0 || r.EndLine > 0 || r.StartCharacter > 0 || r.EndCharacter > 0
}

func normalizeLineScope(content string, startLine, endLine int) (lineScope, error) {
	if startLine < 0 || endLine < 0 {
		return lineScope{}, fmt.Errorf("startLine and endLine must be positive when provided")
	}
	if startLine == 0 && endLine == 0 {
		return lineScope{}, nil
	}

	count := lineCount(content)
	if count == 0 {
		count = 1
	}
	if startLine == 0 {
		startLine = 1
	}
	if endLine == 0 {
		endLine = count
	}
	if startLine < 1 || endLine < 1 || startLine > endLine || endLine > count {
		return lineScope{}, fmt.Errorf("invalid line range %d-%d for file with %d lines", startLine, endLine, count)
	}
	return lineScope{Start: startLine, End: endLine}, nil
}

func normalizeTextRange(content string, rng textRange) (textRange, error) {
	if !rng.active() {
		return textRange{}, nil
	}
	if rng.StartLine < 1 || rng.EndLine < 1 {
		return textRange{}, fmt.Errorf("range startLine and endLine must be >= 1")
	}
	if rng.StartCharacter < 0 || rng.EndCharacter < 0 {
		return textRange{}, fmt.Errorf("range startCharacter and endCharacter must be >= 0")
	}
	if rng.EndLine < rng.StartLine {
		return textRange{}, fmt.Errorf("range endLine must be >= startLine")
	}
	if rng.EndLine == rng.StartLine && rng.EndCharacter < rng.StartCharacter {
		return textRange{}, fmt.Errorf("range endCharacter must be >= startCharacter when startLine == endLine")
	}
	if _, err := byteOffsetAtTextPosition(content, rng.StartLine, rng.StartCharacter); err != nil {
		return textRange{}, err
	}
	if _, err := byteOffsetAtTextPosition(content, rng.EndLine, rng.EndCharacter); err != nil {
		return textRange{}, err
	}
	return rng, nil
}

func lineCount(content string) int {
	if content == "" {
		return 1
	}
	count := 1
	for _, r := range content {
		if r == '\n' {
			count++
		}
	}
	if strings.HasSuffix(content, "\n") {
		count--
	}
	if count < 1 {
		return 1
	}
	return count
}

func lineOffsets(content string) []int {
	offsets := []int{0}
	for i, r := range content {
		if r == '\n' && i+1 < len(content) {
			offsets = append(offsets, i+1)
		}
	}
	return offsets
}

func scopedContent(content string, scope lineScope) (prefix, segment, suffix string) {
	if !scope.active() {
		return "", content, ""
	}
	offsets := lineOffsets(content)
	start := offsets[scope.Start-1]
	end := len(content)
	if scope.End < len(offsets) {
		end = offsets[scope.End]
	}
	return content[:start], content[start:end], content[end:]
}

func replaceByTextRange(content, old, new string, rng textRange) (string, error) {
	span, err := locateRangeReplacement(content, old, new, rng)
	if err != nil {
		return "", err
	}
	return applyReplacementSpans(content, []replacementSpan{span}), nil
}

func locateRangeReplacement(content, old, new string, rng textRange) (replacementSpan, error) {
	start, end, err := byteOffsetsForTextRange(content, rng)
	if err != nil {
		return replacementSpan{}, &replaceFailure{code: "invalid_range", message: err.Error()}
	}
	current := content[start:end]
	if old != "" && current != old {
		msg := fmt.Sprintf("range %d:%d-%d:%d does not match oldString.", rng.StartLine, rng.StartCharacter, rng.EndLine, rng.EndCharacter)
		if snippet := truncateSnippet(strings.TrimSpace(current), 120); snippet != "" {
			msg += fmt.Sprintf(" Current text in range: %q.", snippet)
		}
		msg += " Reread the file and use the exact current text or update the range."
		return replacementSpan{}, &replaceFailure{code: "not_found", message: msg}
	}
	return replacementSpan{start: start, end: end, text: new}, nil
}

func locateExactReplacement(content, old, new string, scope lineScope) (replacementSpan, error) {
	prefix, segment, _ := scopedContent(content, scope)
	idx, last := strings.Index(segment, old), strings.LastIndex(segment, old)
	if idx < 0 {
		return replacementSpan{}, &replaceFailure{
			code:    "not_found",
			message: replaceFailureMessage("not_found", segment, old, scope),
		}
	}
	if idx != last {
		return replacementSpan{}, &replaceFailure{
			code:    "ambiguous",
			message: replaceFailureMessage("ambiguous", segment, old, scope),
		}
	}
	start := len(prefix) + idx
	return replacementSpan{start: start, end: start + len(old), text: new}, nil
}

func applyReplacementSpans(content string, spans []replacementSpan) string {
	if len(spans) == 0 {
		return content
	}
	out := content
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		out = out[:span.start] + span.text + out[span.end:]
	}
	return out
}

func replaceFailureMessage(kind, content, old string, scope lineScope) string {
	var sb strings.Builder
	switch kind {
	case "ambiguous":
		sb.WriteString("found multiple matches for oldString")
	default:
		sb.WriteString("oldString not found in content")
	}
	if scope.active() {
		fmt.Fprintf(&sb, " within lines %d-%d", scope.Start, scope.End)
	}
	if anchor := firstNonEmptyLine(old); anchor != "" {
		fmt.Fprintf(&sb, `. First non-empty line: %q.`, truncateSnippet(anchor, 120))
	}
	lineBase := 0
	if scope.active() {
		lineBase = scope.Start - 1
	}
	if hints := candidateLineHints(content, old, kind == "ambiguous", lineBase); len(hints) > 0 {
		sb.WriteString(" Candidate lines: ")
		for i, hint := range hints {
			if i > 0 {
				sb.WriteString("; ")
			}
			fmt.Fprintf(&sb, "%d: %s", hint.Line, truncateSnippet(hint.Text, 100))
		}
		sb.WriteString(".")
	}
	if kind == "ambiguous" {
		sb.WriteString(" Use the current exact text and constrain the edit with startLine/endLine or an exact range.")
	} else {
		sb.WriteString(" Reread the file and use the current exact text, or constrain the edit with startLine/endLine or an exact range.")
	}
	return sb.String()
}

type lineHint struct {
	Line int
	Text string
}

func candidateLineHints(content, old string, exactOnly bool, lineBase int) []lineHint {
	anchor := firstNonEmptyLine(old)
	if anchor == "" {
		return nil
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var exact []lineHint
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if trimmed == anchor || strings.Contains(trimmed, anchor) || strings.Contains(anchor, trimmed) {
			exact = append(exact, lineHint{Line: i + 1 + lineBase, Text: trimmed})
		}
	}
	if len(exact) > 0 || exactOnly {
		return limitLineHints(exact, 3)
	}

	type scoredLine struct {
		lineHint
		score float64
	}
	var scored []scoredLine
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		maxLen := max(len(anchor), len(trimmed))
		if maxLen == 0 || maxLen > 200 {
			continue
		}
		score := 1.0 - float64(levenshtein(anchor, trimmed))/float64(maxLen)
		if score < 0.45 {
			continue
		}
		scored = append(scored, scoredLine{lineHint: lineHint{Line: i + 1 + lineBase, Text: trimmed}, score: score})
	}
	for i := 0; i < len(scored); i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}
	hints := make([]lineHint, 0, min(len(scored), 3))
	for i := 0; i < len(scored) && i < 3; i++ {
		hints = append(hints, scored[i].lineHint)
	}
	return hints
}

func limitLineHints(hints []lineHint, n int) []lineHint {
	if len(hints) <= n {
		return hints
	}
	out := make([]lineHint, n)
	copy(out, hints[:n])
	return out
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func truncateSnippet(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes-1]) + "…"
}

func byteOffsetsForTextRange(content string, rng textRange) (int, int, error) {
	start, err := byteOffsetAtTextPosition(content, rng.StartLine, rng.StartCharacter)
	if err != nil {
		return 0, 0, err
	}
	end, err := byteOffsetAtTextPosition(content, rng.EndLine, rng.EndCharacter)
	if err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, fmt.Errorf("invalid range: end precedes start")
	}
	return start, end, nil
}

func byteOffsetAtTextPosition(content string, line, character int) (int, error) {
	if line < 1 {
		return 0, fmt.Errorf("line must be >= 1")
	}
	if character < 0 {
		return 0, fmt.Errorf("character must be >= 0")
	}
	offsets := lineOffsets(content)
	if line > len(offsets) {
		return 0, fmt.Errorf("line %d out of range for file with %d lines", line, len(offsets))
	}
	start := offsets[line-1]
	lineEnd := len(content)
	if idx := strings.IndexByte(content[start:], '\n'); idx >= 0 {
		lineEnd = start + idx
	}
	rel, err := byteOffsetAtUTF16Character(content[start:lineEnd], character)
	if err != nil {
		return 0, fmt.Errorf("line %d: %w", line, err)
	}
	return start + rel, nil
}

func byteOffsetAtUTF16Character(line string, character int) (int, error) {
	if character == 0 {
		return 0, nil
	}
	units := 0
	offset := 0
	for offset < len(line) {
		r, size := utf8.DecodeRuneInString(line[offset:])
		if r == utf8.RuneError && size == 1 {
			return 0, fmt.Errorf("invalid utf-8 content")
		}
		var width int
		if r > 0xFFFF {
			width = 2
		} else {
			width = utf16.RuneLen(r)
			if width < 0 {
				width = 1
			}
		}
		if units == character {
			return offset, nil
		}
		if units+width > character {
			return 0, fmt.Errorf("character %d splits a multibyte code point", character)
		}
		units += width
		offset += size
	}
	if units == character {
		return len(line), nil
	}
	return 0, fmt.Errorf("character %d out of range (max %d)", character, units)
}
