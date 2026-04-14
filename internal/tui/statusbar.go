package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

const statusBarHeight = 0

type MCPServerStatus struct {
	Name    string
	Active  bool
	Enabled bool
}

// StatusBar renders a 1-row information bar below the footer showing
// persistent session state such as git, MCP, queueing, and warnings.
type StatusBar struct {
	width            int
	theme            *theme.Theme
	toolCount        int
	pinCount         int
	queuedTurns      int
	mcpServers       []MCPServerStatus
	lspServers       []string
	changedFiles     int
	inputTokens      int
	outputTokens     int
	reasoningTokens  int
	cacheReadTokens  int
	cacheWriteTokens int
	maxInputTokens   int
	maxOutputTokens  int
	contextSize      int
	budgetWarn       bool
	toolLoopStep     int
	streaming        bool
	compacting       bool
	loopDetected     bool
	gitBranch        string
	sessionCost      float64
	subagentCost     float64
	costSnapshot     *CostSnapshotPayload
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

func (s *StatusBar) SetQueuedTurns(n int) { s.queuedTurns = n }

func (s *StatusBar) SetMCPServers(servers []MCPServerStatus) { s.mcpServers = servers }

func (s *StatusBar) SetLSPServers(names []string) { s.lspServers = append([]string(nil), names...) }

func (s *StatusBar) SetChangedFiles(n int) { s.changedFiles = n }

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

func (s StatusBar) contextUsagePercent() float64 {
	limit := s.maxInputTokens
	if limit <= 0 {
		limit = s.contextSize
	}
	if s.inputTokens <= 0 || limit <= 0 {
		return 0
	}
	return (float64(s.inputTokens) / float64(limit)) * 100
}

func (s StatusBar) lspStatusText() string {
	if len(s.lspServers) == 0 {
		return ""
	}
	label := "LSP " + strings.Join(s.lspServers, " + ")
	return truncateTail(label, 32)
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
	warnColor := colorFrom(s.theme, "warning", lipgloss.Color("214"))
	accentColor := colorFrom(s.theme, "secondary", lipgloss.Color("141"))

	var parts []string

	if s.gitBranch != "" {
		gitStyle := lipgloss.NewStyle().Foreground(activeGreen)
		parts = append(parts, gitStyle.Render("⎇ "+s.gitBranch))
	}

	if lspStatus := s.lspStatusText(); lspStatus != "" {
		lspStyle := lipgloss.NewStyle().Foreground(accentColor)
		parts = append(parts, lspStyle.Render(lspStatus))
	}

	if s.changedFiles > 0 {
		changeStyle := lipgloss.NewStyle().Foreground(warnColor)
		label := fmt.Sprintf("%d changed", s.changedFiles)
		parts = append(parts, changeStyle.Render(label))
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

	if s.budgetWarn {
		warnStyle := lipgloss.NewStyle().Foreground(warnColor)
		parts = append(parts, warnStyle.Render("⚠ budget"))
	}

	if pct := s.contextUsagePercent(); pct >= 80 {
		contextColor := warnColor
		if pct >= 90 {
			contextColor = errorColor
		}
		contextStyle := lipgloss.NewStyle().Foreground(contextColor)
		parts = append(parts, contextStyle.Render(fmt.Sprintf("⚠ context %.0f%%", pct)))
	}

	if s.loopDetected {
		warnStyle := lipgloss.NewStyle().Foreground(warnColor)
		parts = append(parts, warnStyle.Render("⚠ loop detected"))
	}

	if s.pinCount > 0 {
		pinStyle := lipgloss.NewStyle().Foreground(dimColor)
		parts = append(parts, pinStyle.Render(fmt.Sprintf("📌 %d", s.pinCount)))
	}
	if s.queuedTurns > 0 {
		queueColor := colorFrom(s.theme, "warning", lipgloss.Color("214"))
		queueStyle := lipgloss.NewStyle().Foreground(queueColor)
		parts = append(parts, queueStyle.Render(fmt.Sprintf("◷ queued %d", s.queuedTurns)))
	}

	left := strings.Join(parts, "  ")
	line := " " + left + " "
	if lipgloss.Width(line) > w {
		line = ansi.Truncate(line, w, "")
	}

	return lipgloss.NewStyle().
		Width(w).
		Render(line)
}
