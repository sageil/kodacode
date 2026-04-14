package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (f Footer) viewFullWidth() string {
	w := max(f.width, 1)
	dimColor := colorFrom(f.theme, "subtext", lipgloss.Color("241"))
	muted := lipgloss.NewStyle().Foreground(dimColor)
	accentColor := colorFrom(f.theme, "secondary", lipgloss.Color("4"))
	accentStyle := lipgloss.NewStyle().Foreground(accentColor)

	if f.blocked {
		warnColor := colorFrom(f.theme, "warning", lipgloss.Color("214"))
		warnStyle := lipgloss.NewStyle().Foreground(warnColor)
		return warnStyle.Render("  ⚠ respond to the panel above…")
	}

	inputView := f.input.View()

	var searchView string
	if f.historySearch {
		accent := lipgloss.NewStyle().Foreground(accentColor)
		dim := lipgloss.NewStyle().Foreground(dimColor)
		searchLine := accent.Render("reverse-search: ") + f.historySearchQuery + accent.Render("_")
		if f.historySearchResult != "" {
			result := f.historySearchResult
			if len(result) > w-20 {
				result = result[:w-23] + "..."
			}
			searchLine += "  " + dim.Render(result)
		}
		searchView = searchLine + "\n"
	}

	var sb strings.Builder
	sb.WriteString(f.renderSlashCompletion())
	if contextView := f.renderContextRow(w, muted, accentStyle); contextView != "" {
		sb.WriteString(contextView)
		sb.WriteString("\n")
	}
	sb.WriteString(searchView)
	sb.WriteString(f.renderInputRow(w, inputView))
	if infoView := f.renderInfoRow(w, muted); infoView != "" {
		sb.WriteString("\n")
		sb.WriteString(infoView)
	}
	return sb.String()
}

func (f Footer) viewBoxed() string {
	borderColor := colorFrom(f.theme, "primary", lipgloss.Color("62"))

	inputView := f.input.View()

	var searchView string
	if f.historySearch {
		w := f.boxWidth - 4
		accentColor := colorFrom(f.theme, "secondary", lipgloss.Color("4"))
		accent := lipgloss.NewStyle().Foreground(accentColor)
		dim := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "subtext", lipgloss.Color("241")))
		searchLine := accent.Render("reverse-search: ") + f.historySearchQuery + accent.Render("_")
		if f.historySearchResult != "" {
			result := f.historySearchResult
			if len(result) > w-20 {
				result = result[:w-23] + "..."
			}
			searchLine += "  " + dim.Render(result)
		}
		searchView = searchLine + "\n"
	}

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(f.boxWidth - 2).
		Render(searchView + inputView)

	return f.renderSlashCompletion() + box
}

func (f Footer) renderContextRow(w int, muted, accentStyle lipgloss.Style) string {
	if f.streaming {
		return f.renderStreamingRow(w, muted, accentStyle)
	}
	if !f.showRecentContext() {
		return ""
	}
	parts := f.contextStats()
	if len(parts) == 0 {
		return ""
	}
	rowText := strings.Join(parts, "  ")
	if lipgloss.Width(rowText) > w-2 {
		rowText = truncateTail(rowText, max(w-2, 1))
	}
	return " " + muted.Render(rowText) + " "
}

func (f Footer) renderInputRow(w int, inputView string) string {
	if !f.streaming {
		return inputView
	}

	bodyWidth := max(w-2, 1)
	leftRail := lipgloss.NewStyle().
		Foreground(colorFrom(f.theme, "secondary", lipgloss.Color("4"))).
		Render("▎")
	rightRail := lipgloss.NewStyle().
		Foreground(colorFrom(f.theme, "subtext", lipgloss.Color("241"))).
		Faint(true).
		Render("▕")
	bodyStyle := lipgloss.NewStyle().
		Background(colorFrom(f.theme, "surface", lipgloss.Color("236"))).
		Width(bodyWidth)

	lines := strings.Split(inputView, "\n")
	for i, line := range lines {
		lines[i] = leftRail + bodyStyle.Render(ansi.Truncate(line, bodyWidth, "")) + rightRail
	}
	return strings.Join(lines, "\n")
}

func (f Footer) renderStreamingRow(w int, muted, accentStyle lipgloss.Style) string {
	var sb strings.Builder
	sb.WriteString(" ")

	steps := f.toolLoopStep
	phase := pulseTick % 16
	pulseDot := accentStyle.Render("●")
	if phase >= 8 {
		pulseDot = accentStyle.Faint(true).Render("●")
	}
	if f.compacting {
		sb.WriteString(pulseDot)
		sb.WriteString("  " + muted.Render("compacting"))
	} else if steps > 0 {
		maxDots := 10
		shown := min(steps, maxDots)
		for i := 1; i <= shown; i++ {
			sb.WriteString(accentStyle.Render("●"))
		}
		sb.WriteString(" " + pulseDot)
		sb.WriteString("  " + muted.Render(fmt.Sprintf("step %d", steps)))
	} else {
		sb.WriteString(pulseDot)
		sb.WriteString("  " + muted.Render("thinking"))
	}
	if !f.streamStartTime.IsZero() {
		timerStr := formatFooterDuration(time.Since(f.streamStartTime).Truncate(time.Second))
		leftWidth := lipgloss.Width(sb.String()) + 3
		gap := max(w-leftWidth-lipgloss.Width(timerStr)-1, 1)
		sb.WriteString(strings.Repeat(" ", gap))
		sb.WriteString(accentStyle.Render(timerStr))
	}
	sb.WriteString(" ")
	return sb.String()
}

func (f Footer) contextStats() []string {
	var parts []string
	if f.lastTurnElapsed > 0 {
		parts = append(parts, "last turn "+formatFooterDuration(f.lastTurnElapsed))
	}
	if costStr := f.footerCostString(); costStr != "" {
		parts = append(parts, costStr)
	}
	if f.inputTokens > 0 {
		parts = append(parts, "input "+formatTokenCount(f.inputTokens))
	}
	if f.outputTokens > 0 {
		parts = append(parts, "output "+formatTokenCount(f.outputTokens))
	}
	if contextPct := f.contextUsagePercent(); contextPct > 0 {
		parts = append(parts, fmt.Sprintf("context %.0f%%", contextPct))
	}
	if f.budgetWarn {
		parts = append(parts, "budget warning")
	}
	return parts
}

func (f Footer) footerCostString() string {
	costVal := f.sessionCost
	if f.costSnapshot != nil && f.costSnapshot.TotalCost > 0 {
		costVal = f.costSnapshot.TotalCost
	}
	if costVal <= 0 {
		return ""
	}
	if costVal >= 0.01 {
		return fmt.Sprintf("$%.2f", costVal)
	}
	return fmt.Sprintf("$%.4f", costVal)
}

func (f Footer) contextUsagePercent() float64 {
	limit := f.maxInputTokens
	if limit <= 0 {
		limit = f.contextSize
	}
	if f.inputTokens <= 0 || limit <= 0 {
		return 0
	}
	return (float64(f.inputTokens) / float64(limit)) * 100
}

func formatFooterDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d >= time.Minute {
		return fmt.Sprintf("%dm %02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func (f Footer) renderInfoRow(w int, muted lipgloss.Style) string {
	if w <= 2 {
		return ""
	}

	successStyle := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "success", lipgloss.Color("76")))
	accentStyle := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "secondary", lipgloss.Color("141")))
	warnStyle := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "warning", lipgloss.Color("214")))
	errorStyle := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "error", lipgloss.Color("196")))

	var leftParts []string
	if f.projectDir != "" {
		leftParts = append(leftParts, muted.Render(truncateHead(f.projectDir, 28)))
	}
	if f.gitBranch != "" {
		leftParts = append(leftParts, successStyle.Render("⎇ "+f.gitBranch))
	}
	if lspLabel := f.footerLSPStatusText(); lspLabel != "" {
		leftParts = append(leftParts, accentStyle.Render(lspLabel))
	}
	if f.changedFiles > 0 {
		leftParts = append(leftParts, warnStyle.Render(fmt.Sprintf("%d changed", f.changedFiles)))
	}
	if f.pinCount > 0 {
		leftParts = append(leftParts, muted.Render(fmt.Sprintf("📌 %d", f.pinCount)))
	}
	if f.queuedTurns > 0 {
		leftParts = append(leftParts, warnStyle.Render(fmt.Sprintf("◷ queued %d", f.queuedTurns)))
	}
	if f.loopDetected {
		leftParts = append(leftParts, warnStyle.Render("⚠ loop"))
	}
	if f.budgetWarn {
		leftParts = append(leftParts, warnStyle.Render("⚠ budget"))
	}
	if pct := f.contextUsagePercent(); pct >= 80 {
		contextStyle := warnStyle
		if pct >= 90 {
			contextStyle = errorStyle
		}
		leftParts = append(leftParts, contextStyle.Render(fmt.Sprintf("⚠ context %.0f%%", pct)))
	}

	available := w - 2
	leftText := strings.Join(leftParts, "  ")
	maxRightWidth := max(available-lipgloss.Width(leftText)-1, 0)
	rightText := footerShortcutText(maxRightWidth)

	rightWidth := lipgloss.Width(rightText)
	maxLeftWidth := available - rightWidth - 1
	if maxLeftWidth < 0 {
		maxLeftWidth = 0
	}
	if lipgloss.Width(leftText) > maxLeftWidth {
		leftText = ansi.Truncate(leftText, maxLeftWidth, "…")
	}
	leftWidth := lipgloss.Width(leftText)
	gap := max(available-leftWidth-rightWidth, 1)

	return " " + leftText + strings.Repeat(" ", gap) + muted.Render(rightText) + " "
}

func (f Footer) footerLSPStatusText() string {
	if len(f.lspServers) == 0 {
		return ""
	}
	return truncateTail("LSP "+strings.Join(f.lspServers, " + "), 30)
}

func footerShortcutText(maxWidth int) string {
	candidates := []string{
		"⇧↵ newline · ^E expand · ^O editor · ^R history",
		"⇧↵ newline · ^E expand · ^R history",
		"^E expand · ^R history",
		"",
	}
	for _, candidate := range candidates {
		if lipgloss.Width(candidate) <= maxWidth {
			return candidate
		}
	}
	return ""
}

func truncateHead(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	if maxWidth == 1 {
		return "…"
	}
	keep := maxWidth - 1
	if keep >= len(runes) {
		return s
	}
	return "…" + string(runes[len(runes)-keep:])
}

func truncateTail(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	runes := []rune(s)
	if maxWidth == 1 {
		return "…"
	}
	keep := maxWidth - 1
	if keep >= len(runes) {
		return s
	}
	return string(runes[:keep]) + "…"
}
