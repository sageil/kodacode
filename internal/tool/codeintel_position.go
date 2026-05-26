package tool

import (
	"fmt"
	"os"
	"strings"
)

func resolveCodeIntelCharacter(path string, line, character int, hasCharacter bool, symbol string) (int, error) {
	if hasCharacter {
		return character, nil
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" {
		return 0, ErrDefinitionCharacterInvalid
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lineText, ok := sourceLineAt(string(content), line)
	if !ok {
		return 0, fmt.Errorf("line %d is not available in %s", line, path)
	}
	index := strings.Index(lineText, symbol)
	if index < 0 {
		return 0, fmt.Errorf("symbol %q was not found on line %d in %s; read the line and retry with the exact symbol text or character offset", symbol, line, path)
	}
	if strings.Contains(lineText[index+len(symbol):], symbol) {
		return 0, fmt.Errorf("symbol %q appears multiple times on line %d in %s; retry with character", symbol, line, path)
	}
	return utf16CharacterOffset(lineText[:index]), nil
}

func sourceLineAt(content string, line int) (string, bool) {
	if line < 1 {
		return "", false
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") && len(lines) > 0 {
		lines = lines[:len(lines)-1]
	}
	if line > len(lines) {
		return "", false
	}
	return strings.TrimRight(lines[line-1], "\r"), true
}

func utf16CharacterOffset(text string) int {
	offset := 0
	for _, r := range text {
		if r > 0xFFFF {
			offset += 2
			continue
		}
		offset++
	}
	return offset
}
