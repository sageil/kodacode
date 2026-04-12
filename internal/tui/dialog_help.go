package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type shortcut struct {
	Key  string
	Desc string
}

// helpSection is a single sidebar entry with its content rows.
type helpSection struct {
	Name  string     // sidebar label
	Group string     // group header shown above this item in sidebar (empty = same group as previous)
	Items []shortcut // content rows
}

var keysSections = []helpSection{
	{
		Name:  "General",
		Group: "Keys",
		Items: []shortcut{
			{"Ctrl+C", "Quit"},
			{"Esc", "Cancel / close dialog"},
			{"Tab", "Cycle agent"},
			{"Ctrl+Y", "Paste from clipboard"},
		},
	},
	{
		Name: "Home View",
		Items: []shortcut{
			{"Ctrl+J / K", "Navigate recent sessions"},
			{"Enter", "Open selected session"},
		},
	},
	{
		Name: "Session View",
		Items: []shortcut{
			{"Page Up/Dn", "Scroll"},
			{"Click", "Toggle tool panel"},
		},
	},
}

var inputSections = []helpSection{
	{
		Name:  "Cursor Movement",
		Group: "Input",
		Items: []shortcut{
			{"Ctrl+A / E", "Line start / end"},
			{"Ctrl+F / B", "Char forward / back"},
			{"Ctrl+N / P", "Next / prev line"},
			{"Alt+F / B", "Word forward / back"},
		},
	},
	{
		Name: "Editing",
		Items: []shortcut{
			{"Ctrl+K", "Delete to line end"},
			{"Ctrl+U", "Delete to line start"},
			{"Ctrl+W", "Delete word backward"},
			{"Ctrl+D", "Delete char forward"},
			{"Shift+Enter", "Insert newline"},
		},
	},
}

// helpCategoryOrder defines which command categories appear and in what order.
var helpCategoryOrder = []string{"Navigation", "Session", "Memory", "Debug", "Agents"}

// buildHelpSections constructs a flat list of sidebar sections from static
// key/input data and the dynamic slash command list.
func buildHelpSections(cmds []SlashCommand) []helpSection {
	var sections []helpSection
	sections = append(sections, keysSections...)

	groups := make(map[string][]SlashCommand)
	for _, c := range cmds {
		if c.Category == "" {
			continue
		}
		groups[c.Category] = append(groups[c.Category], c)
	}
	first := true
	for _, cat := range helpCategoryOrder {
		list := groups[cat]
		if len(list) == 0 {
			continue
		}
		s := helpSection{Name: cat}
		if first {
			s.Group = "Commands"
			first = false
		}
		for _, c := range list {
			key := c.Name
			if c.HelpText != "" {
				key = c.HelpText
			}
			s.Items = append(s.Items, shortcut{key, c.Desc})
		}
		sections = append(sections, s)
	}

	sections = append(sections, inputSections...)
	return sections
}

const (
	helpSidebarWidth = 18
	helpKeyWidth     = 16
	helpDescWidth    = 30
	helpContentWidth = helpKeyWidth + helpDescWidth
	helpDialogWidth  = helpSidebarWidth + helpContentWidth + 2 // separator + space
)

// HelpDialog displays a sidebar-navigated overlay with keyboard shortcuts and commands.
type HelpDialog struct {
	id        string
	theme     *theme.Theme
	activeIdx int
	sections  []helpSection
}

func (d *HelpDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

func NewHelpDialog(id string, th *theme.Theme, cmds []SlashCommand) HelpDialog {
	return HelpDialog{
		id:       id,
		theme:    th,
		sections: buildHelpSections(cmds),
	}
}

func (d HelpDialog) Init() tea.Cmd { return nil }

func (d HelpDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return d, nil
	}
	switch kp.String() {
	case "esc", "ctrl+c":
		return d, closeDialog(d.id, nil)
	case "up", "k":
		if d.activeIdx > 0 {
			d.activeIdx--
		}
	case "down", "j":
		if d.activeIdx < len(d.sections)-1 {
			d.activeIdx++
		}
	}
	return d, nil
}

func (d HelpDialog) View() tea.View {
	accentColor := colorFrom(d.theme, "secondary", lipgloss.Color("4"))
	dimColor := colorFrom(d.theme, "subtext", lipgloss.Color("241"))
	overlayColor := colorFrom(d.theme, "overlay", lipgloss.Color("236"))
	primaryColor := colorFrom(d.theme, "primary", lipgloss.Color("62"))
	borderColor := colorFrom(d.theme, "primary", lipgloss.Color("62"))
	textColor := colorFrom(d.theme, "text", lipgloss.Color("250"))

	// ── sidebar ──
	sidebarTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		PaddingLeft(1).PaddingBottom(1).
		Render("Help")

	var sidebarRows []string
	sidebarRows = append(sidebarRows, sidebarTitle)

	prevGroup := ""
	for i, s := range d.sections {
		if s.Group != "" && s.Group != prevGroup {
			if i > 0 {
				sidebarRows = append(sidebarRows, "")
			}
			prevGroup = s.Group
		}
		label := s.Name
		if i == d.activeIdx {
			row := lipgloss.NewStyle().
				Bold(true).
				Foreground(primaryColor).
				Background(overlayColor).
				Width(helpSidebarWidth - 1).
				PaddingLeft(1).
				Render(label)
			sidebarRows = append(sidebarRows, row)
		} else {
			row := lipgloss.NewStyle().
				Foreground(dimColor).
				Width(helpSidebarWidth - 1).
				PaddingLeft(1).
				Render(label)
			sidebarRows = append(sidebarRows, row)
		}
	}

	sidebar := strings.Join(sidebarRows, "\n")

	// ── content ──
	contentTitle := lipgloss.NewStyle().
		Bold(true).
		Foreground(accentColor).
		MarginBottom(1)

	keyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(primaryColor).
		Width(helpKeyWidth)
	descStyle := lipgloss.NewStyle().
		Foreground(textColor).
		MaxWidth(helpDescWidth)

	var contentRows []string
	if d.activeIdx < len(d.sections) {
		sec := d.sections[d.activeIdx]
		contentRows = append(contentRows, contentTitle.Render(sec.Name))
		for _, item := range sec.Items {
			contentRows = append(contentRows, keyStyle.Render(item.Key)+descStyle.Render(item.Desc))
		}
	}

	hint := hintStyle(d.theme).Render("↑/↓ navigate · esc close")
	contentRows = append(contentRows, "", hint)

	content := strings.Join(contentRows, "\n")

	// ── compose ──
	sidebarBox := lipgloss.NewStyle().
		Width(helpSidebarWidth).
		Render(sidebar)

	height := max(lipgloss.Height(sidebarBox), lipgloss.Height(content))
	sepLines := make([]string, height)
	for i := range sepLines {
		sepLines[i] = "│"
	}
	sep := lipgloss.NewStyle().
		Foreground(borderColor).
		Render(strings.Join(sepLines, "\n"))

	body := lipgloss.JoinHorizontal(lipgloss.Top, sidebarBox, sep, " "+content)
	box := dialogStyle(d.theme, helpDialogWidth).Render(body)
	return tea.NewView(dropShadow(box, d.theme))
}
