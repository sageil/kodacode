package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

const composerPasteTokenMinNewlines = 2

type composerPastedTextChunk struct {
	Tag   string
	Value string
}

type composerProtectedToken struct {
	start int
	end   int
	tag   string
	kind  string
}

const (
	composerProtectedTokenKindPastedText = "pasted_text"
	composerProtectedTokenKindAttachment = "attachment"
	composerProtectedTokenKindFocusPath  = "focus_path"
)

func (m *Model) handleComposerPaste(text string) tea.Cmd {
	m.resetComposerHistoryRecall()
	if attachment, ok := m.validatePastedAttachment(text); ok {
		if attachment, ok := m.appendPendingAttachment(attachment); ok {
			m.composer.InsertString(attachment.Tag)
			m.snapComposerCursorOutOfToken()
		}
		return m.refreshComposerPopup()
	}

	if strings.Count(text, "\n") < composerPasteTokenMinNewlines {
		m.composer.InsertString(text)
		m.snapComposerCursorOutOfToken()
		return m.refreshComposerPopup()
	}

	m.composerState.nextPastedTextID++
	tag := fmt.Sprintf("[Pasted text #%d +%d lines]", m.composerState.nextPastedTextID, strings.Count(text, "\n")+1)
	m.composerState.pastedText = append(m.composerState.pastedText, composerPastedTextChunk{
		Tag:   tag,
		Value: text,
	})
	m.composer.InsertString(tag)
	m.snapComposerCursorOutOfToken()
	return m.refreshComposerPopup()
}

func (m Model) expandComposerPastedText(text string) string {
	if len(m.composerState.pastedText) == 0 {
		return text
	}
	for _, chunk := range m.composerState.pastedText {
		text = strings.Replace(text, chunk.Tag, chunk.Value, 1)
	}
	return text
}

func (m *Model) clearComposerPastedText() {
	m.composerState.pastedText = nil
	m.composerState.nextPastedTextID = 0
}

func (m Model) composerProtectedTokens() []composerProtectedToken {
	if len(m.composerState.pastedText) == 0 && len(m.composerState.pendingAttachments) == 0 && len(m.composerState.pendingFocusPaths) == 0 {
		return nil
	}

	value := m.composer.Value()
	var tokens []composerProtectedToken
	for _, chunk := range m.composerState.pastedText {
		if chunk.Tag == "" {
			continue
		}
		searchStart := 0
		for {
			idx := strings.Index(value[searchStart:], chunk.Tag)
			if idx < 0 {
				break
			}
			startByte := searchStart + idx
			endByte := startByte + len(chunk.Tag)
			tokens = append(tokens, composerProtectedToken{
				start: utf8.RuneCountInString(value[:startByte]),
				end:   utf8.RuneCountInString(value[:endByte]),
				tag:   chunk.Tag,
				kind:  composerProtectedTokenKindPastedText,
			})
			searchStart = endByte
		}
	}
	for _, attachment := range m.composerState.pendingAttachments {
		if attachment.Tag == "" {
			continue
		}
		searchStart := 0
		for {
			idx := strings.Index(value[searchStart:], attachment.Tag)
			if idx < 0 {
				break
			}
			startByte := searchStart + idx
			endByte := startByte + len(attachment.Tag)
			tokens = append(tokens, composerProtectedToken{
				start: utf8.RuneCountInString(value[:startByte]),
				end:   utf8.RuneCountInString(value[:endByte]),
				tag:   attachment.Tag,
				kind:  composerProtectedTokenKindAttachment,
			})
			searchStart = endByte
		}
	}
	for _, focusPath := range m.composerState.pendingFocusPaths {
		if focusPath.Tag == "" {
			continue
		}
		searchStart := 0
		for {
			idx := strings.Index(value[searchStart:], focusPath.Tag)
			if idx < 0 {
				break
			}
			startByte := searchStart + idx
			endByte := startByte + len(focusPath.Tag)
			tokens = append(tokens, composerProtectedToken{
				start: utf8.RuneCountInString(value[:startByte]),
				end:   utf8.RuneCountInString(value[:endByte]),
				tag:   focusPath.Tag,
				kind:  composerProtectedTokenKindFocusPath,
			})
			searchStart = endByte
		}
	}
	sort.Slice(tokens, func(i, j int) bool {
		return tokens[i].start < tokens[j].start
	})
	return tokens
}

func (m Model) composerCursorOffset() int {
	lines := strings.Split(m.composer.Value(), "\n")
	line := min(max(m.composer.Line(), 0), max(len(lines)-1, 0))
	offset := 0
	for i := 0; i < line; i++ {
		offset += len([]rune(lines[i])) + 1
	}
	return offset + m.composer.Column()
}

func (m *Model) setComposerCursorOffset(target int) {
	runes := []rune(m.composer.Value())
	if target < 0 {
		target = 0
	}
	if target > len(runes) {
		target = len(runes)
	}

	lines := strings.Split(string(runes), "\n")
	lineIdx := 0
	col := target
	for i, line := range lines {
		lineRunes := len([]rune(line))
		if col <= lineRunes {
			lineIdx = i
			break
		}
		col -= lineRunes + 1
		lineIdx = i
	}

	m.composer.MoveToBegin()
	for attempts := 0; m.composer.Line() < lineIdx && attempts <= len(runes); attempts++ {
		prevLine := m.composer.Line()
		prevCol := m.composer.Column()
		m.composer.CursorDown()
		if m.composer.Line() == prevLine && m.composer.Column() == prevCol {
			break
		}
	}
	m.composer.SetCursorColumn(col)
}

func (m Model) composerTokenAtCursor() (composerProtectedToken, bool) {
	cursor := m.composerCursorOffset()
	for _, token := range m.composerProtectedTokens() {
		if cursor > token.start && cursor < token.end {
			return token, true
		}
	}
	return composerProtectedToken{}, false
}

func (m *Model) snapComposerCursorOutOfToken() {
	token, ok := m.composerTokenAtCursor()
	if !ok {
		return
	}
	cursor := m.composerCursorOffset()
	if cursor-token.start <= token.end-cursor {
		m.setComposerCursorOffset(token.start)
		return
	}
	m.setComposerCursorOffset(token.end)
}

func (m *Model) snapComposerCursorAcrossToken(direction int) {
	token, ok := m.composerTokenAtCursor()
	if !ok {
		return
	}
	if direction < 0 {
		m.setComposerCursorOffset(token.start)
		return
	}
	m.setComposerCursorOffset(token.end)
}

func (m Model) composerBackspaceTokenTarget() (composerProtectedToken, bool) {
	cursor := m.composerCursorOffset()
	for _, token := range m.composerProtectedTokens() {
		if cursor > token.start && cursor <= token.end {
			return token, true
		}
	}
	return composerProtectedToken{}, false
}

func (m *Model) removeComposerProtectedToken(token composerProtectedToken) {
	runes := []rune(m.composer.Value())
	start, end := token.start, token.end
	if start > 0 && runes[start-1] == ' ' {
		start--
	} else if end < len(runes) && runes[end] == ' ' {
		end++
	}

	m.composer.SetValue(string(append(runes[:start], runes[end:]...)))
	m.setComposerCursorOffset(start)

	if token.kind == composerProtectedTokenKindAttachment {
		_ = m.removePendingAttachmentByTag(token.tag)
		return
	}
	if token.kind == composerProtectedTokenKindFocusPath {
		_ = m.removePendingFocusPathByTag(token.tag)
		return
	}

	filtered := m.composerState.pastedText[:0]
	for _, chunk := range m.composerState.pastedText {
		if chunk.Tag == token.tag {
			continue
		}
		filtered = append(filtered, chunk)
	}
	m.composerState.pastedText = filtered
}
