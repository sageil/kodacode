package tui

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

const (
	inputMaxHeight = 8
	inputMinHeight = 1
)

var pastedTextTagPattern = regexp.MustCompile(`\[Pasted text #\d+ \+\d+ lines\]`)

type submitMsg struct {
	text string
}

type editorOpenMsg struct {
	currentText string
}

type editorDoneMsg struct {
	text string
	err  error
}

type attachmentRemoveMsg struct {
	index int
}

type protectedToken struct {
	start int
	end   int
	kind  string
	index int
}

type Footer struct {
	input              textarea.Model
	width              int
	boxed              bool
	boxWidth           int
	theme              *theme.Theme
	streaming          bool
	errorFlash         bool
	animCol            int
	toolLoopStep       int
	compacting         bool
	streamStartTime    time.Time
	history            []string
	historyIdx         int
	historyBuf         string
	pendingAttachments []Attachment
	blocked            bool

	slashCommands []SlashCommand
	slashFiltered []SlashCommand
	slashActive   bool
	slashCursor   int

	historySearch       bool
	historySearchQuery  string
	historySearchResult string

	expanded     bool
	expandedMax  int
	pastedChunks []string
}

func (f *Footer) ApplyTheme(t *theme.Theme) {
	f.theme = t

	text := colorFrom(t, "text", lipgloss.Color("7"))
	subtext := colorFrom(t, "subtext", lipgloss.Color("241"))

	styles := f.input.Styles()

	styles.Focused.Text = lipgloss.NewStyle().Foreground(text)
	styles.Focused.Placeholder = lipgloss.NewStyle().Foreground(subtext)
	styles.Focused.Prompt = lipgloss.NewStyle().Foreground(subtext)
	styles.Focused.EndOfBuffer = lipgloss.NewStyle().Foreground(subtext)
	styles.Focused.CursorLine = lipgloss.NewStyle()

	styles.Blurred.Text = lipgloss.NewStyle().Foreground(subtext)
	styles.Blurred.Placeholder = lipgloss.NewStyle().Foreground(subtext)
	styles.Blurred.Prompt = lipgloss.NewStyle().Foreground(subtext)
	styles.Blurred.EndOfBuffer = lipgloss.NewStyle().Foreground(subtext)
	styles.Blurred.CursorLine = lipgloss.NewStyle()

	f.input.SetStyles(styles)
}

func (f *Footer) HandlePaste(text string) {
	lines := strings.Count(text, "\n")
	if lines < 2 {
		f.input.InsertString(text)
		f.snapCursorOutOfToken()
		f.syncInputHeight()
		return
	}
	idx := len(f.pastedChunks) + 1
	f.pastedChunks = append(f.pastedChunks, text)
	tag := fmt.Sprintf("[Pasted text #%d +%d lines]", idx, lines+1)
	f.input.InsertString(tag)
	f.snapCursorOutOfToken()
	f.syncInputHeight()
}

func (f *Footer) ResolvePastedText(text string) string {
	if len(f.pastedChunks) == 0 {
		return text
	}
	for i, chunk := range f.pastedChunks {
		tag := fmt.Sprintf("[Pasted text #%d +%d lines]", i+1, strings.Count(chunk, "\n")+1)
		text = strings.Replace(text, tag, chunk, 1)
	}
	f.pastedChunks = nil
	return text
}

func NewFooter() Footer {
	ta := textarea.New()
	ta.Placeholder = "Ask anything... (⇧↵ newline · ^E expand · ^O editor · ^R history)"
	ta.ShowLineNumbers = false
	ta.Prompt = "  "
	ta.SetHeight(3)

	km := textarea.DefaultKeyMap()
	km.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter"),
		key.WithHelp("shift+enter", "new line"),
	)
	ta.KeyMap = km

	styles := ta.Styles()
	styles.Focused.CursorLine = lipgloss.NewStyle()
	styles.Blurred.CursorLine = lipgloss.NewStyle()
	ta.SetStyles(styles)

	_ = ta.Focus()
	footer := Footer{input: ta}
	footer.updatePrompt()
	return footer
}

func (f *Footer) SetSize(w int) {
	f.boxed = false
	f.width = w
	f.updatePrompt()
	f.input.SetWidth(w - 2)
	f.syncInputHeight()
}

func (f *Footer) SetBoxed(innerWidth int) {
	f.boxed = true
	f.boxWidth = innerWidth
	f.updatePrompt()
	f.input.SetWidth(innerWidth - 4)
	f.syncInputHeight()
}

func (f *Footer) syncInputHeight() {
	maxH := inputMaxHeight
	if f.expanded && f.expandedMax > 0 {
		maxH = f.expandedMax / 2
		if maxH < inputMaxHeight {
			maxH = inputMaxHeight
		}
	}
	h := max(min(f.visualLineCount(), maxH), 3)
	if f.expanded && h < maxH {
		h = maxH
	}
	if h != f.input.Height() {
		f.input.SetHeight(h)
	}
}

func (f *Footer) visualLineCount() int {
	w := f.input.Width()
	if w <= 0 {
		w = 80
	}
	total := 0
	for line := range strings.SplitSeq(f.input.Value(), "\n") {
		rw := lipgloss.Width(line)
		if rw <= w {
			total++
		} else {
			total += (rw + w - 1) / w
		}
	}
	if total == 0 {
		total = 1
	}
	return total
}

func (f *Footer) SetExpandMax(h int) { f.expandedMax = h }
func (f *Footer) IsExpanded() bool   { return f.expanded }
func (f *Footer) SetBlocked(b bool)  { f.blocked = b }

func (f *Footer) SetPendingAttachments(atts []Attachment) {
	f.pendingAttachments = atts
	f.updatePrompt()
	f.syncInputHeight()
}

func (f *Footer) SetStreaming(streaming bool) {
	f.streaming = streaming
	if streaming {
		if f.streamStartTime.IsZero() {
			f.streamStartTime = time.Now()
		}
	} else {
		f.animCol = 0
		f.toolLoopStep = 0
		f.compacting = false
		f.streamStartTime = time.Time{}
	}
}

func (f *Footer) SetCompacting(b bool)  { f.compacting = b }
func (f *Footer) SetToolLoopStep(n int) { f.toolLoopStep = n }

func (f *Footer) SetErrorFlash(on bool) { f.errorFlash = on }

func (f *Footer) AdvanceAnim() {
	if f.streaming {
		f.animCol = (f.animCol + 1) % max(f.width, 1)
	}
}

func (f *Footer) Reset() { f.input.Reset() }

func (f *Footer) Focus() tea.Cmd { return f.input.Focus() }

func (f Footer) attachmentPrompt() string {
	if len(f.pendingAttachments) == 0 {
		return "  "
	}
	labels := make([]string, len(f.pendingAttachments))
	for i, att := range f.pendingAttachments {
		labels[i] = formatAttachmentLabel(i+1, att)
	}
	return "  " + strings.Join(labels, " ") + " "
}

func (f *Footer) updatePrompt() {
	prompt := f.attachmentPrompt()
	promptWidth := lipgloss.Width(prompt)
	f.input.SetPromptFunc(promptWidth, func(info textarea.PromptInfo) string {
		if info.LineNumber == 0 {
			return prompt
		}
		return strings.Repeat(" ", promptWidth)
	})
}

func (f Footer) protectedTokens() []protectedToken {
	var tokens []protectedToken
	value := f.input.Value()
	for _, loc := range pastedTextTagPattern.FindAllStringIndex(value, -1) {
		tokens = append(tokens, protectedToken{
			start: utf8.RuneCountInString(value[:loc[0]]),
			end:   utf8.RuneCountInString(value[:loc[1]]),
			kind:  "pasted",
			index: -1,
		})
	}
	return tokens
}

func (f Footer) cursorOffset() int {
	lines := strings.Split(f.input.Value(), "\n")
	line := min(max(f.input.Line(), 0), max(len(lines)-1, 0))
	offset := 0
	for i := 0; i < line; i++ {
		offset += len([]rune(lines[i])) + 1
	}
	return offset + f.input.LineInfo().CharOffset
}

func (f *Footer) setCursorOffset(target int) {
	runes := []rune(f.input.Value())
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
	f.input.MoveToBegin()
	for i := 0; i < lineIdx; i++ {
		f.input.CursorDown()
	}
	f.input.SetCursorColumn(col)
}

func (f Footer) tokenAtCursor() (protectedToken, bool) {
	cursor := f.cursorOffset()
	for _, tok := range f.protectedTokens() {
		if cursor > tok.start && cursor < tok.end {
			return tok, true
		}
	}
	return protectedToken{}, false
}

func (f *Footer) snapCursorOutOfToken() {
	tok, ok := f.tokenAtCursor()
	if !ok {
		return
	}
	cursor := f.cursorOffset()
	if cursor-tok.start <= tok.end-cursor {
		f.setCursorOffset(tok.start)
		return
	}
	f.setCursorOffset(tok.end)
}

func (f Footer) backspaceTokenTarget() (protectedToken, bool) {
	cursor := f.cursorOffset()
	for _, tok := range f.protectedTokens() {
		if cursor > tok.start && cursor <= tok.end {
			return tok, true
		}
	}
	return protectedToken{}, false
}

func (f *Footer) removeProtectedToken(tok protectedToken) {
	runes := []rune(f.input.Value())
	start, end := tok.start, tok.end
	if start > 0 && runes[start-1] == ' ' {
		start--
	} else if end < len(runes) && runes[end] == ' ' {
		end++
	}
	f.input.SetValue(string(append(runes[:start], runes[end:]...)))
	f.setCursorOffset(start)
	f.syncInputHeight()
}

func (f Footer) Height() int {
	h := f.fullWidthHeight()
	if f.boxed {
		h = f.boxedHeight()
	}
	return h + f.SlashCompletionHeight()
}

func (f Footer) fullWidthHeight() int {
	lines := max(min(f.input.Height(), inputMaxHeight), inputMinHeight)
	h := 1 + lines
	if f.streaming {
		h++
	}
	if f.historySearch {
		h++
	}
	return h
}

func (f Footer) boxedHeight() int {
	lines := max(min(f.input.Height(), inputMaxHeight), inputMinHeight)
	h := 2 + lines
	return h
}

func (f Footer) Update(msg tea.Msg) (Footer, tea.Cmd) {
	if pm, ok := msg.(tea.PasteMsg); ok {
		f.HandlePaste(pm.String())
		return f, nil
	}
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		if f.historySearch {
			switch kp.String() {
			case "esc":
				f.historySearch = false
				f.historySearchQuery = ""
				f.historySearchResult = ""
				return f, nil
			case "enter":
				result := f.historySearchResult
				f.historySearch = false
				f.historySearchQuery = ""
				f.historySearchResult = ""
				if result != "" {
					f.input.Reset()
					f.input.SetValue(result)
					f.syncInputHeight()
				}
				return f, nil
			case "backspace":
				if len(f.historySearchQuery) > 0 {
					f.historySearchQuery = f.historySearchQuery[:len(f.historySearchQuery)-1]
					f.historySearchResult = searchHistory(f.history, f.historySearchQuery)
				}
				return f, nil
			default:
				if len(kp.String()) == 1 {
					f.historySearchQuery += kp.String()
					f.historySearchResult = searchHistory(f.history, f.historySearchQuery)
				}
				return f, nil
			}
		}

		if kp.String() == "ctrl+r" && len(f.history) > 0 && !f.streaming {
			f.historySearch = true
			f.historySearchQuery = ""
			f.historySearchResult = ""
			return f, nil
		}

		if kp.String() == "ctrl+e" && !f.streaming {
			f.expanded = !f.expanded
			f.syncInputHeight()
			return f, nil
		}

		if kp.String() == "ctrl+o" && !f.streaming {
			return f, func() tea.Msg {
				return editorOpenMsg{currentText: f.input.Value()}
			}
		}

		if f.slashActive {
			switch kp.String() {
			case "esc":
				f.slashActive = false
				f.input.Reset()
				f.syncInputHeight()
				return f, nil
			case "up":
				if f.slashCursor > 0 {
					f.slashCursor--
				}
				return f, nil
			case "down":
				if f.slashCursor < len(f.slashFiltered)-1 {
					f.slashCursor++
				}
				return f, nil
			case "enter", "tab":
				if len(f.slashFiltered) > 0 {
					selected := f.slashFiltered[f.slashCursor]
					f.input.Reset()
					f.input.SetValue(selected.Name + " ")
					f.slashActive = false
					f.syncInputHeight()
					switch selected.Name {
					case "/agents", "/models", "/sessions", "/connect", "/new", "/help", "/config", "/theme", "/exit", "/diff", "/undo", "/cost", "/export", "/pins", "/reload":
						text := strings.TrimSpace(selected.Name)
						f.input.Reset()
						f.syncInputHeight()
						return f, func() tea.Msg { return submitMsg{text: text} }
					}
				}
				return f, nil
			case "backspace":
				var cmd tea.Cmd
				f.input, cmd = f.input.Update(msg)
				f.snapCursorOutOfToken()
				f.syncInputHeight()
				f.filterSlashCommands()
				return f, cmd
			default:
				var cmd tea.Cmd
				f.input, cmd = f.input.Update(msg)
				f.snapCursorOutOfToken()
				f.syncInputHeight()
				f.filterSlashCommands()
				return f, cmd
			}
		}

		if kp.Key().Code == tea.KeyBackspace {
			if tok, ok := f.backspaceTokenTarget(); ok {
				if tok.kind == "attachment" {
					return f, func() tea.Msg { return attachmentRemoveMsg{index: tok.index} }
				}
				f.removeProtectedToken(tok)
				return f, nil
			}
			if f.cursorOffset() == 0 && len(f.pendingAttachments) > 0 {
				return f, func() tea.Msg { return attachmentRemoveMsg{index: len(f.pendingAttachments) - 1} }
			}
		}

		switch kp.String() {
		case "enter":
			if f.streaming {
				return f, nil
			}
			text := strings.TrimSpace(f.input.Value())
			text = f.ResolvePastedText(text)
			f.input.Reset()
			f.slashActive = false
			f.syncInputHeight()
			return f, func() tea.Msg { return submitMsg{text: text} }

		case "up":
			if len(f.history) > 0 && strings.Count(f.input.Value(), "\n") == 0 {
				if f.historyIdx > 0 {
					if f.historyIdx == len(f.history) {
						f.historyBuf = f.input.Value()
					}
					f.historyIdx--
					f.input.Reset()
					f.input.SetValue(f.history[f.historyIdx])
					f.syncInputHeight()
					return f, nil
				}
				return f, nil
			}

		case "down":
			if len(f.history) > 0 && strings.Count(f.input.Value(), "\n") == 0 && f.historyIdx < len(f.history) {
				f.historyIdx++
				f.input.Reset()
				if f.historyIdx == len(f.history) {
					f.input.SetValue(f.historyBuf)
				} else {
					f.input.SetValue(f.history[f.historyIdx])
				}
				f.syncInputHeight()
				return f, nil
			}
		}
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	f.snapCursorOutOfToken()
	f.syncInputHeight()
	val := f.input.Value()
	if strings.HasPrefix(val, "/") && !strings.Contains(val[1:], " ") && len(f.slashCommands) > 0 {
		f.filterSlashCommands()
	} else if !strings.HasPrefix(val, "/") {
		f.slashActive = false
	}
	return f, cmd
}

func (f Footer) View() string {
	if f.boxed {
		return f.viewBoxed()
	}
	return f.viewFullWidth()
}
