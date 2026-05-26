package app

import (
	"fmt"
	"sort"
	"unicode/utf16"

	"github.com/sageil/kodacode/internal/lsp"
)

func applyCodeIntelTextEdits(content string, edits []lsp.TextEdit) (string, int, error) {
	if len(edits) == 0 {
		return content, 0, nil
	}
	type resolvedEdit struct {
		start   int
		end     int
		newText string
	}
	resolved := make([]resolvedEdit, 0, len(edits))
	for _, edit := range edits {
		start, end, err := codeIntelByteOffsetsForRange(content, edit.Range)
		if err != nil {
			return "", 0, err
		}
		resolved = append(resolved, resolvedEdit{start: start, end: end, newText: edit.NewText})
	}
	sort.Slice(resolved, func(i, j int) bool {
		if resolved[i].start == resolved[j].start {
			return resolved[i].end > resolved[j].end
		}
		return resolved[i].start > resolved[j].start
	})
	for i := 0; i < len(resolved)-1; i++ {
		if resolved[i].start < resolved[i+1].end {
			return "", 0, fmt.Errorf("overlapping text edits are not supported")
		}
	}
	current := content
	for _, edit := range resolved {
		current = current[:edit.start] + edit.newText + current[edit.end:]
	}
	return current, len(resolved), nil
}

func codeIntelByteOffsetsForRange(content string, rng lsp.Range) (int, int, error) {
	start, err := codeIntelByteOffsetAtPosition(content, rng.Start.Line+1, rng.Start.Character)
	if err != nil {
		return 0, 0, err
	}
	end, err := codeIntelByteOffsetAtPosition(content, rng.End.Line+1, rng.End.Character)
	if err != nil {
		return 0, 0, err
	}
	if end < start {
		return 0, 0, fmt.Errorf("invalid workspace edit range")
	}
	return start, end, nil
}

func codeIntelByteOffsetAtPosition(content string, line, character int) (int, error) {
	if line < 1 || character < 0 {
		return 0, fmt.Errorf("invalid text position")
	}
	currentLine := 1
	currentChar := 0
	for offset, r := range content {
		if currentLine == line && currentChar == character {
			return offset, nil
		}
		if r == '\n' {
			currentLine++
			currentChar = 0
			if currentLine > line {
				return offset, nil
			}
			continue
		}
		currentChar += codeIntelUTF16Width(r)
	}
	if currentLine == line && currentChar == character {
		return len(content), nil
	}
	if currentLine == line && currentChar < character {
		return 0, fmt.Errorf("character %d is beyond the end of line %d", character, line)
	}
	return 0, fmt.Errorf("line %d does not exist", line)
}

func codeIntelUTF16Width(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	width := utf16.RuneLen(r)
	if width < 0 {
		return 1
	}
	return width
}
