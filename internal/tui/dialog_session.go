package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type SessionItem struct {
	ID        string
	Title     string
	AgentID   string
	CreatedAt time.Time
}

type sessionDialogMode int

const (
	sessionModeList sessionDialogMode = iota
	sessionModeCreate
	sessionModeConfirmDelete
	sessionModePurge
)

type SessionDialogResult struct {
	New     bool
	Title   string
	Session SessionItem

	Delete   bool     // single session delete
	PurgeIDs []string // bulk purge: session IDs to delete
}

type purgeOption struct {
	label    string
	duration time.Duration // 0 = delete all
	count    int           // computed: how many will be deleted
}

const sessionListMaxVisible = 12

type SessionDialog struct {
	id          string
	sessions    []SessionItem
	filtered    []SessionItem
	cursor      int
	offset      int
	mode        sessionDialogMode
	input       textinput.Model
	filterInput textinput.Model
	keys        dialogKeys
	width       int
	theme       *theme.Theme

	purgeCursor  int
	purgeOptions []purgeOption
}

func sessionLabel(s SessionItem) string {
	if strings.TrimSpace(s.Title) == "" {
		return "Untitled"
	}
	return s.Title
}

func (d *SessionDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

func NewSessionDialog(id string, sessions []SessionItem, th *theme.Theme) SessionDialog {
	ti := textinput.New()
	ti.Placeholder = "Session title…"

	fi := textinput.New()
	fi.Placeholder = "filter…"
	fi.CharLimit = 64
	fi.Focus()

	// Compute width from the longest session title.
	// dialogStyle uses Padding(1,3) + RoundedBorder so chrome = 3+3+1+1 = 8.
	const chrome = 8
	const minInner = 54 // wide enough for the hint line
	const maxInner = 72
	inner := minInner
	for _, s := range sessions {
		label := sessionLabel(s)
		age := relativeTime(s.CreatedAt)
		// prefix(2) + title + gap(2) + age
		needed := 4 + len([]rune(label)) + len(age)
		if needed > inner {
			inner = needed
		}
	}
	if inner > maxInner {
		inner = maxInner
	}
	w := inner + chrome

	return SessionDialog{
		id:          id,
		sessions:    sessions,
		filtered:    sessions,
		input:       ti,
		filterInput: fi,
		keys:        filterDialogKeys(),
		width:       w,
		theme:       th,
	}
}

func (d *SessionDialog) SetWidth(w int) { d.width = w }
func (d SessionDialog) Width() int      { return d.width }

func (d SessionDialog) Init() tea.Cmd { return nil }

func (d SessionDialog) totalItems() int { return len(d.filtered) + 1 }

func fuzzyMatchSession(s SessionItem, query string) bool {
	if query == "" {
		return true
	}
	label := sessionLabel(s)
	ok, _ := fuzzyScore(query, label)
	return ok
}

func (d *SessionDialog) refilterSessions() {
	q := d.filterInput.Value()
	if q == "" {
		d.filtered = d.sessions
		return
	}
	d.filtered = nil
	for _, s := range d.sessions {
		if fuzzyMatchSession(s, q) {
			d.filtered = append(d.filtered, s)
		}
	}
}

func (d *SessionDialog) ensureVisible() {
	if d.cursor < d.offset {
		d.offset = d.cursor
	}
	if d.cursor >= d.offset+sessionListMaxVisible {
		d.offset = d.cursor - sessionListMaxVisible + 1
	}
}

func (d SessionDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch d.mode {
	case sessionModeCreate:
		return d.updateCreate(msg)
	case sessionModeConfirmDelete:
		return d.updateConfirmDelete(msg)
	case sessionModePurge:
		return d.updatePurge(msg)
	default:
		return d.updateList(msg)
	}
}

func (d SessionDialog) updateList(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, isKey := msg.(tea.KeyPressMsg)
	if isKey {
		switch {
		case key.Matches(kp, d.keys.Up):
			if d.cursor > 0 {
				d.cursor--
				d.ensureVisible()
			}
			return d, nil
		case key.Matches(kp, d.keys.Down):
			if d.cursor < d.totalItems()-1 {
				d.cursor++
				d.ensureVisible()
			}
			return d, nil
		case key.Matches(kp, d.keys.Select):
			if d.cursor == len(d.filtered) {
				d.mode = sessionModeCreate
				return d, d.input.Focus()
			}
			result := SessionDialogResult{Session: d.filtered[d.cursor]}
			return d, closeDialog(d.id, result)
		case key.Matches(kp, d.keys.Cancel):
			return d, closeDialog(d.id, nil)
		case kp.String() == "ctrl+d":
			if d.cursor < len(d.filtered) {
				d.mode = sessionModeConfirmDelete
				return d, nil
			}
		case kp.String() == "ctrl+p":
			d.buildPurgeOptions()
			d.purgeCursor = 0
			d.mode = sessionModePurge
			return d, nil
		}
	}

	prev := d.filterInput.Value()
	var cmd tea.Cmd
	d.filterInput, cmd = d.filterInput.Update(msg)
	if d.filterInput.Value() != prev {
		d.refilterSessions()
		d.cursor = 0
		d.offset = 0
	}
	return d, cmd
}

func (d SessionDialog) updateCreate(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if ok {
		switch {
		case key.Matches(kp, d.keys.Confirm):
			title := strings.TrimSpace(d.input.Value())
			if title == "" {
				title = "New session"
			}
			result := SessionDialogResult{New: true, Title: title}
			return d, closeDialog(d.id, result)
		case key.Matches(kp, d.keys.Cancel):
			d.mode = sessionModeList
			d.input.SetValue("")
			d.filterInput.Focus()
			return d, nil
		}
	}
	var cmd tea.Cmd
	d.input, cmd = d.input.Update(msg)
	return d, cmd
}

func (d SessionDialog) updateConfirmDelete(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch kp.String() {
	case "y", "enter":
		sess := d.filtered[d.cursor]
		result := SessionDialogResult{Delete: true, Session: sess}
		return d, closeDialog(d.id, result)
	case "n", "esc":
		d.mode = sessionModeList
		return d, nil
	}
	return d, nil
}

func (d *SessionDialog) buildPurgeOptions() {
	now := time.Now()
	cutoffs := []struct {
		label    string
		duration time.Duration
	}{
		{"Keep last week", 7 * 24 * time.Hour},
		{"Keep last month", 30 * 24 * time.Hour},
		{"Keep last 6 months", 180 * 24 * time.Hour},
		{"Delete all", 0},
	}

	d.purgeOptions = nil
	for _, c := range cutoffs {
		count := 0
		for _, s := range d.sessions {
			if c.duration == 0 || (!s.CreatedAt.IsZero() && now.Sub(s.CreatedAt) > c.duration) {
				count++
			}
		}
		d.purgeOptions = append(d.purgeOptions, purgeOption{
			label:    c.label,
			duration: c.duration,
			count:    count,
		})
	}
}

func (d SessionDialog) purgeIDsForOption(opt purgeOption) []string {
	now := time.Now()
	var ids []string
	for _, s := range d.sessions {
		if opt.duration == 0 || (!s.CreatedAt.IsZero() && now.Sub(s.CreatedAt) > opt.duration) {
			ids = append(ids, s.ID)
		}
	}
	return ids
}

func (d SessionDialog) updatePurge(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch {
	case key.Matches(kp, d.keys.Up):
		if d.purgeCursor > 0 {
			d.purgeCursor--
		}
	case key.Matches(kp, d.keys.Down):
		if d.purgeCursor < len(d.purgeOptions)-1 {
			d.purgeCursor++
		}
	case key.Matches(kp, d.keys.Select):
		opt := d.purgeOptions[d.purgeCursor]
		if opt.count == 0 {
			d.mode = sessionModeList
			return d, nil
		}
		ids := d.purgeIDsForOption(opt)
		result := SessionDialogResult{PurgeIDs: ids}
		return d, closeDialog(d.id, result)
	case key.Matches(kp, d.keys.Cancel):
		d.mode = sessionModeList
		return d, nil
	}
	return d, nil
}

func (d SessionDialog) View() tea.View {
	switch d.mode {
	case sessionModeCreate:
		return d.viewCreate()
	case sessionModeConfirmDelete:
		return d.viewConfirmDelete()
	case sessionModePurge:
		return d.viewPurge()
	default:
		return d.viewList()
	}
}

func (d SessionDialog) viewList() tea.View {
	total := d.totalItems()
	counter := hintStyle(d.theme).Render(fmt.Sprintf("%d / %d", d.cursor+1, total))
	title := titleStyle(d.theme).Render("Sessions") + "  " + counter
	hint := hintStyle(d.theme).Render("↑/↓ nav • enter open • ^D delete • ^P purge • esc cancel")
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	dim := hintStyle(d.theme)

	// Inner width available for row content (dialog width minus padding+border).
	innerW := d.width - 8 // padding 3+3 + border 1+1

	// Find the widest age string so we can right-align them all.
	ageWidth := 0
	for _, s := range d.filtered {
		if w := len(relativeTime(s.CreatedAt)); w > ageWidth {
			ageWidth = w
		}
	}

	type listItem struct {
		line     string // fully padded plain-text line
		selected bool
	}
	var items []listItem
	for i, s := range d.filtered {
		age := relativeTime(s.CreatedAt)
		prefix := "  "
		if i == d.cursor {
			prefix = "> "
		}
		// Reserve: prefix(2) + gap(2) + age column.
		maxTitle := innerW - 4 - ageWidth
		label := truncate(sessionLabel(s), maxTitle)
		pad := innerW - 2 - len([]rune(label)) - len(age)
		if pad < 1 {
			pad = 1
		}
		line := prefix + label + strings.Repeat(" ", pad) + age
		items = append(items, listItem{line: line, selected: i == d.cursor})
	}
	newPrefix := "  "
	if d.cursor == len(d.filtered) {
		newPrefix = "> "
	}
	items = append(items, listItem{line: newPrefix + "+ New session", selected: d.cursor == len(d.filtered)})

	// Windowed rendering.
	end := min(d.offset+sessionListMaxVisible, len(items))
	var rows []string
	if d.offset > 0 {
		rows = append(rows, dim.Render("  ↑ more"))
	}
	for i := d.offset; i < end; i++ {
		it := items[i]
		if it.selected {
			rows = append(rows, sel.Render(it.line))
		} else {
			rows = append(rows, norm.Render(it.line))
		}
	}
	if end < len(items) {
		rows = append(rows, dim.Render("  ↓ more"))
	}

	body := title + "\n\n" + d.filterInput.View() + "\n\n" +
		strings.Join(rows, "\n") + "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(box)
}

func (d SessionDialog) viewCreate() tea.View {
	title := titleStyle(d.theme).Render("New Session")
	hint := hintStyle(d.theme).Render("enter confirm • esc back")
	body := title + "\n\n" + d.input.View() + "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(box)
}

func (d SessionDialog) viewConfirmDelete() tea.View {
	sess := d.filtered[d.cursor]
	label := sessionLabel(sess)
	title := titleStyle(d.theme).Render("Delete Session")
	warn := dangerTextStyle(d.theme).Render(fmt.Sprintf("Delete \"%s\"? This cannot be undone.", truncate(label, d.width-20)))
	hint := hintStyle(d.theme).Render("y/enter confirm • n/esc cancel")
	body := title + "\n\n" + warn + "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(box)
}

func (d SessionDialog) viewPurge() tea.View {
	title := titleStyle(d.theme).Render("Purge Sessions")
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)
	danger := dangerTextStyle(d.theme)

	var rows []string
	for i, opt := range d.purgeOptions {
		label := opt.label
		countStr := danger.Render(fmt.Sprintf("-%d", opt.count))
		if opt.count == 0 {
			countStr = hintStyle(d.theme).Render("none")
		}
		line := fmt.Sprintf("%-24s %s", label, countStr)
		if i == d.purgeCursor {
			rows = append(rows, sel.Render("> "+line))
		} else {
			rows = append(rows, norm.Render("  "+line))
		}
	}

	selected := d.purgeOptions[d.purgeCursor]
	var warn string
	if selected.count > 0 {
		warn = danger.Render(fmt.Sprintf("⚠ %d session(s) will be permanently deleted", selected.count))
	}

	hint := hintStyle(d.theme).Render("↑/↓ nav • enter confirm • esc back")
	body := title + "\n\n" + strings.Join(rows, "\n")
	if warn != "" {
		body += "\n\n" + warn
	}
	body += "\n\n" + hint
	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(box)
}

func dangerTextStyle(th *theme.Theme) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(colorFrom(th, "error", lipgloss.Color("196")))
}

func relativeTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo", int(d.Hours()/(24*30)))
	}
}
