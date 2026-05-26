package tui

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderOverviewInspector(m Model, state events.SessionState, turnID string, turn *events.TurnState, width int) string {
	parts := make([]string, 0, 2)
	context := resolveInspectorAgentContext(m, state, turnID, turn)
	detailWidth := max(width-4, 1)
	rows := []struct {
		label string
		value string
		mark  string
		dim   bool
	}{
		{label: "AGENT", value: context.AgentLabel, dim: true},
		{label: "STATUS", value: context.StatusLabel},
		{label: "TURN", value: context.TurnLabel},
		{label: "MODEL", value: context.ModelLabel},
		{label: "PROVIDER", value: context.ProviderLabel, dim: true},
		func() struct {
			label string
			value string
			mark  string
			dim   bool
		} {
			value, fallback := inspectorUtilityModelValue(m, state)
			mark := ""
			if fallback {
				mark = "*"
			}
			return struct {
				label string
				value string
				mark  string
				dim   bool
			}{label: "UTILITY MODEL", value: value, mark: mark, dim: true}
		}(),
	}
	if strings.TrimSpace(context.ReasoningLabel) != "" {
		rows = append(rows, struct {
			label string
			value string
			mark  string
			dim   bool
		}{label: "REASONING", value: context.ReasoningLabel, dim: true})
	}
	rows = append(rows,
		struct {
			label string
			value string
			mark  string
			dim   bool
		}{label: "PROJECT", value: context.PWDLabel, dim: true},
		struct {
			label string
			value string
			mark  string
			dim   bool
		}{label: "THEME", value: context.ThemeLabel, dim: true},
	)
	detailLines := make([]string, 0, len(rows))
	for idx, row := range rows {
		if strings.TrimSpace(row.mark) != "" {
			detailLines = append(detailLines, renderDetailRowWithMarker(m, row.label, row.value, row.mark, detailWidth, idx == len(rows)-1))
		} else if row.dim {
			detailLines = append(detailLines, renderDetailRowDimmed(m, row.label, row.value, detailWidth, idx == len(rows)-1))
		} else {
			detailLines = append(detailLines, renderDetailRow(m, row.label, row.value, detailWidth, idx == len(rows)-1))
		}
	}
	detailRows := strings.Join(detailLines, "\n")
	parts = append(parts, renderInspectorCardExternalTitle(m, "Environment", detailRows, width, ""))

	if grants := renderSessionGrantCard(m, state, width); grants != "" {
		parts = append(parts, grants)
	}
	return strings.Join(parts, "\n\n")
}

func inspectorUtilityModelValue(m Model, state events.SessionState) (string, bool) {
	if err := m.providerCatalog.utilityModel.Validate(); err == nil {
		return m.providerCatalog.utilityModel.String(), false
	}
	if ref, ok := effectiveSelectedAgentModelRef(m, state); ok {
		return ref.String(), true
	}
	return "unset", false
}

func inspectorPWDLabel(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(trimmed))
	switch base {
	case "", ".", string(filepath.Separator):
		return trimmed
	default:
		return base
	}
}

func renderDetailRow(m Model, label, value string, width int, last bool) string {
	return renderDetailRowWithMarker(m, label, value, "", width, last)
}

func renderDetailRowDimmed(m Model, label, value string, width int, last bool) string {
	softColor := lipgloss.Color(colorFor(m.theme, "soft", softTextColor))
	labelStyle := lipgloss.NewStyle().Foreground(softColor)
	valueStyle := lipgloss.NewStyle().Foreground(softColor)
	row := joinBar(labelStyle.Render(label), valueStyle.Render(value), max(width, 1))
	if last {
		return row
	}
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render(strings.Repeat("─", max(width, 1)))
	return row + "\n" + sep
}

func renderDetailRowWithMarker(m Model, label, value, marker string, width int, last bool) string {
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor)))
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
		Bold(true)
	valueRendered := valueStyle.Render(value)
	if strings.TrimSpace(marker) != "" {
		markerStyle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "warning", "#ffd28f"))).
			Bold(true)
		valueRendered += markerStyle.Render(marker)
	}
	row := joinBar(labelStyle.Render(label), valueRendered, max(width, 1))
	if last {
		return row
	}
	sep := lipgloss.NewStyle().
		Foreground(lipgloss.Color(lineTone(m))).
		Render(strings.Repeat("─", max(width, 1)))
	return row + "\n" + sep
}

func renderCommandBox(m Model, body string, width int) string {
	return lipgloss.NewStyle().
		Width(max(width, 1)).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(lineTone(m))).
		Padding(0, 1).
		Render(body)
}

func renderInspectorBlock(m Model, title, body string, width int) string {
	return renderInspectorCard(m, title, body, width, "")
}
