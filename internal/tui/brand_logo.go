package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/tui/theme"
)

var brandLogoLines = []string{
	"  _  __         _          ____          _      ",
	" | |/ /___   __| | __ _   / ___|___   __| | ___ ",
	" | ' // _ \\ / _` |/ _` | | |   / _ \\ / _` |/ _ \\",
	" | . \\ (_) | (_| | (_| | | |__| (_) | (_| |  __/",
	" |_|\\_\\___/ \\__,_|\\__,_|  \\____\\___/ \\__,_|\\___|",
}

const (
	brandLogoWidth     = 50
	brandLogoShimWidth = 8
)

// renderBrandLogo ports the KodaCode home wordmark shimmer so other entry
// surfaces can reuse the same branding without duplicating color logic.
func renderBrandLogo(th *theme.Theme, shimCol int) string {
	bg := lipgloss.Color(toneValue(th, toneBG))
	primary := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(th, "primary", "#7aa2f7"))).
		Background(bg)
	highlight := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorFor(th, "secondary", "#7dcfff"))).
		Background(bg)

	lines := make([]string, 0, len(brandLogoLines))
	for _, line := range brandLogoLines {
		runes := []rune(line)
		var sb strings.Builder
		for col, ch := range runes {
			rendered := primary
			if col >= shimCol && col < shimCol+brandLogoShimWidth {
				rendered = highlight
			}
			sb.WriteString(rendered.Render(string(ch)))
		}
		lines = append(lines, sb.String())
	}
	return strings.Join(lines, "\n")
}
