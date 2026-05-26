package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

func lerpHexColor(a, b string, t float64) string {
	ar, ag, ab := parseHex(a)
	br, bg, bb := parseHex(b)
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	r := ar + int(float64(br-ar)*t)
	g := ag + int(float64(bg-ag)*t)
	bl := ab + int(float64(bb-ab)*t)
	return fmt.Sprintf("#%02x%02x%02x", clamp8(r), clamp8(g), clamp8(bl))
}

func parseHex(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	r, _ := strconv.ParseInt(hex[0:2], 16, 32)
	g, _ := strconv.ParseInt(hex[2:4], 16, 32)
	b, _ := strconv.ParseInt(hex[4:6], 16, 32)
	return int(r), int(g), int(b)
}

func clamp8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

const (
	shellBackgroundColor    = "#0b1020"
	shellBackgroundAltColor = "#121933"
	panelSurfaceColor       = "#141d38"
	panelSurfaceAltColor    = "#0f162b"
	lineColor               = "#2a3356"
	lineStrongColor         = "#3a4670"
	softTextColor           = "#c9d0e6"
)

const (
	toneBG         = "bg"
	toneBGAlt      = "bg-alt"
	tonePanel      = "panel"
	tonePanelAlt   = "panel-alt"
	toneLine       = "line"
	toneLineStrong = "line-strong"
	toneSoft       = "soft"
)

func terminalWidth(m Model) int {
	return max(m.width, 1)
}

func renderStatus(m Model, state events.SessionState) string {
	if m.pendingInteractionSubmissionInFlight() {
		return "resuming"
	}
	if pending := m.pendingExecution(); pending != nil {
		return "execution approval"
	}
	if pending := m.pendingPermission(); pending != nil {
		return "pending permission"
	}
	if pending := m.pendingQuestion(); pending != nil {
		return "pending question"
	}
	turn := state.Turns[m.turnID]
	if turn == nil {
		if m.busy {
			return "starting"
		}
		return "idle"
	}
	if m.busy && turn.Status == events.TurnStatusRunning {
		return "running"
	}
	return string(turn.Status)
}

func renderStatusForTurn(m Model, state events.SessionState, turnID string) string {
	if strings.TrimSpace(turnID) == "" {
		return "idle"
	}
	turn := state.Turns[turnID]
	if turn == nil {
		if m.busy && turnID == m.turnID {
			return "starting"
		}
		return "idle"
	}
	if m.busy && turnID == m.turnID && turn.Status == events.TurnStatusRunning {
		return "running"
	}
	return string(turn.Status)
}

func pickWorkspace(workspaceRoot, fallback string) string {
	if workspaceRoot != "" {
		return workspaceRoot
	}
	return fallback
}

func toolStatus(call *events.ToolCallState) string {
	if call != nil && call.Execution != nil && call.Execution.Background != nil {
		background := call.Execution.Background
		switch background.Status {
		case events.ExecutionBackgroundStatusStarting, events.ExecutionBackgroundStatusRunning, events.ExecutionBackgroundStatusReady:
			return "running"
		case events.ExecutionBackgroundStatusSupervisionLost:
			return "error"
		case events.ExecutionBackgroundStatusExited:
			if strings.TrimSpace(background.Error) != "" {
				return "error"
			}
			if background.ExitCode != nil && *background.ExitCode != 0 {
				return "error"
			}
			return "done"
		}
	}
	switch {
	case call.Completed && toolCallHasErrorResult(call):
		return "error"
	case call.Completed:
		return "done"
	case call.Executing:
		return "running"
	case call.Declared:
		return "declared"
	default:
		return "building"
	}
}

func toolCallHasErrorResult(call *events.ToolCallState) bool {
	if call == nil {
		return false
	}
	return strings.TrimSpace(call.Error) != "" ||
		(call.ErrorBlob != nil && strings.TrimSpace(call.ErrorBlob.Ref) != "") ||
		call.ErrorDetail != nil ||
		strings.TrimSpace(call.FailureClass) != ""
}

func colorFor(th *theme.Theme, token, fallback string) string {
	if th == nil {
		return fallback
	}
	if resolved := th.PaletteToken(token); resolved != "" {
		return resolved
	}
	return fallback
}

func summarizeBlock(text string) string {
	const maxLines = 8

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[:maxLines], "\n") + "\n..."
}

func toneValue(th *theme.Theme, token string) string {
	if strings.TrimSpace(token) == "" || token == "none" {
		return ""
	}
	if th != nil {
		if resolved := th.ToneToken(token); resolved != "" {
			return resolved
		}
	}
	switch token {
	case toneBG:
		return colorFor(th, "surface", shellBackgroundColor)
	case toneBGAlt:
		return colorFor(th, "surface", shellBackgroundAltColor)
	case tonePanel:
		return colorFor(th, "overlay", panelSurfaceColor)
	case tonePanelAlt:
		return colorFor(th, "surface", panelSurfaceAltColor)
	case toneLine:
		return colorFor(th, "overlay", lineColor)
	case toneLineStrong:
		return colorFor(th, "overlay", lineStrongColor)
	case toneSoft:
		return colorFor(th, "subtext", softTextColor)
	default:
		return colorFor(th, token, shellBackgroundColor)
	}
}

func toneFillStyle(th *theme.Theme, token string) lipgloss.Style {
	bg := toneValue(th, token)
	style := lipgloss.NewStyle()
	if strings.TrimSpace(bg) == "" {
		return style
	}
	return style.Background(lipgloss.Color(bg))
}
