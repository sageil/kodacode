package tui

import "github.com/sageil/kodacode/internal/events"

func shellModeName(m Model) string {
	if m.chrome.focus == focusTranscript && m.transcriptView.visualActive {
		return "VISUAL"
	}
	switch m.chrome.focus {
	case focusComposer:
		return "INSERT"
	case focusInspector:
		return "SELECT"
	default:
		return "NORMAL"
	}
}

func shellModeColor(m Model) string {
	if m.chrome.focus == focusTranscript && m.transcriptView.visualActive {
		return colorFor(m.theme, "warning", "#ffd28f")
	}
	switch m.chrome.focus {
	case focusComposer:
		return colorFor(m.theme, "success", "#90e5b4")
	case focusInspector:
		return colorFor(m.theme, "warning", "#ffd28f")
	default:
		return colorFor(m.theme, "primary", "#7cc7ff")
	}
}

func shellStatusHints(m Model, state events.SessionState) string {
	if pending := m.pendingQuestion(); pending != nil {
		return questionPanelHints(*pending)
	}
	if m.hasPendingApproval() {
		return "1/2/3 choose · ↑↓ move · enter confirm"
	}
	switch m.chrome.focus {
	case focusInspector:
		if effectiveInspectorTab(m) == inspectorTabTools {
			return "j/k tools · enter details · h/l tabs · i insert · ctrl+l layout · ctrl+\\ drawer · ctrl+n new · pgup/pgdn scroll · ctrl+] normal"
		}
		return "h/l tabs · i insert · ctrl+l layout · ctrl+\\ drawer · ctrl+n new · pgup/pgdn scroll · ctrl+] normal"
	case focusComposer:
		hints := "enter submit · shift+enter nl · ctrl+w workflow · ctrl+e edit · ↑/↓ recall"
		if shellLayoutEnabled(m) {
			hints += " · ctrl+t tools"
		}
		hints += " · ctrl+n new · ctrl+\\ drawer · pgup/pgdn"
		if !m.currentTurnRunning() {
			hints += " · tab agent"
		}
		return hints + " · ctrl+] normal"
	default:
		if m.transcriptView.visualActive {
			return "y copy · v/esc cancel · h/l chars · ↑↓ lines · 0/$ line · home/end doc"
		}
		if shellLayoutEnabled(m) {
			toolHint := "t tools · T inline tools"
			if selectedTranscriptToolAvailable(m, state) && m.shellToolCallsVisible {
				toolHint = "enter expand · " + toolHint
			}
			return toolHint + " · h/l chars · v select · i insert · ctrl+l layout · ctrl+n new"
		}
		return "drag/v select · h/l chars · ↑↓ lines · j/k tools · i insert · ctrl+l layout · ctrl+\\ drawer · ctrl+n new"
	}
}

func selectedTranscriptToolAvailable(m Model, state events.SessionState) bool {
	_, _, _, _, ok := selectedSessionToolCall(state, m)
	return ok
}
