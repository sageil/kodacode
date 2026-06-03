package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderDecisionChoices(m Model, width int) string {
	type option struct {
		label string
		key   string
	}
	pending := effectiveExecutionApprovalChoiceState(m)
	options := []option{
		{label: "Allow", key: "1"},
		{label: executionChoicePillLabel(pending), key: "2"},
		{label: "Deny", key: "3"},
	}
	if pending != nil {
		key := 4
		for _, decision := range pending.AvailableDecisions {
			switch decision {
			case events.ExecutionApprovalDecisionApplyNetworkPolicy:
				options = append(options, option{label: "Network", key: fmt.Sprintf("%d", key)})
				key++
			case events.ExecutionApprovalDecisionAcceptWithExecPolicy:
				options = append(options, option{label: "Exec Policy", key: fmt.Sprintf("%d", key)})
				key++
			}
		}
	}

	n := len(options)
	gap := 1
	borderH := 2
	usable := width - gap*(n-1)
	pillW := max(usable/n, borderH+2)

	pills := make([]string, 0, n)
	for idx, opt := range options {
		w := pillW
		if idx == n-1 {
			w = max(usable-pillW*(n-1), borderH+2)
		}
		contentW := max(w-borderH, 1)

		borderColor := lineTone(m)
		fg := colorFor(m.theme, "text", "#ecf0ff")
		if idx == m.interaction.cursor {
			if idx == 2 {
				borderColor = colorFor(m.theme, "error", "#ff9aa6")
				fg = colorFor(m.theme, "error", "#ff9aa6")
			} else {
				borderColor = colorFor(m.theme, "primary", "#7cc7ff")
				fg = colorFor(m.theme, "primary", "#7cc7ff")
			}
		}

		label := lipgloss.NewStyle().
			Width(contentW).
			Align(lipgloss.Center).
			Foreground(lipgloss.Color(fg)).
			Bold(true).
			Render(truncateEnd(opt.label, contentW))
		key := lipgloss.NewStyle().
			Width(contentW).
			Align(lipgloss.Center).
			Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor))).
			Render(opt.key)

		pill := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(borderColor)).
			Render(label + "\n" + key)
		pills = append(pills, pill)
	}

	joined := make([]string, 0, len(pills)*2-1)
	for idx, p := range pills {
		if idx > 0 {
			joined = append(joined, strings.Repeat(" ", gap))
		}
		joined = append(joined, p)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, joined...)
}

func executionChoicePillLabel(pending *events.ExecutionApprovalState) string {
	if pending != nil && len(pending.PrefixRule) > 0 {
		return "Rule"
	}
	return "Session"
}

func renderSessionGrantCard(m Model, state events.SessionState, width int) string {
	body := renderGrantSummaryRows(m, state, max(width-4, 1))
	if body == "" {
		return ""
	}
	return renderInspectorCardExternalTitle(m, "Session Grant", body, width, "")
}

func renderGrantSummaryRows(m Model, state events.SessionState, width int) string {
	if len(state.AdditionalWorkspaceRoots) == 0 && len(state.SessionGrantDecisions) == 0 && len(state.WorkspaceGrants) == 0 && len(state.ExecutionGrants) == 0 && len(state.NetworkGrants) == 0 && strings.TrimSpace(state.PermissionMode) == "" {
		return ""
	}

	type row struct {
		label string
		value string
	}
	rows := make([]row, 0, len(state.AdditionalWorkspaceRoots)+len(state.SessionGrantDecisions)+len(state.WorkspaceGrants)+len(state.ExecutionGrants)+len(state.NetworkGrants)+2)
	appendUniqueRow := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		for idx := range rows {
			if rows[idx].label == label && rows[idx].value == value {
				return
			}
		}
		rows = append(rows, row{label: label, value: value})
	}
	appendCountedRows := func(label string, values []string) {
		type countedRow struct {
			label string
			value string
			count int
		}
		counts := make(map[string]*countedRow, len(values))
		order := make([]string, 0, len(values))
		for _, value := range values {
			trimmed := strings.TrimSpace(value)
			if trimmed == "" {
				continue
			}
			key := label + "\x00" + trimmed
			if existing := counts[key]; existing != nil {
				existing.count++
				continue
			}
			counts[key] = &countedRow{label: label, value: trimmed, count: 1}
			order = append(order, key)
		}
		for _, key := range order {
			item := counts[key]
			if item == nil {
				continue
			}
			value := item.value
			if item.count > 1 {
				value = fmt.Sprintf("%s [x%d]", value, item.count)
			}
			rows = append(rows, row{label: item.label, value: value})
		}
	}

	appendUniqueRow("MODE", sessionPermissionModeLabel(effectiveSessionPermissionMode(m, state)))
	if trimmed := strings.TrimSpace(state.WorkspaceRoot); trimmed != "" {
		appendUniqueRow("ROOT", displaySessionPath(state.WorkspaceRoot, trimmed))
	}
	for _, root := range state.AdditionalWorkspaceRoots {
		if trimmed := strings.TrimSpace(root); trimmed != "" {
			appendUniqueRow("ROOT", displaySessionPath(state.WorkspaceRoot, trimmed))
		}
	}
	decisionRows := make([]string, 0, len(state.SessionGrantDecisions))
	for _, decision := range state.SessionGrantDecisions {
		line := sessionGrantDecisionSummary(state, decision)
		if strings.TrimSpace(line) != "" {
			decisionRows = append(decisionRows, line)
		}
	}
	appendCountedRows("DECISION", decisionRows)
	pathRows := make([]string, 0, len(state.WorkspaceGrants))
	for _, grant := range state.WorkspaceGrants {
		line := displaySessionPath(state.WorkspaceRoot, grant.Path)
		if grant.Recursive {
			line += " (recursive)"
		}
		pathRows = append(pathRows, line)
	}
	appendCountedRows("PATH", pathRows)

	execRows := make([]string, 0, len(state.ExecutionGrants))
	for _, grant := range state.ExecutionGrants {
		if len(grant.PrefixRule) == 0 {
			continue
		}
		execRows = append(execRows, "exec: "+strings.Join(grant.PrefixRule, " "))
	}
	appendCountedRows("EXEC", execRows)

	networkRows := make([]string, 0, len(state.NetworkGrants))
	for _, grant := range state.NetworkGrants {
		target := strings.TrimSpace(grant.Target)
		if target == "" {
			continue
		}
		if strings.HasPrefix(target, "command: ") {
			networkRows = append(networkRows, strings.TrimPrefix(target, "command: "))
			continue
		}
		networkRows = append(networkRows, target)
	}
	appendCountedRows("NETWORK", networkRows)
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows))
	for idx, row := range rows {
		lines = append(lines, renderDetailRow(m, row.label, row.value, width, idx == len(rows)-1))
	}
	return strings.Join(lines, "\n")
}

func executionDecisionLabels(pending events.ExecutionApprovalState) []string {
	lines := []string{
		"1. allow once",
		"2. " + executionSessionDecisionLabel(pending),
		"3. deny",
	}
	next := 4
	for _, decision := range pending.AvailableDecisions {
		switch decision {
		case events.ExecutionApprovalDecisionApplyNetworkPolicy:
			lines = append(lines, fmt.Sprintf("%d. allow with network enabled", next))
			next++
		case events.ExecutionApprovalDecisionAcceptWithExecPolicy:
			lines = append(lines, fmt.Sprintf("%d. allow with execution policy amendment", next))
			next++
		}
	}
	return lines
}

func executionSessionDecisionLabel(pending events.ExecutionApprovalState) string {
	return "allow for session duration"
}

func sessionPermissionModeLabel(mode string) string {
	switch strings.TrimSpace(mode) {
	case "read_only":
		return "read-only"
	case "full_access":
		return "full access"
	default:
		return "auto"
	}
}

func sessionGrantDecisionSummary(state events.SessionState, decision events.SessionGrantDecisionState) string {
	subject := sessionGrantDecisionSubject(state, decision)
	if strings.TrimSpace(subject) == "" {
		return ""
	}
	return "Approved " + subject
}

func sessionGrantDecisionSubject(state events.SessionState, decision events.SessionGrantDecisionState) string {
	switch decision.Source {
	case events.SessionGrantDecisionSourceExecutionApproval:
		if command := strings.TrimSpace(decision.Command); command != "" {
			return command
		}
		if dir := strings.TrimSpace(decision.WorkingDirectory); dir != "" {
			return "exec in " + displaySessionPath(state.WorkspaceRoot, dir)
		}
	case events.SessionGrantDecisionSourcePermission:
		switch decision.PermissionKind {
		case events.PermissionRequestKindPath:
			target := displaySessionPath(state.WorkspaceRoot, decision.Path)
			if tool := strings.TrimSpace(decision.ToolName); tool != "" && strings.TrimSpace(target) != "" {
				return tool + " " + target
			}
			if strings.TrimSpace(target) != "" {
				return target
			}
		case events.PermissionRequestKindNetwork:
			if target := strings.TrimSpace(decision.Path); target != "" {
				return "network " + target
			}
		case events.PermissionRequestKindExecution:
			if command := strings.TrimSpace(decision.Command); command != "" {
				return command
			}
			if dir := strings.TrimSpace(decision.WorkingDirectory); dir != "" {
				return "exec in " + displaySessionPath(state.WorkspaceRoot, dir)
			}
		}
	}
	if command := strings.TrimSpace(decision.Command); command != "" {
		return command
	}
	if path := strings.TrimSpace(decision.Path); path != "" {
		return displaySessionPath(state.WorkspaceRoot, path)
	}
	return strings.TrimSpace(decision.ToolName)
}
