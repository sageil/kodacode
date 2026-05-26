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
			return "j/k tools · enter details · h/l tabs · i insert · ctrl+\\ drawer · ctrl+s sessions · ctrl+n new · pgup/pgdn scroll · ctrl+] normal"
		}
		return "h/l tabs · i insert · ctrl+\\ drawer · ctrl+s sessions · ctrl+n new · pgup/pgdn scroll · ctrl+] normal"
	case focusComposer:
		hints := "enter submit · shift+enter nl · ctrl+e edit · ↑/↓ recall · ctrl+s sessions · ctrl+n new · ctrl+\\ drawer · pgup/pgdn"
		if !m.currentTurnRunning() {
			hints += " · tab agent"
		}
		return hints + " · ctrl+] normal"
	default:
		if m.transcriptView.visualActive {
			return "y copy · v/esc cancel · h/l chars · ↑↓ lines · 0/$ line · home/end doc"
		}
		return "drag/v select · h/l chars · ↑↓ lines · j/k tools · {/]} handoffs · i insert · ctrl+\\ drawer · ctrl+s sessions · ctrl+n new"
	}
}
