package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type QuestionMode int

const (
	QuestionSingle QuestionMode = iota
	QuestionMulti
)

type QuestionOption struct {
	Value   string
	Label   string
	checked bool
}

func (o QuestionOption) isOtherOption() bool {
	lower := strings.ToLower(o.Label)
	return lower == "other" || lower == "something else" ||
		strings.HasPrefix(lower, "other ") || strings.HasPrefix(lower, "something else")
}

// QuestionDialog is a modal asking the user to pick one or more options.
//
// Result type on confirmation:
//   - QuestionSingle → string (the selected Value)
//   - QuestionMulti  → []string (all checked Values)
//
// Result is nil on cancellation.
type QuestionDialog struct {
	id      string
	title   string
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

func (d *QuestionDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

// NewQuestionDialog creates a QuestionDialog. The width auto-sizes to fit the
// longest option label, the prompt, and the title, with a minimum of 40 and
// a maximum of 100 columns.
func NewQuestionDialog(id, title, prompt string, options []QuestionOption, mode QuestionMode, th *theme.Theme) QuestionDialog {
	// Compute width from content. Account for border (2) + padding (4) + prefix (4).
	const pad = 10 // border + padding + prefix chars
	const minWidth, maxWidth = 40, 100

	w := max(len(prompt), len(title))
	for _, o := range options {
		if len(o.Label)+4 > w { // +4 for "> " or "[x] " prefix
			w = len(o.Label) + 4
		}
	}
	w += pad
	w = max(w, minWidth)
	w = min(w, maxWidth)

	ti := textinput.New()
	ti.Placeholder = "Type your answer..."
	ti.SetWidth(w - pad)

	return QuestionDialog{
		id:      id,
		title:   title,
		prompt:  prompt,
		options: options,
		mode:    mode,
		keys:    defaultDialogKeys(),
		width:   w,
		theme:   th,
		input:   ti,
	}
}

func (d *QuestionDialog) SetWidth(w int) { d.width = w }
func (d QuestionDialog) Width() int               { return d.width }

func (d QuestionDialog) Init() tea.Cmd { return nil }

func (d QuestionDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// In free-text input mode, forward to the text input.
	if d.inputMode {
		return d.updateInputMode(msg)
	}

	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch {
	case key.Matches(kp, d.keys.Up):
		if d.cursor > 0 {
			d.cursor--
		}
	case key.Matches(kp, d.keys.Down):
		if d.cursor < len(d.options)-1 {
			d.cursor++
		}
	case key.Matches(kp, d.keys.Toggle):
		if d.mode == QuestionMulti {
			d.options[d.cursor].checked = !d.options[d.cursor].checked
		} else if len(d.options) > 0 {
			// Single-select: if this is an "other" option, switch to input mode.
			if d.options[d.cursor].isOtherOption() {
				d.inputMode = true
				return d, d.input.Focus()
			}
			return d, closeDialog(d.id, d.options[d.cursor].Value)
		}
	case key.Matches(kp, d.keys.Confirm):
		if len(d.options) == 0 {
			return d, closeDialog(d.id, nil)
		}
		if d.mode == QuestionSingle {
			// If this is an "other" option, switch to input mode.
			if d.options[d.cursor].isOtherOption() {
				d.inputMode = true
				return d, d.input.Focus()
			}
			return d, closeDialog(d.id, d.options[d.cursor].Value)
		}
		// Multi: collect all checked values.
		var chosen []string
		hasOther := false
		for _, o := range d.options {
			if o.checked {
				if o.isOtherOption() {
					hasOther = true
				} else {
					chosen = append(chosen, o.Value)
				}
			}
		}
		// If "other" is checked, switch to input mode to get the text.
		if hasOther {
			d.inputMode = true
			return d, d.input.Focus()
		}
		if len(chosen) == 0 {
			return d, nil // no-op: require at least one selection
		}
		return d, closeDialog(d.id, chosen)
	case key.Matches(kp, d.keys.Cancel):
		return d, closeDialog(d.id, nil)
	}
	return d, nil
}

func (d QuestionDialog) updateInputMode(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kp, ok := msg.(tea.KeyPressMsg); ok {
		switch {
		case key.Matches(kp, d.keys.Cancel):
			// Esc goes back to option selection, not full cancel.
			d.inputMode = false
			return d, nil
		case key.Matches(kp, d.keys.Confirm):
			text := strings.TrimSpace(d.input.Value())
			if text == "" {
				return d, nil // require non-empty input
			}
			if d.mode == QuestionSingle {
				return d, closeDialog(d.id, text)
			}
			// Multi: collect checked non-other values + the typed text.
			var chosen []string
			for _, o := range d.options {
				if o.checked && !o.isOtherOption() {
					chosen = append(chosen, o.Value)
				}
			}
			chosen = append(chosen, text)
			return d, closeDialog(d.id, chosen)
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return d, cmd
}

func (d QuestionDialog) View() tea.View {
	title := titleStyle(d.theme).Render(d.title)
	prompt := itemStyle(d.theme).Render(d.prompt)

	var hint string
	if d.inputMode {
		hint = hintStyle(d.theme).Render("enter confirm • esc back")
	} else {
		switch d.mode {
		case QuestionSingle:
			hint = hintStyle(d.theme).Render("↑/↓ navigate • enter select • esc cancel")
		case QuestionMulti:
			hint = hintStyle(d.theme).Render("↑/↓ navigate • space toggle • enter confirm • esc cancel")
		}
	}

	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	chk := checkedItemStyle(d.theme)

	var rows []string
	for i, opt := range d.options {
		var prefix string
		switch d.mode {
		case QuestionSingle:
			if i == d.cursor {
				prefix = "● "
			} else {
				prefix = "○ "
			}
		case QuestionMulti:
			if opt.checked {
				prefix = "◉ "
			} else {
				prefix = "○ "
			}
		}

		var style lipgloss.Style
		switch {
		case d.mode == QuestionMulti && opt.checked:
			style = chk
		case i == d.cursor:
			style = sel
		default:
			style = norm
		}
		rows = append(rows, style.Render(prefix+opt.Label))
	}

	var body string
	if d.prompt != "" {
		body = title + "\n\n" + prompt + "\n\n" + strings.Join(rows, "\n")
	} else {
		body = title + "\n\n" + strings.Join(rows, "\n")
	}

	// Show text input below options when in input mode.
	if d.inputMode {
		body += "\n\n" + d.input.View()
	}

	body += "\n\n" + hint

	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(dropShadow(box, d.theme))
}
