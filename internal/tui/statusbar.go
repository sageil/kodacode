package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

const statusBarHeight = 1

type MCPServerStatus struct {
	Name    string
	Active  bool
	Enabled bool
}

// StatusBar renders a 1-row information bar below the footer showing
// git branch, connected MCP servers, tool count, and keyboard shortcuts.
type StatusBar struct {
	width          int
	theme          *theme.Theme
	toolCount      int
	pinCount       int
	mcpServers     []MCPServerStatus
	inputTokens      int
	outputTokens     int
	reasoningTokens  int
	cacheReadTokens  int
	cacheWriteTokens int
	maxInputTokens   int
	maxOutputTokens  int
	contextSize      int
	budgetWarn       bool
	toolLoopStep   int
	streaming      bool
	compacting     bool
	loopDetected   bool
	gitBranch      string
	sessionCost    float64
	subagentCost   float64
	costSnapshot   *CostSnapshotPayload
}

func NewStatusBar() StatusBar { return StatusBar{} }

func formatTokenCount(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		v := float64(n) / 1000
		if v == float64(int(v)) {
			return fmt.Sprintf("%dk", int(v))
		}
		return fmt.Sprintf("%.1fk", v)
	}
	return fmt.Sprintf("%d", n)
}

func (s *StatusBar) ApplyTheme(t *theme.Theme) { s.theme = t }

func (s *StatusBar) SetSize(w int) { s.width = w }

func (s *StatusBar) SetToolCount(n int) { s.toolCount = n }

func (s *StatusBar) SetPinCount(n int) { s.pinCount = n }

func (s *StatusBar) SetMCPServers(servers []MCPServerStatus) { s.mcpServers = servers }

func (s *StatusBar) SetToolLoopStep(n int) { s.toolLoopStep = n }

func (s *StatusBar) SetStreaming(v bool) {
	s.streaming = v
	if !v {
		s.loopDetected = false
	}
}

func (s *StatusBar) SetCompacting(v bool)   { s.compacting = v }
func (s *StatusBar) SetLoopDetected(v bool) { s.loopDetected = v }

func (s *StatusBar) SetGitBranch(branch string) { s.gitBranch = branch }

func (s *StatusBar) SetSessionCost(cost, subagentCost float64) {
	s.sessionCost = cost
	s.subagentCost = subagentCost
}

func (s *StatusBar) SetTokenBreakdown(reasoning, cacheRead, cacheWrite int) {
	s.reasoningTokens = reasoning
	s.cacheReadTokens = cacheRead
	s.cacheWriteTokens = cacheWrite
}

func (s *StatusBar) SetBudgetWarning(v bool) { s.budgetWarn = v }

func (s *StatusBar) SetCostSnapshot(snap *CostSnapshotPayload) { s.costSnapshot = snap }

func (s *StatusBar) SetTokens(inputTokens, outputTokens, contextSize, maxInputTokens, maxOutputTokens int) {
	s.inputTokens = inputTokens
	s.outputTokens = outputTokens
	s.contextSize = contextSize
	if maxInputTokens > 0 && maxInputTokens < contextSize {
		s.maxInputTokens = maxInputTokens
		s.maxOutputTokens = contextSize - maxInputTokens
	} else {
		s.maxInputTokens = contextSize
		s.maxOutputTokens = maxOutputTokens
	}
}

// tokenColor returns a theme-consistent color based on token usage ratio:
// < 80% → subtext (dim), 80-90% → warning, ≥ 90% → error.
func (s StatusBar) tokenColor() color.Color {
	if s.maxInputTokens <= 0 {
		return colorFrom(s.theme, "subtext", lipgloss.Color("241"))
	}
	pct := float64(s.inputTokens) / float64(s.maxInputTokens) * 100
	switch {
	case pct >= 90:
		return colorFrom(s.theme, "error", lipgloss.Color("196"))
	case pct >= 80:
		return colorFrom(s.theme, "warning", lipgloss.Color("214"))
	default:
		return colorFrom(s.theme, "subtext", lipgloss.Color("241"))
	}
}

func (s StatusBar) renderStreamingIndicator(label string) string {
	accentColor := colorFrom(s.theme, "secondary", lipgloss.Color("141"))
	brightColor := colorFrom(s.theme, "primary", lipgloss.Color("255"))
	dimColor := colorFrom(s.theme, "subtext", lipgloss.Color("241"))

	accentStyle := lipgloss.NewStyle().Foreground(accentColor)
	brightStyle := lipgloss.NewStyle().Foreground(brightColor)
	midStyle := accentStyle
	dimLabelStyle := lipgloss.NewStyle().Foreground(dimColor)

	frames := []rune(spinnerFrames)
	frame := (pulseTick / 2) % int64(len(frames))
	icon := string(frames[frame])

	wavePos := int(pulseTick/3) % (len(label) + 4)

	var sb strings.Builder
	sb.WriteString(accentStyle.Render(icon))
	sb.WriteByte(' ')
	for i, ch := range label {
		dist := wavePos - i
		if dist < 0 {
			dist = -dist
		}
		switch {
		case dist == 0:
			sb.WriteString(brightStyle.Render(string(ch)))
		case dist <= 2:
			sb.WriteString(midStyle.Render(string(ch)))
		default:
			sb.WriteString(dimLabelStyle.Render(string(ch)))
		}
	}
	return sb.String()
}

func (s StatusBar) View() string {
	w := s.width
	if w < 1 {
		w = 80
	}

	dimColor := colorFrom(s.theme, "subtext", lipgloss.Color("241"))
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)
	errorColor := colorFrom(s.theme, "error", lipgloss.Color("196"))
	activeGreen := colorFrom(s.theme, "success", lipgloss.Color("76"))

	var parts []string

	if s.gitBranch != "" {
		gitStyle := lipgloss.NewStyle().Foreground(activeGreen)
		parts = append(parts, gitStyle.Render("⎇ "+s.gitBranch))
	}

	for _, srv := range s.mcpServers {
		var dot string
		nameStyle := dimStyle
		if !srv.Enabled {
			dot = dimStyle.Render("●")
		} else if srv.Active {
			dot = lipgloss.NewStyle().Foreground(activeGreen).Render("●")
			nameStyle = dimStyle
		} else {
			dot = lipgloss.NewStyle().Foreground(errorColor).Render("●")
		}
		parts = append(parts, dot+" "+nameStyle.Render(srv.Name))
	}

	if s.streaming {
		var label string
		switch {
		case s.compacting:
			label = "compacting"
		case s.toolLoopStep > 0:
			label = "working"
		default:
			label = "thinking"
		}
		parts = append(parts, s.renderStreamingIndicator(label))
	}

	if s.loopDetected {
		warnColor := colorFrom(s.theme, "warning", lipgloss.Color("214"))
		warnStyle := lipgloss.NewStyle().Foreground(warnColor)
		parts = append(parts, warnStyle.Render("⚠ loop detected"))
	}

	if s.pinCount > 0 {
		pinStyle := lipgloss.NewStyle().Foreground(dimColor)
		parts = append(parts, pinStyle.Render(fmt.Sprintf("📌 %d", s.pinCount)))
	}

	left := strings.Join(parts, "  ")

	var rightParts []string
	if s.sessionCost > 0 {
		costStr := fmt.Sprintf("$%.4f", s.sessionCost)
		if s.sessionCost >= 0.01 {
			costStr = fmt.Sprintf("$%.2f", s.sessionCost)
		}
		costStyle := dimStyle
		if s.budgetWarn {
			warnColor := colorFrom(s.theme, "warning", lipgloss.Color("214"))
			costStyle = lipgloss.NewStyle().Foreground(warnColor)
			costStr = "⚠ " + costStr
		}
		rightParts = append(rightParts, costStyle.Render(costStr))
	}
	if s.inputTokens > 0 {
		inputStr := formatTokenCount(s.inputTokens)
		if s.maxInputTokens > 0 {
			inputStr += "/" + formatTokenCount(s.maxInputTokens)
		}
		inputStr += "↑"
		tokenStyle := lipgloss.NewStyle().Foreground(s.tokenColor())
		rightParts = append(rightParts, tokenStyle.Render(inputStr))

		if s.outputTokens > 0 || s.maxOutputTokens > 0 {
			outStr := formatTokenCount(s.outputTokens)
			if s.maxOutputTokens > 0 {
				outStr += "/" + formatTokenCount(s.maxOutputTokens)
			}
			outStr += "↓"
			outStyle := lipgloss.NewStyle().Foreground(dimColor)
			rightParts = append(rightParts, outStyle.Render(outStr))
		}
	}
	right := strings.Join(rightParts, "  ")

	leftWidth := lipgloss.Width(left)
	rightWidth := lipgloss.Width(right)
	gap := max(w-leftWidth-rightWidth-2, 1)

	line := " " + left + strings.Repeat(" ", gap) + right + " "

	return lipgloss.NewStyle().
		Width(w).
		Render(line)
}
