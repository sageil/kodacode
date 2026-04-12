package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Messages) renderReasoning(reasoning string, boxWidth int, msg Message) string {
	thoughtStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "thinking", lipgloss.Color("141"))).Italic(true)
	faintStyle := lipgloss.NewStyle().Faint(true)

	var sb strings.Builder

	if !msg.ReasoningDone {
		wave := thoughtStyle.Render(dashWaveFrames(pulseTick))
		elapsed := ""
		if !msg.ReasoningStartTime.IsZero() {
			secs := int(time.Since(msg.ReasoningStartTime).Seconds())
			elapsed = " " + faintStyle.Render(fmt.Sprintf("%ds", secs))
		}
		sb.WriteString("  " + wave + " " + thoughtStyle.Render("Thinking") + elapsed + "\n")
		lines := strings.Split(strings.TrimRight(reasoning, "\n"), "\n")
		if len(lines) > 0 {
			lastLine := stripMarkdownInline(strings.TrimSpace(lines[len(lines)-1]))
			maxW := max(boxWidth-6, 1)
			if len(lastLine) > maxW {
				lastLine = lastLine[:maxW-1] + "…"
			}
			sb.WriteString("    " + faintStyle.Render(lastLine) + "\n")
		}
	} else if msg.ReasoningCollapsed {
		dur := formatThinkingDuration(msg.ReasoningElapsed)
		summary := reasoningSummaryLine(reasoning)
		header := "  " + thoughtStyle.Render("▸ Thought") + " " + faintStyle.Render("for "+dur)
		if summary != "" {
			maxW := max(boxWidth-len("  ▸ Thought for "+dur+" — ")-2, 10)
			if len(summary) > maxW {
				summary = summary[:maxW-1] + "…"
			}
			header += faintStyle.Render(" — " + summary)
		}
		sb.WriteString(header + "\n")
	} else {
		dur := formatThinkingDuration(msg.ReasoningElapsed)
		summary := reasoningSummaryLine(reasoning)
		header := "  " + thoughtStyle.Render("▾ Thought") + " " + faintStyle.Render("for "+dur)
		if summary != "" {
			maxW := max(boxWidth-len("  ▾ Thought for "+dur+" — ")-2, 10)
			if len(summary) > maxW {
				summary = summary[:maxW-1] + "…"
			}
			header += faintStyle.Render(" — " + summary)
		}
		sb.WriteString(header + "\n")
		wrapWidth := max(boxWidth-6, 1)
		borderStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "border", lipgloss.Color("8")))
		prefix := "    " + borderStyle.Render("│") + " "
		for _, line := range strings.Split(strings.TrimRight(reasoning, "\n"), "\n") {
			line = stripMarkdownInline(line)
			wrapped := ansi.Wrap(line, wrapWidth, "")
			for _, wl := range strings.Split(wrapped, "\n") {
				sb.WriteString(prefix + faintStyle.Render(wl) + "\n")
			}
		}
	}

	return sb.String()
}

func formatThinkingDuration(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 1 {
		return "<1s"
	}
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	return fmt.Sprintf("%dm%ds", secs/60, secs%60)
}

func reasoningSummaryLine(reasoning string) string {
	lines := strings.Split(strings.TrimSpace(reasoning), "\n")
	if len(lines) == 0 {
		return ""
	}
	first := strings.TrimSpace(lines[0])
	first = strings.TrimLeft(first, "- #>")
	first = stripMarkdownInline(first)
	return strings.TrimSpace(first)
}

func stripMarkdownInline(s string) string {
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	s = strings.ReplaceAll(s, "~~", "")
	if len(s) > 1 && s[0] == '*' && s[len(s)-1] == '*' {
		s = s[1 : len(s)-1]
	}
	if len(s) > 1 && s[0] == '_' && s[len(s)-1] == '_' {
		s = s[1 : len(s)-1]
	}
	if len(s) > 1 && s[0] == '`' && s[len(s)-1] == '`' {
		s = s[1 : len(s)-1]
	}
	return s
}
