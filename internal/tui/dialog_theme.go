package tui

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type ThemeItem struct {
	Name        string // e.g. "rose-pine-moon" or "default" for system palette
	DisplayName string
}

// ThemePickerDialog lets the user choose a theme from a list.
//
// Result type on confirmation: ThemeItem.
// Result is nil on cancellation.
type ThemePickerDialog struct {
	id       string
	title    string
	items    []ThemeItem
	filtered []ThemeItem
	cursor   int
	filter   textinput.Model
	keys     dialogKeys
	width    int
	theme    *theme.Theme
}

func (d *ThemePickerDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

// NewThemePickerDialog creates a new theme-picker dialog.
// id is a caller-supplied identifier echoed back in dialogClosedMsg.
// current is the bare name of the active theme ("" or "default" = system palette);
// the cursor is pre-positioned on the matching item when found.
func NewThemePickerDialog(id string, items []ThemeItem, th *theme.Theme, current string) ThemePickerDialog {
	fi := textinput.New()
	fi.Placeholder = "filter…"
	fi.CharLimit = 64
	fi.Focus()

	// Pre-position cursor on the currently active theme.
	cursor := 0
	for i, item := range items {
		if item.Name == current {
			cursor = i
			break
		}
	}

	return ThemePickerDialog{
		id:       id,
		title:    "Select Theme",
		items:    items,
		filtered: items,
		cursor:   cursor,
		filter:   fi,
		keys:     filterDialogKeys(),
		width:    50,
		theme:    th,
	}
}

func (d *ThemePickerDialog) SetWidth(w int) { d.width = w }
func (d ThemePickerDialog) Width() int               { return d.width }

func (d ThemePickerDialog) Init() tea.Cmd { return nil }

func fuzzyMatchTheme(item ThemeItem, query string) bool {
	if query == "" {
		return true
	}
	ok, _ := fuzzyScore(query, item.DisplayName)
	return ok
}

func (d *ThemePickerDialog) refilterThemes() {
	q := d.filter.Value()
	if q == "" {
		d.filtered = d.items
		return
	}
	d.filtered = nil
	for _, item := range d.items {
		if fuzzyMatchTheme(item, q) {
			d.filtered = append(d.filtered, item)
		}
	}
}

func (d ThemePickerDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, isKey := msg.(tea.KeyPressMsg)
	if isKey {
		switch {
		case key.Matches(kp, d.keys.Up):
			if d.cursor > 0 {
				d.cursor--
			}
			return d, nil
		case key.Matches(kp, d.keys.Down):
			if d.cursor < len(d.filtered)-1 {
				d.cursor++
			}
			return d, nil
		case key.Matches(kp, d.keys.Select):
			if len(d.filtered) > 0 {
				return d, closeDialog(d.id, d.filtered[d.cursor])
			}
			return d, nil
		case key.Matches(kp, d.keys.Cancel):
			return d, closeDialog(d.id, nil)
		}
	}

	// Forward all other messages (including character input, backspace, etc.)
	// to the filter textinput.
	prev := d.filter.Value()
	var cmd tea.Cmd
	d.filter, cmd = d.filter.Update(msg)
	if d.filter.Value() != prev {
		d.refilterThemes()
		// Clamp cursor to new list length.
		if d.cursor >= len(d.filtered) {
			if len(d.filtered) > 0 {
				d.cursor = len(d.filtered) - 1
			} else {
				d.cursor = 0
			}
		}
	}
	return d, cmd
}

func (d ThemePickerDialog) View() tea.View {
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	title := titleStyle(d.theme).Render(d.title)
	hint := hintStyle(d.theme).Render("type to filter • ↑/↓ navigate • enter select • esc cancel")

	var rows []string
	for i, item := range d.filtered {
		name := truncate(item.DisplayName, 44)
		if i == d.cursor {
			rows = append(rows, sel.Render("> "+name))
		} else {
			rows = append(rows, norm.Render("  "+name))
		}
	}
	if len(rows) == 0 {
		rows = append(rows, norm.Render("  no matches"))
	}

	body := title + "\n\n" + d.filter.View() + "\n\n" +
		strings.Join(rows, "\n") + "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(box)
}
