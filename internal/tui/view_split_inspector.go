package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderSplitTabsComponent(m Model, width, height int) string {
	if m.renderCache.splitInspectorPane == nil {
		return renderSplitTabsComponentUncached(m, width, height)
	}
	return m.renderCache.splitInspectorPane.renderedFor(splitInspectorPaneCacheKey(m, width, height), func() string {
		return renderSplitTabsComponentUncached(m, width, height)
	})
}

func renderSplitTabsComponentUncached(m Model, width, height int) string {
	tabs := renderSplitInspectorTabs(m, width)
	body := strings.TrimRight(m.inspector.body.View(), "\n")
	if strings.TrimSpace(body) == "" {
		body = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render("No active context.")
	}
	if tabs == "" {
		return body
	}
	windowHeight := splitInspectorWindowHeight(m, width, height)
	return tabs + "\n" + renderSplitInspectorWindow(m, body, width, windowHeight)
}

func splitInspectorWindowHeight(m Model, width, totalHeight int) int {
	tabsHeight := lipgloss.Height(renderSplitInspectorTabs(m, width))
	return max(totalHeight-tabsHeight-1, 1)
}

func splitInspectorViewportHeight(m Model, width, totalHeight int) int {
	return max(splitInspectorWindowHeight(m, width, totalHeight)-1, 1)
}

func renderSplitSidebarContent(m Model, state events.SessionState, width int) string {
	detailTurnID := effectiveDetailTurnID(m, state)
	detailTurn := inspectorDetailTurn(state, m)

	switch effectiveInspectorTab(m) {
	case inspectorTabTools:
		return renderToolsListInspector(m, state, width)
	case inspectorTabTasks:
		return renderTasksInspector(m, state, width)
	default:
		return renderOverviewInspector(m, state, detailTurnID, detailTurn, width)
	}
}

func renderSplitInspectorTabs(m Model, width int) string {
	if len(inspectorTabs) == 0 {
		return ""
	}
	activeTab := effectiveInspectorTab(m)
	activeBorder := splitTabBorder("┘", " ", "└")
	inactiveBorder := splitTabBorder("┴", "─", "┴")
	borderColor := lineTone(m)
	if m.chrome.focus == focusInspector {
		borderColor = colorFor(m.theme, "primary", "#7cc7ff")
	}
	textColor := colorFor(m.theme, "text", "#ecf0ff")
	subtextColor := colorFor(m.theme, "subtext", "#9da8ca")

	inactive := lipgloss.NewStyle().
		Border(inactiveBorder, true).
		BorderForeground(lipgloss.Color(borderColor)).
		Foreground(lipgloss.Color(subtextColor)).
		Padding(0, 1)
	active := inactive.
		Border(activeBorder, true).
		BorderForeground(lipgloss.Color(borderColor)).
		Foreground(lipgloss.Color(textColor))

	rendered := make([]string, 0, len(inspectorTabs))
	for _, slot := range splitInspectorTabSlots(width) {
		i, label := slot.Index, slot.Label
		style := inactive
		enabled := inspectorTabEnabled(m, i)
		if !enabled {
			style = style.Foreground(lipgloss.Color(lineTone(m)))
		}
		if enabled && i == activeTab {
			style = active
		}
		border, _, _, _, _ := style.GetBorder()
		switch {
		case i == 0 && i == activeTab:
			border.BottomLeft = "│"
		case i == 0:
			border.BottomLeft = "├"
		case i == len(inspectorTabs)-1 && i == activeTab:
			border.BottomRight = "│"
		case i == len(inspectorTabs)-1:
			border.BottomRight = "┤"
		}
		styledTab := style.Border(border)
		rendered = append(rendered, styledTab.Width(slot.Width).Align(lipgloss.Center).Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
}

func renderSplitInspectorWindow(m Model, body string, width, height int) string {
	borderColor := lineTone(m)
	if m.chrome.focus == focusInspector {
		borderColor = colorFor(m.theme, "primary", "#7cc7ff")
	}
	window := lipgloss.NewStyle().
		Width(max(width, 1)).
		Height(max(height, 1)).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color(borderColor)).
		UnsetBorderTop()
	contentWidth := max(width-window.GetHorizontalFrameSize(), 1)
	contentHeight := max(height-window.GetVerticalFrameSize(), 1)
	return window.Render(placeBlock(contentWidth, contentHeight, "", body))
}

func splitTabBorder(left, middle, right string) lipgloss.Border {
	border := lipgloss.RoundedBorder()
	border.BottomLeft = left
	border.Bottom = middle
	border.BottomRight = right
	return border
}

type splitInspectorTabSlot struct {
	Index int
	Label string
	Start int
	Width int
}

func splitInspectorTabSlots(width int) []splitInspectorTabSlot {
	if len(inspectorTabs) == 0 {
		return nil
	}
	baseWidth := max(width/len(inspectorTabs), 1)
	slots := make([]splitInspectorTabSlot, 0, len(inspectorTabs))
	start := 0
	for i, label := range inspectorTabs {
		slotWidth := baseWidth
		if i == len(inspectorTabs)-1 {
			slotWidth = max(width-baseWidth*(len(inspectorTabs)-1), 1)
		}
		slots = append(slots, splitInspectorTabSlot{
			Index: i,
			Label: label,
			Start: start,
			Width: slotWidth,
		})
		start += slotWidth
	}
	return slots
}

func splitInspectorTabIndexAt(width, x int) int {
	for _, slot := range splitInspectorTabSlots(width) {
		if x >= slot.Start && x < slot.Start+slot.Width {
			return slot.Index
		}
	}
	return -1
}
