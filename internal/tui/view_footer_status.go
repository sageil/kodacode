package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func renderFooterStatusBar(m Model, state events.SessionState, width int) string {
	width = max(width, 1)
	metricsState, metricsTurnID, _ := effectiveStatusMetricsScope(m, state)
	segments := footerStatusSegments(m, state)
	meta := footerStatusMeta(metricsState, metricsTurnID)
	if len(segments) == 0 && meta == "" {
		return ""
	}

	leftWidth := width
	metaRendered := ""
	if meta != "" {
		metaRendered = lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
			Render(meta)
		leftWidth = max(width-lipgloss.Width(metaRendered)-1, 1)
	}
	left := renderTranscriptStatusSegments(m, segments, leftWidth)
	if metaRendered == "" {
		return lipgloss.NewStyle().Width(width).Render(left)
	}
	return lipgloss.NewStyle().Width(width).Render(joinBar(left, metaRendered, width))
}

func footerStatusSegments(m Model, state events.SessionState) []transcriptStatusSegment {
	turnID := effectiveFooterTurnID(m, state)
	metricsState, metricsTurnID, delegated := effectiveStatusMetricsScope(m, state)
	turn := currentTurn(metricsState, metricsTurnID)
	segments := make([]transcriptStatusSegment, 0, 8)

	if label, tone, bold := footerWorkflowLabel(m, state); label != "" {
		segments = append(segments, transcriptStatusSegment{
			Text:  label,
			Color: tone,
			Bold:  bold,
		})
	}
	if agent := footerAgentLabel(m, state, turnID); agent != "" {
		segments = append(segments, transcriptStatusSegment{
			Text:  agent,
			Color: colorFor(m.theme, "primary", "#7cc7ff"),
		})
	}
	if git := footerGitStatus(m.footerStatus.workspace); git != nil {
		if branch := strings.TrimSpace(git.Branch); branch != "" && !shellLayoutEnabled(m) {
			segments = append(segments, transcriptStatusSegment{
				Text:  m.terminalIcon(terminalIconGitBranch) + " " + branch,
				Color: colorFor(m.theme, "success", "#90e5b4"),
			})
		}
		if git.ChangedFiles > 0 {
			segments = append(segments, transcriptStatusSegment{
				Text:  fmt.Sprintf("%d changed", git.ChangedFiles),
				Color: colorFor(m.theme, "warning", "#ffd28f"),
			})
		}
	}
	roundtripCount, workflowCounts := effectiveFooterRoundtripCount(m, state, turn, delegated)
	if roundtripCount > 0 {
		stepLabel := footerRoundtripCountLabel(roundtripCount, workflowCounts)
		segments = append(segments, transcriptStatusSegment{
			Text:  stepLabel,
			Color: footerActivityCountColor(m, roundtripCount),
		})
	}
	toolCount, workflowCounts := effectiveFooterToolCount(m, state, turn, delegated)
	if toolCount > 0 {
		segments = append(segments, transcriptStatusSegment{
			Text:  footerToolCountLabel(toolCount, workflowCounts),
			Color: footerActivityCountColor(m, toolCount),
		})
	}
	if label, tone := footerLSPLabel(m.footerStatus.workspace); label != "" {
		segments = append(segments, transcriptStatusSegment{
			Text:  label,
			Color: tone,
		})
	}
	if label, tone := footerSearchLabel(m.footerStatus.workspace); label != "" {
		segments = append(segments, transcriptStatusSegment{
			Text:  label,
			Color: tone,
		})
	}
	if pendingLoopQuestionFromState(state) != nil {
		segments = append(segments, transcriptStatusSegment{
			Text:  m.terminalIcon(terminalIconWarning) + " loop",
			Color: colorFor(m.theme, "warning", "#ffd28f"),
			Bold:  true,
		})
	}
	if label, tone := footerBudgetLabel(m.theme, m.footerStatus.budget); label != "" {
		segments = append(segments, transcriptStatusSegment{
			Text:  label,
			Color: tone,
		})
	}
	if mode := strings.TrimSpace(sessionPermissionModeLabel(effectiveSessionPermissionMode(m, state))); mode != "" {
		segments = append(segments, transcriptStatusSegment{
			Text:  "mode:" + mode,
			Color: colorFor(m.theme, "subtext", "#9da8ca"),
		})
	}
	return segments
}

func effectiveSessionPermissionMode(m Model, state events.SessionState) string {
	if !sessionStateConfigured(state) {
		if mode := strings.TrimSpace(m.permissionMode); mode != "" {
			return mode
		}
	}
	if mode := strings.TrimSpace(state.PermissionMode); mode != "" {
		return mode
	}
	return strings.TrimSpace(m.permissionMode)
}

func sessionStateConfigured(state events.SessionState) bool {
	return strings.TrimSpace(state.SessionID) != "" ||
		strings.TrimSpace(state.WorkspaceRoot) != "" ||
		len(state.TurnOrder) > 0 ||
		len(state.Turns) > 0
}

func footerWorkflowLabel(m Model, state events.SessionState) (string, string, bool) {
	workflow := visibleFooterWorkflow(state)
	if workflow == nil || strings.TrimSpace(workflow.WorkflowID) == "" {
		if workflowID := strings.TrimSpace(m.workflowID); workflowID != "" {
			return "workflow:" + workflowID, colorFor(m.theme, "subtext", "#9da8ca"), false
		}
		return "", "", false
	}
	label := "workflow:" + strings.TrimSpace(workflow.WorkflowID)
	if phaseID := strings.TrimSpace(workflow.CurrentPhaseID); phaseID != "" {
		label += " phase:" + phaseID
	}
	status := workflowDisplayStatus(m, state, workflow)
	if status != "" {
		label += " " + status
	}
	if reason := workflowStopReason(workflow); reason != "" {
		label += ": " + reason
	}
	switch status {
	case events.WorkflowStatusBlocked:
		return label, colorFor(m.theme, "warning", "#ffd28f"), true
	case events.WorkflowStatusCompleted:
		return label, colorFor(m.theme, "success", "#90e5b4"), false
	default:
		return label, colorFor(m.theme, "primary", "#7cc7ff"), false
	}
}

func visibleFooterWorkflow(state events.SessionState) *events.WorkflowState {
	workflow := state.Workflow
	if workflow == nil || workflow.Status == events.WorkflowStatusCompleted {
		return nil
	}
	return workflow
}

func workflowDisplayStatus(m Model, state events.SessionState, workflow *events.WorkflowState) string {
	if workflow == nil {
		return ""
	}
	status := strings.TrimSpace(workflow.Status)
	if status != events.WorkflowStatusActive {
		return status
	}
	if m.busy || m.liveTurn.spinnerArmed {
		return status
	}
	if turn := currentTurn(state, effectiveFooterTurnID(m, state)); turn != nil {
		if turn.Status == events.TurnStatusRunning {
			return status
		}
		if isTurnFinished(turn) {
			return "paused"
		}
	}
	return status
}

func workflowStopReason(workflow *events.WorkflowState) string {
	if workflow == nil {
		return ""
	}
	if reason := strings.TrimSpace(workflow.StopReason); reason != "" {
		return reason
	}
	phase := workflow.Phases[strings.TrimSpace(workflow.CurrentPhaseID)]
	if phase == nil {
		return ""
	}
	return strings.TrimSpace(phase.StopReason)
}

func footerStatusMeta(state events.SessionState, turnID string) string {
	return ""
}

func footerAgentLabel(m Model, state events.SessionState, turnID string) string {
	turn := currentTurn(state, turnID)
	if turn != nil && !isTurnFinished(turn) && turn.Config != nil {
		if agentID := strings.TrimSpace(turn.Config.AgentID); agentID != "" {
			return agentID
		}
	}
	return transcriptAgentLabel(m, state, turnID)
}

func currentTurnProviderRequestCount(turn *events.TurnState) int {
	if turn == nil || turn.ProviderUsage == nil {
		return 0
	}
	return max(turn.ProviderUsage.Steps, 0)
}

func currentTurnToolCount(turn *events.TurnState) int {
	return len(orderedToolCallIDs(turn))
}

func footerRoundtripCountLabel(n int, workflow bool) string {
	label := "1 roundtrip"
	if n != 1 {
		label = fmt.Sprintf("%d roundtrips", n)
	}
	if workflow {
		return "Σ" + label
	}
	return label
}

func footerToolCountLabel(n int, workflow bool) string {
	label := "1 tool"
	if n != 1 {
		label = fmt.Sprintf("%d tools", n)
	}
	if workflow {
		return "Σ" + label
	}
	return label
}

func effectiveFooterRoundtripCount(m Model, state events.SessionState, turn *events.TurnState, delegated bool) (int, bool) {
	if summary, ok := effectiveFooterWorkflowSummary(m, state, delegated); ok && summary.Steps > 0 {
		return summary.Steps, true
	}
	return currentTurnProviderRequestCount(turn), false
}

func effectiveFooterToolCount(m Model, state events.SessionState, turn *events.TurnState, delegated bool) (int, bool) {
	if summary, ok := effectiveFooterWorkflowSummary(m, state, delegated); ok && summary.ToolCalls > 0 {
		return summary.ToolCalls, true
	}
	return currentTurnToolCount(turn), false
}

func effectiveFooterWorkflowSummary(m Model, state events.SessionState, delegated bool) (app.SessionUsageSummary, bool) {
	return app.SessionUsageSummary{}, false
}

func footerLSPLabel(status app.WorkspaceStatus) (string, string) {
	if status.LSP == nil {
		return "", ""
	}
	active := normalizeStatusNames(status.LSP.ActiveServers)
	if len(active) == 0 {
		return "", ""
	}
	return "lsp:" + strings.Join(active, ","), "#39bae6"
}

func footerSearchLabel(status app.WorkspaceStatus) (string, string) {
	if status.Search == nil || !status.Search.Configured {
		return "", ""
	}
	switch {
	case strings.TrimSpace(status.Search.LastWarmupError) != "":
		if searchWarmupErrorLooksEmbeddingOffline(status.Search.LastWarmupError) {
			return "search:embed-offline", "#ff8f8f"
		}
		return "search:error", "#ff8f8f"
	case status.Search.TrackedFiles == 0:
		return "search:idle", "#9da8ca"
	case status.Search.IndexedFiles == 0 && status.Search.PendingFiles > 0:
		return "search:cold", "#ffd28f"
	case status.Search.PendingFiles > 0:
		return fmt.Sprintf("search:%d/%d", status.Search.IndexedFiles, status.Search.TrackedFiles), "#ffd28f"
	default:
		return "search:warm", "#90e5b4"
	}
}

func searchWarmupErrorLooksEmbeddingOffline(raw string) bool {
	errText := strings.ToLower(strings.TrimSpace(raw))
	if errText == "" {
		return false
	}
	embeddingContext := strings.Contains(errText, "/v1/embeddings") ||
		strings.Contains(errText, "embedding") ||
		strings.Contains(errText, "embeddings")
	if !embeddingContext {
		return false
	}
	for _, marker := range []string{
		"connection refused",
		"connect: connection refused",
		"no such host",
		"network is unreachable",
		"connection reset",
		"connection reset by peer",
		"connection timed out",
		"i/o timeout",
		"econnrefused",
		"enotfound",
	} {
		if strings.Contains(errText, marker) {
			return true
		}
	}
	return false
}

func normalizeStatusNames(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(raw))
	names := make([]string, 0, len(raw))
	for _, name := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

func footerActivityCountColor(m Model, count int) string {
	switch {
	case count >= 8:
		return colorFor(m.theme, "error", "#ff9aa6")
	case count >= 5:
		return colorFor(m.theme, "warning", "#ffd28f")
	default:
		return colorFor(m.theme, "subtext", "#9da8ca")
	}
}

func sessionEstimatedCostLabel(state events.SessionState) string {
	cost := sessionEstimatedCost(state)
	if cost <= 0 {
		return ""
	}
	return "est. " + formatCompactEstimatedCost(cost)
}

func effectiveSessionUsageSummary(m Model, state events.SessionState) (app.SessionUsageSummary, bool) {
	if !m.footerStatus.sessionUsage.ValidFor(state.SessionID) {
		return app.SessionUsageSummary{}, false
	}
	if !m.footerStatus.sessionUsage.HasUsage() {
		return app.SessionUsageSummary{}, false
	}
	return m.footerStatus.sessionUsage, true
}

func effectiveSessionEstimatedCostLabel(m Model, state events.SessionState) string {
	if summary, ok := effectiveSessionUsageSummary(m, state); ok {
		if summary.EstimatedCost <= 0 {
			return ""
		}
		return "est. " + formatCompactEstimatedCost(summary.EstimatedCost)
	}
	return sessionEstimatedCostLabel(state)
}

func sessionEstimatedCost(state events.SessionState) float64 {
	total := 0.0
	for _, turn := range state.Turns {
		if turn == nil || turn.ProviderUsage == nil {
			continue
		}
		total += turn.ProviderUsage.EstimatedInputCost + turn.ProviderUsage.EstimatedOutputCost
	}
	return total
}

func footerBudgetLabel(th *theme.Theme, status app.BudgetStatus) (string, string) {
	switch {
	case status.TotalExceeded:
		return "total hit", colorFor(th, "error", "#ff9aa6")
	case status.WorkflowExceeded:
		return "workflow hit", colorFor(th, "error", "#ff9aa6")
	case status.SessionExceeded:
		return "budget hit", colorFor(th, "error", "#ff9aa6")
	case status.TotalWarn:
		if percent, ok := status.TotalPercent(); ok {
			return fmt.Sprintf("total %d%%", percent), colorFor(th, "warning", "#ffd28f")
		}
		return "total warn", colorFor(th, "warning", "#ffd28f")
	case status.WorkflowWarn:
		if percent, ok := status.WorkflowPercent(); ok {
			return fmt.Sprintf("workflow %d%%", percent), colorFor(th, "warning", "#ffd28f")
		}
		return "workflow warn", colorFor(th, "warning", "#ffd28f")
	case status.SessionWarn:
		if percent, ok := status.SessionPercent(); ok {
			return fmt.Sprintf("budget %d%%", percent), colorFor(th, "warning", "#ffd28f")
		}
		return "budget warn", colorFor(th, "warning", "#ffd28f")
	default:
		return "", ""
	}
}

func formatEstimatedCost(cost float64) string {
	switch {
	case cost >= 10:
		return fmt.Sprintf("$%.2f", cost)
	case cost >= 1:
		return fmt.Sprintf("$%.3f", cost)
	case cost >= 0.1:
		return fmt.Sprintf("$%.4f", cost)
	default:
		return fmt.Sprintf("$%.5f", cost)
	}
}

func formatCompactEstimatedCost(cost float64) string {
	switch {
	case cost >= 10:
		return fmt.Sprintf("$%.2f", cost)
	case cost >= 1:
		return fmt.Sprintf("$%.3f", cost)
	default:
		return fmt.Sprintf("$%.4f", cost)
	}
}

func renderFooterHintsLine(m Model, state events.SessionState, width int) string {
	noticeText, _ := footerNoticeText(m, state)
	if !m.chrome.hintsExpanded || strings.TrimSpace(noticeText) != "" {
		collapsed := lipgloss.NewStyle().
			Width(max(width, 1)).
			Align(lipgloss.Right).
			Foreground(lipgloss.Color(colorFor(m.theme, "soft", softTextColor))).
			Render("? shortcuts")
		return lipgloss.NewStyle().Width(max(width, 1)).Render(collapsed)
	}
	hints := shellStatusHints(m, state)
	hintsRendered := lipgloss.NewStyle().
		Width(max(width, 1)).
		Align(lipgloss.Right).
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render(truncateEnd(hints, max(width, 1)))
	return lipgloss.NewStyle().Width(max(width, 1)).Render(hintsRendered)
}
