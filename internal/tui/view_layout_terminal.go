package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
)

func renderWideTerminalPane(m Model, state events.SessionState, width int) string {
	var backgroundCall *events.ToolCallState
	var executingCall *events.ToolCallState
	for _, ref := range orderedSessionToolCallRefs(state) {
		_, call := sessionToolCall(state, ref)
		if call != nil && call.Execution != nil && call.Execution.Background != nil {
			switch call.Execution.Background.Status {
			case events.ExecutionBackgroundStatusStarting, events.ExecutionBackgroundStatusRunning, events.ExecutionBackgroundStatusReady, events.ExecutionBackgroundStatusSupervisionLost:
				backgroundCall = call
			}
		}
		if call != nil && call.Executing {
			executingCall = call
		}
	}
	latestCall := latestTerminalCommandCall(state)
	if backgroundCall != nil {
		executingCall = backgroundCall
	}
	if executingCall == nil {
		executingCall = latestCall
	}

	subtextColor := colorFor(m.theme, "subtext", "#9da8ca")
	commandTitle := "idle"

	if executingCall == nil {
		idle := lipgloss.NewStyle().
			Foreground(lipgloss.Color(subtextColor)).
			Render("No active terminal command.")
		header := renderWidePaneTitle(m, "Terminal", commandTitle, width, colorFor(m.theme, "secondary", "#8be9fd"))
		prompt := lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorFor(m.theme, "secondary", "#8be9fd"))).
			Render("$")
		return header + "\n" + prompt + " " + idle
	}

	inputPreview := truncateEnd(strings.TrimSpace(commandToolDisplayName(state.WorkspaceRoot, executingCall)), max(width, 4))
	commandTitle = prettyToolName(strings.TrimSpace(executingCall.ToolName))
	if commandTitle == "" || commandTitle == "Tool" {
		commandTitle = "Command"
	}
	if inputPreview != "" && inputPreview != strings.TrimSpace(executingCall.ToolName) {
		commandTitle += " · " + inputPreview
	}
	header := renderWidePaneTitle(m, "Terminal", commandTitle, width, colorFor(m.theme, "secondary", "#8be9fd"))

	spinnerFrames := []string{"⠋", "⠙", "⠸", "⠴", "⠦", "⠇"}
	spinner := spinnerFrames[m.animation.frame%len(spinnerFrames)]
	inputLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color(subtextColor)).
		Render("$ " + inputPreview)

	outputText := terminalOutputText(executingCall)
	outputLines := strings.Split(outputText, "\n")
	start := max(len(outputLines)-4, 0)
	outParts := make([]string, 0, 4)
	statusTone, statusText := terminalStatusLine(m, executingCall, spinner)
	statusLine := lipgloss.NewStyle().
		Foreground(lipgloss.Color(statusTone)).
		Render(statusText)
	outParts = append(outParts, statusLine)
	for _, line := range outputLines[start:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		outParts = append(outParts, truncateEnd(line, max(width, 4)))
	}
	if len(outParts) == 1 {
		outParts = append(outParts, lipgloss.NewStyle().
			Foreground(lipgloss.Color(subtextColor)).
			Render("(no terminal output yet)"))
	}
	outputBlock := lipgloss.NewStyle().
		Width(max(width, 1)).
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
		Render(strings.Join(outParts, "\n"))

	return header + "\n" + inputLine + "\n" + outputBlock
}

func terminalOutputText(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if call.Execution != nil && call.Execution.Background != nil && strings.TrimSpace(call.Execution.Background.OutputTail) != "" {
		return strings.TrimSpace(call.Execution.Background.OutputTail)
	}
	return strings.TrimSpace(call.Output)
}

func terminalStatusLine(m Model, call *events.ToolCallState, spinner string) (string, string) {
	if call != nil && call.Execution != nil && call.Execution.Background != nil {
		background := call.Execution.Background
		switch background.Status {
		case events.ExecutionBackgroundStatusStarting:
			return colorFor(m.theme, "warning", "#ffb86c"), spinner + " starting"
		case events.ExecutionBackgroundStatusRunning:
			return colorFor(m.theme, "warning", "#ffb86c"), spinner + " running"
		case events.ExecutionBackgroundStatusReady:
			return colorFor(m.theme, "success", "#50fa7b"), terminalIcon(terminalIconSelected) + " ready"
		case events.ExecutionBackgroundStatusSupervisionLost:
			return colorFor(m.theme, "error", "#ff5555"), terminalIcon(terminalIconToolError) + " supervision lost"
		case events.ExecutionBackgroundStatusExited:
			if strings.TrimSpace(background.Error) != "" || (background.ExitCode != nil && *background.ExitCode != 0) {
				return colorFor(m.theme, "error", "#ff5555"), terminalIcon(terminalIconToolError) + " exited"
			}
			return colorFor(m.theme, "success", "#50fa7b"), terminalIcon(terminalIconCheck) + " exited"
		}
	}
	statusTone := colorFor(m.theme, "warning", "#ffb86c")
	statusText := spinner + " running"
	if call != nil && call.Completed && strings.TrimSpace(call.Error) != "" {
		statusTone = colorFor(m.theme, "error", "#ff5555")
		statusText = terminalIcon(terminalIconToolError) + " error"
	} else if call != nil && call.Completed {
		statusTone = colorFor(m.theme, "success", "#50fa7b")
		statusText = terminalIcon(terminalIconCheck) + " done"
	}
	return statusTone, statusText
}
