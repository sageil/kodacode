package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// TestBoxBgWidth verifies that a pure-background lipgloss block (no border
// style) rendered at Width(totalWidth) produces lines whose ansi.StringWidth
// equals totalWidth.
func TestBoxBgWidth(t *testing.T) {
	termWidth := 80

	for _, totalWidth := range []int{termWidth, termWidth - 1, termWidth - 2} {
		s := lipgloss.NewStyle().
			Background(lipgloss.Color("#393552")).
			Padding(0, 1).
			Width(totalWidth)

		rendered := s.Render("User\nhello")
		lines := strings.Split(rendered, "\n")
		for i, line := range lines {
			vw := ansi.StringWidth(line)
			if vw != totalWidth {
				t.Errorf("totalWidth=%d line[%d]: got visual width %d, want %d",
					totalWidth, i, vw, totalWidth)
			}
		}
	}
}
