package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func renderToneBlock(th *theme.Theme, token string, width, height int, content string) string {
	width = max(width, 1)
	height = max(height, 1)
	content = cropBlockHeight(content, height)
	content = cropBlockWidth(content, width)

	bg := toneValue(th, token)
	style := lipgloss.NewStyle().
		Width(width).
		Height(height)
	if strings.TrimSpace(bg) != "" {
		content = persistBackgroundANSI(content, bg)
		style = style.Background(lipgloss.Color(bg))
	}
	return style.Render(content)
}

func lineTone(m Model) string {
	return toneValue(m.theme, toneLine)
}

func shellColumnDivider(m Model, height int, colors ...string) string {
	fg := lineTone(m)
	if len(colors) > 0 && colors[0] != "" {
		fg = colors[0]
	}
	style := lipgloss.NewStyle().
		Width(1).
		Height(max(height, 1)).
		Foreground(lipgloss.Color(fg))
	if len(colors) > 1 && colors[1] != "" {
		style = style.Background(lipgloss.Color(colors[1]))
	}
	return style.Render(strings.Repeat("│\n", max(height, 1)-1) + "│")
}

func wideColumnDivider(m Model, height int, colors ...string) string {
	fg := lineTone(m)
	if len(colors) > 0 && colors[0] != "" {
		fg = colors[0]
	}
	style := lipgloss.NewStyle().
		Width(1).
		Height(max(height, 1)).
		Foreground(lipgloss.Color(fg))
	if len(colors) > 1 && colors[1] != "" {
		style = style.Background(lipgloss.Color(colors[1]))
	}
	return style.Render(strings.Repeat("║\n", max(height, 1)-1) + "║")
}

func fillBackground(width int, bg string, content string) string {
	return renderPersistentBackground(width, bg, content)
}

func renderPersistentBackground(width int, bg string, content string) string {
	style := lipgloss.NewStyle().
		Width(max(width, 1)).
		Background(lipgloss.Color(bg))
	return style.Render(persistBackgroundANSI(content, bg))
}

func placeBlock(width, height int, bg string, content string) string {
	width = max(width, 1)
	height = max(height, 1)
	content = cropBlockHeight(content, height)
	content = cropBlockWidth(content, width)
	lines := strings.Split(content, "\n")

	hasBG := strings.TrimSpace(bg) != ""
	for i, line := range lines {
		pad := width - ansi.StringWidth(ansi.Strip(line))
		if pad > 0 {
			fill := strings.Repeat(" ", pad)
			if hasBG {
				fill = lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(fill)
			}
			lines[i] = line + fill
		}
	}
	for len(lines) < height {
		fill := strings.Repeat(" ", width)
		if hasBG {
			fill = lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(fill)
		}
		lines = append(lines, fill)
	}
	return strings.Join(lines, "\n")
}

func cropBlockHeight(content string, height int) string {
	if height <= 0 {
		return ""
	}
	lines := strings.Split(content, "\n")
	if len(lines) <= height {
		return content
	}
	return strings.Join(lines[:height], "\n")
}

func cropBlockWidth(content string, width int) string {
	if width <= 0 || content == "" {
		return ""
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "…")
		}
	}
	return strings.Join(lines, "\n")
}

func persistBackgroundANSI(content, bg string) string {
	bgANSI := backgroundANSI(bg)
	if bgANSI == "" {
		return content
	}
	content = strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m"+bgANSI)
	content = strings.ReplaceAll(content, "\x1b[m", "\x1b[m"+bgANSI)
	return content
}

func splitWrappedStyledLines(content string, width int) []string {
	wrapped := ansi.Wrap(content, max(width, 1), "")
	wrapped = strings.TrimRight(wrapped, "\n")
	if wrapped == "" {
		return []string{""}
	}
	return splitStandaloneANSILines(wrapped)
}

func splitStandaloneANSILines(content string) []string {
	if content == "" {
		return []string{""}
	}

	lines := make([]string, 0, strings.Count(content, "\n")+1)
	active := ""
	lineStart := true
	var line strings.Builder

	for i := 0; i < len(content); {
		if seq, next, ok := nextANSIEscapeSequence(content, i); ok {
			line.WriteString(seq)
			if strings.HasSuffix(seq, "m") {
				active = mergeActiveSGRANSI(active, seq)
			}
			i = next
			continue
		}

		if content[i] == '\n' {
			lines = append(lines, line.String())
			line.Reset()
			lineStart = true
			i++
			continue
		}

		if lineStart && active != "" {
			line.WriteString(active)
		}

		line.WriteByte(content[i])
		lineStart = false
		i++
	}

	lines = append(lines, line.String())
	return lines
}

func nextANSIEscapeSequence(content string, start int) (string, int, bool) {
	if start+1 >= len(content) || content[start] != '\x1b' || content[start+1] != '[' {
		return "", start, false
	}

	end := start + 2
	for end < len(content) {
		if content[end] >= 0x40 && content[end] <= 0x7e {
			end++
			return content[start:end], end, true
		}
		end++
	}
	return "", start, false
}

func mergeActiveSGRANSI(active, seq string) string {
	switch seq {
	case "\x1b[m", "\x1b[0m":
		return ""
	default:
		return active + seq
	}
}

func backgroundANSI(bg string) string {
	styled := lipgloss.NewStyle().Background(lipgloss.Color(bg)).Render(" ")
	if before, _, ok := strings.Cut(styled, " "); ok {
		return before
	}
	return ""
}

func lerpColor(a, b string, t float64) string {
	ar, ag, ab, ok1 := parseHexColor(a)
	br, bg, bb, ok2 := parseHexColor(b)
	if !ok1 || !ok2 {
		return a
	}
	r := uint8(float64(ar) + t*float64(int(br)-int(ar)))
	g := uint8(float64(ag) + t*float64(int(bg)-int(ag)))
	bv := uint8(float64(ab) + t*float64(int(bb)-int(ab)))
	return fmt.Sprintf("#%02x%02x%02x", r, g, bv)
}

func sweepRowDivider(char string, width, frame int, primary, dim string) string {
	if width <= 0 {
		return ""
	}
	const trailLen = 10
	period := max((width-1)*2, 2)
	raw := frame % period
	pos := raw
	if pos >= width {
		pos = period - raw
	}
	var sb strings.Builder
	for i := 0; i < width; i++ {
		d := pos - i
		if d < 0 {
			d = -d
		}
		var color string
		switch {
		case d == 0:
			color = primary
		case d <= trailLen:
			color = lerpColor(dim, primary, 1.0-float64(d)/float64(trailLen+1))
		default:
			color = dim
		}
		sb.WriteString(lipgloss.NewStyle().
			Foreground(lipgloss.Color(color)).
			Render(char))
	}
	return sb.String()
}

func parseHexColor(s string) (r, g, b uint8, ok bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return 0, 0, 0, false
	}
	val, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return uint8(val >> 16), uint8(val >> 8), uint8(val), true
}
