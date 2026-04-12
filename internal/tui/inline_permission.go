package tui

import (
	"log"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// InlinePermissionPanel is a compact inline panel for permission requests.
// It replaces the task panel area and accepts single-keypress input:
//
//	[1] Allow  [2] Always allow  [3] Deny
//
// Result type on confirmation: PermissionAction.
type InlinePermissionPanel struct {
	id    string
	req   PermissionRequest
	width int
	theme *theme.Theme
}

func NewInlinePermissionPanel(id string, req PermissionRequest, w int, th *theme.Theme) InlinePermissionPanel {
	req.Arguments = humanizeArgs(req.ToolName, req.Arguments)
	return InlinePermissionPanel{id: id, req: req, width: w, theme: th}
}

func (p InlinePermissionPanel) PanelHeight() int {
	height := 4 // accent border + tool name + options + bottom separator
	if p.req.Arguments != "" {
		height += wrappedInlineLineCount(p.req.Arguments, max(p.width-4, 20))
	}
	return height
}

func (p InlinePermissionPanel) Init() tea.Cmd { return nil }

func (p InlinePermissionPanel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	kp, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return p, nil
	}
	switch kp.String() {
	case "1", "y":
		log.Printf("tui: inline permission %s: Allow once (tool=%s)", p.id, p.req.ToolName)
		return p, closeDialog(p.id, PermissionAllow)
	case "2", "a":
		log.Printf("tui: inline permission %s: Always allow (tool=%s)", p.id, p.req.ToolName)
		return p, closeDialog(p.id, PermissionAllowAlways)
	case "3", "n":
		log.Printf("tui: inline permission %s: Deny (tool=%s)", p.id, p.req.ToolName)
		return p, closeDialog(p.id, PermissionDeny)
	case "esc", "ctrl+c":
		log.Printf("tui: inline permission %s: Deny via esc (tool=%s)", p.id, p.req.ToolName)
		return p, closeDialog(p.id, PermissionDeny)
	}
	return p, nil
}

func (p InlinePermissionPanel) View() tea.View {
	w := p.width
	if w < 1 {
		w = 80
	}

	accentColor := colorFrom(p.theme, "warning", lipgloss.Color("214"))
	dimColor := colorFrom(p.theme, "subtext", lipgloss.Color("241"))
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)
	warnStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	optStyle := lipgloss.NewStyle().Foreground(colorFrom(p.theme, "primary", lipgloss.Color("62"))).Bold(true)

	topBorder := lipgloss.NewStyle().Foreground(accentColor).Render(strings.Repeat("▔", w))
	bottomSep := dimStyle.Render(strings.Repeat("─", w))

	toolLine := warnStyle.Render("⚠ " + p.req.ToolName)
	options := optStyle.Render("[1]") + " Allow  " +
		optStyle.Render("[2]") + " Always allow  " +
		optStyle.Render("[3]") + " Deny"

	var sb strings.Builder
	sb.WriteString(topBorder)

	sb.WriteByte('\n')
	sb.WriteString(centerLine(toolLine, w))

	if p.req.Arguments != "" {
		for _, line := range wrappedInlineLines(p.req.Arguments, max(w-4, 20)) {
			sb.WriteByte('\n')
			sb.WriteString("  ")
			sb.WriteString(dimStyle.Render(line))
		}
	}

	sb.WriteByte('\n')
	sb.WriteString(centerLine(options, w))

	sb.WriteByte('\n')
	sb.WriteString(bottomSep)
	return tea.NewView(sb.String())
}

func centerLine(s string, w int) string {
	sw := lipgloss.Width(s)
	pad := max((w-sw)/2, 0)
	return strings.Repeat(" ", pad) + s
}

func wrappedInlineLineCount(s string, width int) int {
	return len(wrappedInlineLines(s, width))
}

func wrappedInlineLines(s string, width int) []string {
	if width < 1 {
		return nil
	}
	block := lipgloss.NewStyle().Width(width).Render(s)
	lines := strings.Split(block, "\n")
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}
