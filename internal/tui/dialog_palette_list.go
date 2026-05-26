package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

func (d *commandPaletteDialog) renderListRows() string {
	options := d.listOptions()
	// No background on the selection — avoids a background block starting mid-row
	// that would visually misalign the badge from the label.
	// indentLines() in renderPaletteDialogBody already adds the leading "  " indent.
	selLabel := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorFor(d.theme, "primary", "#7aa2f7")))
	normal := dialogItemStyle(d.theme)
	subtle := dialogHintStyle(d.theme)

	if len(options) == 0 {
		return normal.Render("no matches")
	}

	start, end := d.visibleRange(len(options))
	visible := options[start:end]

	parts := []string{}
	renderModelRows := d.kind == commandPaletteModel || d.kind == commandPaletteUtilityModel || d.kind == commandPaletteReviewerModel
	renderedModelHeader := false
	badge := renderListBadge(d.theme, d.kind)
	for localIdx, item := range visible {
		globalIdx := start + localIdx
		if renderModelRows && !renderedModelHeader {
			parts = append(parts, d.renderPaletteModelHeaderLine("[ model ] "))
			renderedModelHeader = true
		}

		desc := strings.TrimSpace(item.Description)
		if renderModelRows {
			parts = append(parts, d.renderPaletteModelLine(item.Model, globalIdx == d.cursor, badge+" ", desc, item.Disabled))
			continue
		}
		if item.Disabled {
			row := badge + subtle.Render(" "+item.Label)
			if desc != "" {
				row += "  " + subtle.Render(desc)
			}
			parts = append(parts, row)
			continue
		}
		if globalIdx == d.cursor {
			row := badge + selLabel.Render(" "+item.Label)
			if desc != "" {
				row += "  " + subtle.Render(desc)
			}
			parts = append(parts, row)
		} else {
			row := badge + normal.Render(" "+item.Label)
			if desc != "" {
				row += "  " + subtle.Render(desc)
			}
			parts = append(parts, row)
		}
	}
	return strings.Join(parts, "\n")
}
