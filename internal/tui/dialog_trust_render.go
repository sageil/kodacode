package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (d *trustDialog) renderBody() string {
	width := trustDialogWidth(d.frameWidth)
	contentWidth := max(width-2, 1)
	mainWidth := max(contentWidth-trustDialogSidebarWidth-1, 1)

	sidebar := d.renderSidebar()
	main := d.renderMainPanel(mainWidth)

	sidebarH := lipgloss.Height(sidebar)
	mainH := lipgloss.Height(main)
	totalH := max(sidebarH, mainH)

	divLines := make([]string, totalH)
	for i := range divLines {
		divLines[i] = "│"
	}
	divider := lipgloss.NewStyle().
		Foreground(lipgloss.Color(dialogLineTone(d.theme))).
		Render(strings.Join(divLines, "\n"))

	sidebarBlock := lipgloss.NewStyle().Width(trustDialogSidebarWidth).Render(sidebar)
	mainBlock := lipgloss.NewStyle().Width(mainWidth).Render(main)

	return lipgloss.JoinHorizontal(lipgloss.Top, sidebarBlock, divider, mainBlock)
}

func (d *trustDialog) renderSidebar() string {
	entriesLabel := " Entries"
	dangerLabel := " Danger"

	dots := strings.Repeat(".", trustDialogSidebarWidth)
	dotStr := lipgloss.NewStyle().
		Foreground(lipgloss.Color(dialogLineTone(d.theme))).
		Render(dots)

	var entriesStr, dangerStr string
	warningColor := colorFor(d.theme, "warning", "#ffb454")

	if d.panel == trustDialogPanelEntries {
		entriesStr = dialogSelectedItemStyle(d.theme).
			Width(trustDialogSidebarWidth).
			Render(">>" + entriesLabel)
		dangerStr = lipgloss.NewStyle().
			Foreground(lipgloss.Color(warningColor)).
			Render("  " + dangerLabel)
	} else {
		entriesStr = dialogItemStyle(d.theme).Render("  " + entriesLabel)
		dangerStr = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color(warningColor)).
			Width(trustDialogSidebarWidth).
			Render(">>" + dangerLabel)
	}

	return strings.Join([]string{entriesStr, dotStr, dangerStr}, "\n")
}

func (d *trustDialog) renderMainPanel(width int) string {
	if d.confirm != nil {
		return d.renderConfirmPanel(width)
	}
	if d.panel == trustDialogPanelDanger {
		return d.renderDangerPanel(width)
	}
	return d.renderEntriesPanel(width)
}

func (d *trustDialog) renderEntriesPanel(width int) string {
	lines := []string{
		dialogHintStyle(d.theme).Render(truncateEnd("Trusted Workspaces", max(width-1, 1))),
		"",
	}
	list := d.renderRows(width)
	if list != "" {
		lines = append(lines, list)
	} else {
		lines = append(lines, dialogHintStyle(d.theme).Render("No trust records in this workspace."))
	}
	return strings.Join(lines, "\n")
}

func (d *trustDialog) renderDangerPanel(width int) string {
	warningColor := colorFor(d.theme, "warning", "#ffb454")
	warnStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(warningColor)).Bold(true)
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(dialogLineTone(d.theme)))

	boxW := max(width-2, 12)
	innerW := max(boxW-4, 1)

	renderBox := func(title, desc, key string) string {
		top := borderStyle.Render("." + strings.Repeat("-", boxW-2) + ".")
		bot := borderStyle.Render("'" + strings.Repeat("-", boxW-2) + "'")
		titleLine := borderStyle.Render("| ") + warnStyle.Width(innerW).Render(truncateEnd(title, innerW)) + borderStyle.Render(" |")
		descLine := borderStyle.Render("| ") + dialogHintStyle(d.theme).Width(innerW).Render(truncateEnd(desc, innerW)) + borderStyle.Render(" |")
		keyLine := borderStyle.Render("| ") + dialogItemStyle(d.theme).Width(innerW).Render(truncateEnd("  press ["+key+"] to confirm", innerW)) + borderStyle.Render(" |")
		return strings.Join([]string{top, titleLine, descLine, keyLine, bot}, "\n")
	}

	return strings.Join([]string{
		warnStyle.Render("! Danger Zone"),
		"",
		renderBox("Revoke workspace trust", "Removes trust for this workspace and MCPs.", "a"),
		"",
		renderBox("Revoke all trust", "Removes trust across all workspaces.", "A"),
	}, "\n")
}

func (d *trustDialog) renderConfirmPanel(width int) string {
	return strings.Join([]string{
		dialogSectionStyle(d.theme).Render("Confirm"),
		"",
		dialogSelectedItemStyle(d.theme).Render(truncateEnd(d.confirm.Prompt, max(width-1, 1))),
	}, "\n")
}

func (d *trustDialog) renderHint() string {
	if d.confirm != nil {
		return "enter confirm • esc cancel • q close"
	}
	if d.panel == trustDialogPanelDanger {
		return "a/A act • tab entries • esc close"
	}
	return "↑/↓ move • r revoke • tab danger zone • esc close"
}

func (d *trustDialog) renderRows(width int) string {
	if len(d.rows) == 0 {
		return ""
	}
	start := d.offset
	end := min(start+d.visibleRowCount(), len(d.rows))
	maxWidth := max(width-2, 1)
	labelWidth := max(maxWidth-4, 1)
	lines := make([]string, 0, (end-start)*2)
	for idx := start; idx < end; idx++ {
		row := d.rows[idx]
		label := d.rowLabel(row)
		detail := d.rowDetail(row)
		if idx == d.cursor {
			lines = append(lines, dialogSelectedItemStyle(d.theme).Render("[ * ] "+truncateEnd(label, labelWidth)))
		} else {
			lines = append(lines, dialogItemStyle(d.theme).Render("[   ] "+truncateEnd(label, labelWidth)))
		}
		if detail != "" {
			lines = append(lines, dialogHintStyle(d.theme).Render("      "+truncateEnd(detail, labelWidth)))
		}
	}
	if start > 0 {
		lines[0] = dialogHintStyle(d.theme).Render("↑ more") + "\n" + lines[0]
	}
	if end < len(d.rows) {
		lines = append(lines, dialogHintStyle(d.theme).Render("↓ more"))
	}
	return strings.Join(lines, "\n")
}
