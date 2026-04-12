package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// InlineQuestionPanel is a compact inline panel for user questions.
// It replaces the task panel area. Supports single-select and multi-select modes
// with ↑/↓ navigation, number shortcuts, and Enter/Esc.
//
// Result types match QuestionDialog: string (single) or []string (multi).
type InlineQuestionPanel struct {
	id      string
	prompt  string
	options []QuestionOption
	cursor  int
	mode    QuestionMode
	keys    dialogKeys
	width   int
	theme   *theme.Theme

	inputMode bool
	input     textinput.Model
}

func (p InlineQuestionPanel) Prompt() string { return p.prompt }

func NewInlineQuestionPanel(id, prompt string, options []QuestionOption, mode QuestionMode, w int, th *theme.Theme) InlineQuestionPanel {
	ti := textinput.New()
	ti.Placeholder = "Type your answer..."
	ti.SetWidth(w/2 - 10)

	return InlineQuestionPanel{
		id:      id,
		prompt:  prompt,
		options: options,
		mode:    mode,
		keys:    defaultDialogKeys(),
		width:   w,
		theme:   th,
		input:   ti,
	}
}

func (p InlineQuestionPanel) PanelHeight() int {
	h := 2 // accent border + bottom separator
	// Prompt may wrap across multiple lines.
	h += p.promptLines()
	h += len(p.options)
	if p.inputMode {
		h++ // text input line
	}
	return h
}

func (p InlineQuestionPanel) promptLines() int {
	maxW := max(p.width-6, 20) // padding for centering
	prompt := p.prompt
	if len([]rune(prompt)) <= maxW {
		return 1 // fits on one line (hint appended inline)
	}
	// Count wrapped lines.
	runes := []rune(prompt)
	lines := (len(runes) + maxW - 1) / maxW
	return lines + 1 // +1 for the hint line
}

func (p InlineQuestionPanel) Init() tea.Cmd { return nil }

func (p InlineQuestionPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if p.inputMode {
		return p.updateInputMode(msg)
	}

	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}

	// Number shortcuts: 1-9 for quick selection.
	if s := kp.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		idx := int(s[0]-'1')
		if idx < len(p.options) {
			if p.mode == QuestionMulti {
				p.options[idx].checked = !p.options[idx].checked
				p.cursor = idx
				return p, nil
			}
			// Single-select: check for "other" option.
			if p.options[idx].isOtherOption() {
				p.cursor = idx
				p.inputMode = true
				return p, p.input.Focus()
			}
			return p, closeDialog(p.id, p.options[idx].Value)
		}
	}

	switch {
	case key.Matches(kp, p.keys.Up):
		if p.cursor > 0 {
			p.cursor--
		}
	case key.Matches(kp, p.keys.Down):
		if p.cursor < len(p.options)-1 {
			p.cursor++
		}
	case key.Matches(kp, p.keys.Toggle):
		if p.mode == QuestionMulti {
			p.options[p.cursor].checked = !p.options[p.cursor].checked
		} else if len(p.options) > 0 {
			if p.options[p.cursor].isOtherOption() {
				p.inputMode = true
				return p, p.input.Focus()
			}
			return p, closeDialog(p.id, p.options[p.cursor].Value)
		}
	case key.Matches(kp, p.keys.Confirm):
		if len(p.options) == 0 {
			return p, closeDialog(p.id, nil)
		}
		if p.mode == QuestionSingle {
			if p.options[p.cursor].isOtherOption() {
				p.inputMode = true
				return p, p.input.Focus()
			}
			return p, closeDialog(p.id, p.options[p.cursor].Value)
		}
		// Multi: collect checked values.
		var chosen []string
		hasOther := false
		for _, o := range p.options {
			if o.checked {
				if o.isOtherOption() {
					hasOther = true
				} else {
					chosen = append(chosen, o.Value)
				}
			}
		}
		if hasOther {
			p.inputMode = true
			return p, p.input.Focus()
		}
		if len(chosen) == 0 {
			return p, nil
		}
		return p, closeDialog(p.id, chosen)
	case key.Matches(kp, p.keys.Cancel):
		return p, closeDialog(p.id, nil)
	}
	return p, nil
}

func (p InlineQuestionPanel) updateInputMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kp, p.keys.Cancel):
			p.inputMode = false
			return p, nil
		case key.Matches(kp, p.keys.Confirm):
			text := strings.TrimSpace(p.input.Value())
			if text == "" {
				return p, nil
			}
			if p.mode == QuestionSingle {
				return p, closeDialog(p.id, text)
			}
			var chosen []string
			for _, o := range p.options {
				if o.checked && !o.isOtherOption() {
					chosen = append(chosen, o.Value)
				}
			}
			chosen = append(chosen, text)
			return p, closeDialog(p.id, chosen)
		}
	}
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	return p, cmd
}

func (p InlineQuestionPanel) View() tea.View {
	w := p.width
	if w < 1 {
		w = 80
	}

	accentColor := colorFrom(p.theme, "secondary", lipgloss.Color("4"))
	dimColor := colorFrom(p.theme, "subtext", lipgloss.Color("241"))
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)
	promptStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	selStyle := selectedItemStyle(p.theme)
	normStyle := itemStyle(p.theme)
	chkStyle := checkedItemStyle(p.theme)

	topBorder := lipgloss.NewStyle().Foreground(accentColor).Render(strings.Repeat("▔", w))
	bottomSep := dimStyle.Render(strings.Repeat("─", w))

	var sb strings.Builder
	sb.WriteString(topBorder)

	var hint string
	if p.inputMode {
		hint = dimStyle.Render("  enter confirm · esc back")
	} else if p.mode == QuestionMulti {
		hint = dimStyle.Render("  ↑/↓ space toggle · enter confirm · esc skip")
	} else {
		hint = dimStyle.Render("  ↑/↓ enter select · esc skip")
	}
	maxPromptW := max(w-6, 20) // leave room for centering padding
	promptText := p.prompt
	promptRunes := []rune(promptText)
	if len(promptRunes) <= maxPromptW {
		promptLine := promptStyle.Render(promptText) + hint
		sb.WriteByte('\n')
		sb.WriteString(centerLine(promptLine, w))
	} else {
		for len(promptRunes) > 0 {
			end := min(maxPromptW, len(promptRunes))
			sb.WriteByte('\n')
			sb.WriteString(centerLine(promptStyle.Render(string(promptRunes[:end])), w))
			promptRunes = promptRunes[end:]
		}
		sb.WriteByte('\n')
		sb.WriteString(centerLine(hint, w))
	}

	for i, opt := range p.options {
		num := dimStyle.Render(fmt.Sprintf("%d.", i+1))
		var prefix string
		var style lipgloss.Style

		switch p.mode {
		case QuestionSingle:
			if i == p.cursor {
				prefix = "● "
				style = selStyle
			} else {
				prefix = "○ "
				style = normStyle
			}
		case QuestionMulti:
			if opt.checked {
				prefix = "◉ "
				style = chkStyle
			} else if i == p.cursor {
				prefix = "○ "
				style = selStyle
			} else {
				prefix = "○ "
				style = normStyle
			}
		}

		line := num + " " + style.Render(prefix+opt.Label)
		sb.WriteByte('\n')
		sb.WriteString(centerLine(line, w))
	}

	if p.inputMode {
		sb.WriteByte('\n')
		sb.WriteString(centerLine(p.input.View(), w))
	}

	sb.WriteByte('\n')
	sb.WriteString(bottomSep)
	return tea.NewView(sb.String())
}
