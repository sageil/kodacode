package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
)


func (f Footer) viewFullWidth() string {
	w := max(f.width, 1)
	dimColor := colorFrom(f.theme, "subtext", lipgloss.Color("241"))
	muted := lipgloss.NewStyle().Foreground(dimColor)
	accentColor := colorFrom(f.theme, "secondary", lipgloss.Color("4"))
	accentStyle := lipgloss.NewStyle().Foreground(accentColor)

	// Blocked: show warning-colored border and placeholder when inline panel is active.
	if f.blocked {
		warnColor := colorFrom(f.theme, "warning", lipgloss.Color("214"))
		warnStyle := lipgloss.NewStyle().Foreground(warnColor)
		border := warnStyle.Render(strings.Repeat("─", w))
		placeholder := warnStyle.Render("  ⚠ respond to the panel above…")
		return border + "\n" + placeholder
	}

	// Thin separator line above the input — pulses when streaming.
	var border string
	if f.errorFlash {
		errStyle := lipgloss.NewStyle().Foreground(colorFrom(f.theme, "error", lipgloss.Color("196")))
		border = errStyle.Render(strings.Repeat("─", w))
	} else if f.streaming {
		phase := pulseTick % 16
		style := accentStyle
		if phase >= 8 {
			style = style.Faint(true)
		}
		border = style.Render(strings.Repeat("─", w))
	} else {
		border = muted.Render(strings.Repeat("─", w))
	}

	// Progress indicator: step dots + timer (only when streaming).
	var progressView string
	if f.streaming {
		var sb strings.Builder
		sb.WriteString(" ")

		// Step indicator: show up to 10 dots, then just the count.
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

		// Elapsed timer.
		if !f.streamStartTime.IsZero() {
			elapsed := time.Since(f.streamStartTime).Truncate(time.Second)
			var timerStr string
			if elapsed >= time.Minute {
				timerStr = fmt.Sprintf("%dm %02ds", int(elapsed.Minutes()), int(elapsed.Seconds())%60)
			} else {
				timerStr = fmt.Sprintf("%ds", int(elapsed.Seconds()))
			}
			// Right-align the timer.
			leftWidth := lipgloss.Width(sb.String()) + 3
			gap := max(w-leftWidth-len(timerStr)-1, 1)
			sb.WriteString(strings.Repeat(" ", gap) + accentStyle.Render(timerStr))
		}
		sb.WriteString(" ")
		progressView = sb.String() + "\n"
	}

	inputView := f.input.View()

	// History search overlay.
	var searchView string
	if f.historySearch {
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

	return f.renderSlashCompletion() + progressView + border + "\n" + searchView + inputView
}

func (f Footer) viewBoxed() string {
	borderColor := colorFrom(f.theme, "primary", lipgloss.Color("62"))

	inputView := f.input.View()

	// History search overlay.
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
		Width(f.boxWidth - 2). // lipgloss Width sets inner content width
		Render(searchView + inputView)

	return f.renderSlashCompletion() + box
}
