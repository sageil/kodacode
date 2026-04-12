package tui

import (
	"encoding/json"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

type PermissionAction int

const (
	PermissionAllow PermissionAction = iota
	PermissionAllowAlways
	PermissionDeny
)

type PermissionRequest struct {
	ToolName  string
	Arguments string
}

// PermissionDialog asks the user whether to allow, always allow, or deny a
// pending tool call.
//
// Result type on confirmation: PermissionAction.
// Result is nil on cancellation (treated as PermissionDeny by callers).
type PermissionDialog struct {
	id      string
	req     PermissionRequest
	options []string // display labels in cursor order
	cursor  int
	keys    dialogKeys
	width   int
	theme   *theme.Theme
}

func (d *PermissionDialog) ApplyTheme(t *theme.Theme) {
	d.theme = t
}

var permissionOptions = []string{"Allow once", "Always allow", "Deny"}

func NewPermissionDialog(id string, req PermissionRequest, th *theme.Theme) PermissionDialog {
	req.Arguments = humanizeArgs(req.ToolName, req.Arguments)

	// Auto-size width from content, capped so the dialog doesn't overflow.
	const pad = 10
	const minWidth, maxWidth = 50, 120
	w := len("Permission Required")
	if l := len("Tool: " + req.ToolName); l > w {
		w = l
	}
	if len(req.Arguments) > w {
		w = len(req.Arguments)
	}
	w += pad
	w = max(w, minWidth)
	w = min(w, maxWidth)

	return PermissionDialog{
		id:      id,
		req:     req,
		options: permissionOptions,
		keys:    defaultDialogKeys(),
		width:   w,
		theme:   th,
	}
}

func humanizeArgs(toolName, rawJSON string) string {
	if rawJSON == "" {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &m); err != nil {
		return rawJSON
	}
	if display, ok := m["permission_display"].(string); ok && display != "" {
		return display
	}
	if paths, ok := m["permission_paths"].([]any); ok && len(paths) > 0 {
		var lines []string
		for _, raw := range paths {
			if path, ok := raw.(string); ok && path != "" {
				lines = append(lines, path)
			}
		}
		if len(lines) > 0 {
			return strings.Join(lines, "\n")
		}
	}
	// For bash, show just the command.
	if toolName == "bash" {
		if cmd, ok := m["command"].(string); ok {
			return cmd
		}
	}
	// For file tools, show the path.
	for _, key := range []string{"filePath", "file_path", "path"} {
		if v, ok := m[key].(string); ok {
			return v
		}
	}
	// Fallback: show the raw JSON.
	return rawJSON
}

func (d *PermissionDialog) SetWidth(w int) { d.width = w }
func (d PermissionDialog) Width() int      { return d.width }

func (d PermissionDialog) Init() tea.Cmd { return nil }

func (d PermissionDialog) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
	case key.Matches(kp, d.keys.Select):
		action := PermissionAction(d.cursor)
		return d, closeDialog(d.id, action)
	case key.Matches(kp, d.keys.Cancel):
		return d, closeDialog(d.id, PermissionDeny)
	}
	return d, nil
}

func (d PermissionDialog) View() tea.View {
	title := titleStyle(d.theme).Render("⚠ Permission Required")
	prompt := itemStyle(d.theme).Render("Tool: " + d.req.ToolName)

	var argsLine string
	if d.req.Arguments != "" {
		// Render command in its own inline box for visibility.
		// Width is capped so long commands wrap instead of overflowing.
		cmdBoxWidth := max(d.width-8, 20) // account for dialog padding and border
		cmdBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorFrom(d.theme, "subtext", lipgloss.Color("241"))).
			Padding(0, 1).
			Width(cmdBoxWidth).
			Render(hintStyle(d.theme).Render(d.req.Arguments))
		argsLine = "\n" + cmdBox
	}

	hint := hintStyle(d.theme).Render("↑/↓ navigate • enter select • esc deny")
	sel := selectedItemStyle(d.theme)
	norm := itemStyle(d.theme)

	var rows []string
	for i, label := range d.options {
		if i == d.cursor {
			rows = append(rows, sel.Render("● "+label))
		} else {
			rows = append(rows, norm.Render("○ "+label))
		}
	}

	var bb strings.Builder
	bb.WriteString(title + "\n\n" + prompt + argsLine + "\n\n")
	for _, r := range rows {
		bb.WriteString(r)
		bb.WriteByte('\n')
	}
	bb.WriteString("\n" + hint)
	body := bb.String()

	box := dialogStyle(d.theme, d.width).Render(body)
	return tea.NewView(dropShadow(box, d.theme))
}
