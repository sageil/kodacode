package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (d *themeDialog) renderThemeRows() string {
	selected := dialogSelectedItemStyle(d.theme)
	normal := dialogItemStyle(d.theme)
	rows := make([]string, 0, len(d.visibleThemes()))
	for idx, item := range d.visibleThemes() {
		prefix := "  "
		style := normal
		if d.offset+idx == d.cursor {
			prefix = "> "
			style = selected
		}
		rows = append(rows, style.Render(prefix+item.DisplayName))
	}
	if len(rows) == 0 {
		rows = append(rows, normal.Render("  no matches"))
	}
	return strings.Join(rows, "\n")
}

func (d *skillsDialog) renderSkillRows() string {
	muted := dialogHintStyle(d.theme)
	dialogWidth := d.dialogWidth()
	innerWidth := max(dialogWidth-4, 28)

	detailWidth := min(max(innerWidth/3, 22), 32)
	listWidth := max(innerWidth-detailWidth-3, 20)

	leftLines := d.renderSkillListLines(listWidth)
	rightLines := d.renderSkillDetailLines(detailWidth)

	height := max(len(leftLines), len(rightLines), 1)
	for len(leftLines) < height {
		leftLines = append(leftLines, "")
	}
	for len(rightLines) < height {
		rightLines = append(rightLines, "")
	}

	sep := muted.Render("│")
	rows := make([]string, height)
	for i := 0; i < height; i++ {
		pad := max(listWidth-lipgloss.Width(leftLines[i]), 0)
		rows[i] = leftLines[i] + strings.Repeat(" ", pad) + " " + sep + " " + rightLines[i]
	}

	parts := []string{
		d.renderSkillScopeBar(),
		"",
		strings.Join(rows, "\n"),
		"",
		muted.Render(d.skillSelectionSummary()),
	}
	return strings.Join(parts, "\n")
}

func (d *skillsDialog) renderSkillListLines(listWidth int) []string {
	muted := dialogHintStyle(d.theme)
	selectedStyle := dialogSelectedItemStyle(d.theme)
	normal := dialogItemStyle(d.theme)
	section := dialogSectionStyle(d.theme)

	visible := d.visibleSkills()
	lines := make([]string, 0, len(visible)+4)

	if len(visible) == 0 {
		lines = append(lines, muted.Render("  no skills"))
		return lines
	}

	lastSource := ""
	for idx, item := range visible {
		isCursor := d.offset+idx == d.cursor

		if d.skillScope == "" && item.Source != lastSource {
			if lastSource != "" {
				lines = append(lines, muted.Render(strings.Repeat("─", min(listWidth-2, 28))))
			}
			count := d.countBySource(item.Source)
			lines = append(lines, section.Render(skillSectionHeader(item.Source))+" "+muted.Render(fmt.Sprintf("(%d)", count)))
			lastSource = item.Source
		} else if lastSource == "" {
			lastSource = item.Source
		}

		prefix := "  "
		style := normal
		if isCursor {
			prefix = "> "
			style = selectedStyle
		}

		label := prefix + checkedLabelWithProfile(d.icons, d.skillSelected(item.ID)) + " " + item.ID
		if d.initialSkills != nil && d.initialSkills[item.ID] {
			label += " " + muted.Render(d.icons.Icon(terminalIconSelected))
		}
		lines = append(lines, style.Render(label))
	}
	return lines
}

func (d *skillsDialog) renderSkillDetailLines(detailWidth int) []string {
	muted := dialogHintStyle(d.theme)
	normal := dialogItemStyle(d.theme)

	visible := d.visibleSkills()
	cursorIdx := d.cursor - d.offset
	if cursorIdx < 0 || cursorIdx >= len(visible) {
		return nil
	}
	item := visible[cursorIdx]

	lines := make([]string, 0, 10)
	lines = append(lines, normal.Bold(true).Render(truncateEnd(item.ID, detailWidth)))

	scopeLabel := skillSourceLabel(item.Source)
	if d.initialSkills != nil && d.initialSkills[item.ID] {
		scopeLabel += " · active"
	}
	lines = append(lines, muted.Render(scopeLabel), "")

	if desc := strings.TrimSpace(item.Description); desc != "" {
		for _, line := range wrapWords(desc, detailWidth) {
			lines = append(lines, muted.Render(line))
		}
	}

	return lines
}

func (d *skillsDialog) renderSkillScopeBar() string {
	muted := dialogHintStyle(d.theme)
	activeStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorFor(d.theme, "primary", "#7aa2f7")))

	total := len(d.skillItems)
	type scopeOpt struct {
		id, label string
		count     int
	}
	opts := []scopeOpt{
		{"", "All", total},
		{"project", "Project", d.countBySource("project")},
		{"global", "Global", d.countBySource("global")},
	}

	parts := make([]string, len(opts))
	for i, opt := range opts {
		label := fmt.Sprintf("%s (%d)", opt.label, opt.count)
		if opt.id == d.skillScope {
			parts[i] = activeStyle.Render(label)
		} else {
			parts[i] = muted.Render(label)
		}
	}
	return strings.Join(parts, "  ")
}

func skillSectionHeader(source string) string {
	switch strings.TrimSpace(source) {
	case "project":
		return "PROJECT"
	case "global":
		return "GLOBAL"
	default:
		return strings.ToUpper(strings.TrimSpace(source))
	}
}

func wrapWords(text string, width int) []string {
	if width <= 0 || text == "" {
		return nil
	}
	words := strings.Fields(text)
	var lines []string
	var curr strings.Builder
	for _, word := range words {
		if curr.Len() == 0 {
			curr.WriteString(word)
		} else if curr.Len()+1+len(word) <= width {
			curr.WriteByte(' ')
			curr.WriteString(word)
		} else {
			lines = append(lines, curr.String())
			curr.Reset()
			curr.WriteString(word)
		}
	}
	if curr.Len() > 0 {
		lines = append(lines, curr.String())
	}
	return lines
}

func skillSourceLabel(source string) string {
	switch strings.TrimSpace(source) {
	case "project":
		return "project"
	case "global":
		return "global"
	default:
		return strings.TrimSpace(source)
	}
}

func (d *skillsDialog) skillSelectionSummary() string {
	selected := len(d.selectedSkillIDs())
	total := len(d.skillItems)
	return fmt.Sprintf("%d / %d selected", selected, total)
}
