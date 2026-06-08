package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func (d *commandPaletteDialog) renderButtons() string {
	buttons := d.activeButtons()
	if len(buttons) == 0 {
		return ""
	}
	rendered := make([]string, 0, len(buttons))
	buttonFocus := d.focusedButtonIndex()
	for idx, button := range buttons {
		rendered = append(rendered, renderPaletteButton(d.theme, button.label, idx == buttonFocus))
	}
	return strings.Join(rendered, "  ")
}

func renderPaletteButton(th *theme.Theme, label string, focused bool) string {
	focusedStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7")))
	blurredStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(th, "soft", "#565f89")))

	if focused {
		return focusedStyle.Render("[ " + label + " ]")
	}
	return "[ " + blurredStyle.Render(label) + " ]"
}

func relativeTimeUnix(unixSeconds int64) string {
	if unixSeconds <= 0 {
		return ""
	}
	now := time.Now()
	updatedAt := time.Unix(unixSeconds, 0)
	if updatedAt.IsZero() {
		return ""
	}
	elapsed := now.Sub(updatedAt)
	switch {
	case elapsed < time.Minute:
		return "now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh", int(elapsed.Hours()))
	case elapsed < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(elapsed.Hours()/24))
	default:
		return updatedAt.Format("2006-01-02")
	}
}
