package tui

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func toolDetailDialogMarkdownBody(m Model, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	return toolDetailDialogMarkdownBodyForSession(m, m.sessionID, state, ref, call)
}

func toolDetailDialogMarkdownBodyForSession(m Model, sessionID string, state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	switch {
	case call == nil:
		return ""
	case isCommandToolCall(call):
		return renderCommandToolDetailMarkdownForSession(m, sessionID, ref, call)
	case isTaskToolCall(call):
		return renderTaskToolDetailMarkdownForSession(m, sessionID, ref, call)
	case isDelegateToolCall(call):
		return renderDelegateToolDetailMarkdownForSession(m, sessionID, ref, call)
	case strings.TrimSpace(call.ToolName) == "search_skills":
		output := toolResultOutputForSession(m, sessionID, &ref, call)
		if body, ok := renderSearchSkillsToolDetailMarkdown(call, output); ok {
			return body
		}
		return renderGenericToolDetailMarkdownForSession(m, sessionID, ref, call)
	case strings.TrimSpace(call.ToolName) == "skill":
		output := toolResultOutputForSession(m, sessionID, &ref, call)
		if body, ok := renderSkillToolDetailMarkdown(call, output); ok {
			return body
		}
		return renderGenericToolDetailMarkdownForSession(m, sessionID, ref, call)
	case strings.TrimSpace(call.ToolName) == "question":
		return renderQuestionToolDetailMarkdown(state, ref, call)
	default:
		return renderGenericToolDetailMarkdownForSession(m, sessionID, ref, call)
	}
}

func renderTaskToolDetailMarkdown(m Model, ref sessionToolCallRef, call *events.ToolCallState) string {
	return renderTaskToolDetailMarkdownForSession(m, m.sessionID, ref, call)
}

func renderTaskToolDetailMarkdownForSession(m Model, sessionID string, ref sessionToolCallRef, call *events.ToolCallState) string {
	lines := make([]string, 0, 8)
	errorText := strings.TrimSpace(toolResultErrorForSession(m, sessionID, &ref, call))
	displayError := strings.TrimSpace(taskToolErrorDisplayText(call, errorText))
	if errorText != "" {
		if displayError == "" {
			displayError = errorText
		}
		lines = append(lines, "## Error", toolDetailErrorBlock(displayError))
	}

	output := strings.TrimSpace(toolResultOutputForSession(m, sessionID, &ref, call))
	notice := ""
	if !toolResultBodyLoadedForSession(m, sessionID, &ref) {
		notice = strings.TrimSpace(toolResultPreviewNotice(call))
	}

	switch {
	case output != "":
		if structured, ok := renderTaskToolOutputMarkdown(output); ok {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, structured)
		} else {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "## Output", fencedToolDetailBlock(toolDetailOutputLanguage(call, output), output))
		}
	case errorText == "" && call.Executing:
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Output", fencedToolDetailBlock("text", "(running...)"))
	case errorText == "":
		if placeholder := strings.TrimSpace(toolResultHydrationPlaceholderForSession(m, sessionID, &ref, call)); placeholder != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "## Output", fencedToolDetailBlock("text", "("+placeholder+")"))
			break
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Output", fencedToolDetailBlock("text", "(no output)"))
	}

	if notice != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "_"+notice+"_")
	}
	return strings.Join(lines, "\n")
}

func renderQuestionToolDetailMarkdown(state events.SessionState, ref sessionToolCallRef, call *events.ToolCallState) string {
	input, ok := parseQuestionToolViewInput(call.Input)
	if !ok {
		return renderGenericToolDetailMarkdownForSession(Model{}, state.SessionID, ref, call)
	}

	lines := []string{
		"Question: " + input.Question,
	}
	if strings.TrimSpace(input.Purpose) != "" {
		lines = append(lines, "Purpose: "+input.Purpose)
	}
	if len(input.Options) > 0 {
		lines = append(lines, "", "Options:")
		for idx, option := range input.Options {
			lines = append(lines, fmt.Sprintf("%d. %s", idx+1, option))
		}
	}
	if answer := questionToolAnswer(state, ref, call); answer != "" {
		lines = append(lines, "", "Answer: "+answer)
	}
	if errorText := strings.TrimSpace(call.Error); errorText != "" {
		lines = append(lines, "", "Error:", toolDetailErrorBlock(errorText))
	}
	return strings.Join(lines, "\n")
}

func renderCommandToolDetailMarkdownForSession(m Model, sessionID string, ref sessionToolCallRef, call *events.ToolCallState) string {
	command := strings.TrimSpace(toolDetailCommandText(call))
	workingDir := strings.TrimSpace(toolDetailCommandWorkingDirectory(call))
	result := strings.TrimSpace(toolResultOutputForSession(m, sessionID, &ref, call))
	errorText := strings.TrimSpace(toolResultErrorForSession(m, sessionID, &ref, call))
	notice := ""
	if !toolResultBodyLoadedForSession(m, sessionID, &ref) {
		notice = strings.TrimSpace(toolResultPreviewNotice(call))
	}

	lines := make([]string, 0, 16)
	if command != "" {
		lines = append(lines, "## Command", fencedToolDetailBlock("bash-wrap", command))
	}
	if workingDir != "" && workingDir != "." {
		lines = append(lines, "", "Dir: `"+workingDir+"`")
	}
	if result != "" {
		lines = append(lines, "", "## Output", fencedToolDetailBlock(toolDetailOutputLanguage(call, result), result))
	} else if call.Executing {
		lines = append(lines, "", "## Output", fencedToolDetailBlock("text", "(running...)"))
	} else if placeholder := strings.TrimSpace(toolResultHydrationPlaceholderForSession(m, sessionID, &ref, call)); placeholder != "" {
		lines = append(lines, "", "## Output", fencedToolDetailBlock("text", "("+placeholder+")"))
	} else if errorText == "" {
		lines = append(lines, "", "## Output", fencedToolDetailBlock("text", "(no output)"))
	}
	if errorText != "" {
		lines = append(lines, "", "## Error", toolDetailErrorBlock(errorText))
	}
	if notice != "" {
		lines = append(lines, "", "_"+notice+"_")
	}
	return strings.Join(lines, "\n")
}

func renderGenericToolDetailMarkdownForSession(m Model, sessionID string, ref sessionToolCallRef, call *events.ToolCallState) string {
	lines := make([]string, 0, 20)
	errorText := strings.TrimSpace(toolResultErrorForSession(m, sessionID, &ref, call))
	if errorText != "" {
		lines = append(lines, "## Error", toolDetailErrorBlock(errorText))
	}
	if details := toolDetailTranscriptDetails(call); strings.TrimSpace(details) != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Details")
		for _, line := range strings.Split(details, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			lines = append(lines, "- "+line)
		}
	}
	if isMCPToolCall(call) {
		if structured, ok := renderStructuredPayloadMarkdown("## Input", call.Input); ok {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, structured)
		} else if language, arguments, ok := toolDetailArgumentsBlock(call); ok {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "## Input", fencedToolDetailBlock(language, arguments))
		}
	} else if language, arguments, ok := toolDetailArgumentsBlock(call); ok {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Input", fencedToolDetailBlock(language, arguments))
	}

	output := strings.TrimSpace(toolResultOutputForSession(m, sessionID, &ref, call))
	notice := ""
	if !toolResultBodyLoadedForSession(m, sessionID, &ref) {
		notice = strings.TrimSpace(toolResultPreviewNotice(call))
	}
	if output != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		if isMCPToolCall(call) {
			if structured, ok := renderStructuredPayloadMarkdown("## Output", output); ok {
				lines = append(lines, structured)
			} else {
				lines = append(lines, "## Output", fencedToolDetailBlock(toolDetailOutputLanguage(call, output), output))
			}
		} else {
			lines = append(lines, "## Output", fencedToolDetailBlock(toolDetailOutputLanguage(call, output), output))
		}
	}
	if output == "" && errorText == "" && call.Executing {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Output", fencedToolDetailBlock("text", "(running...)"))
	}
	if output == "" && errorText == "" && !call.Executing {
		if placeholder := strings.TrimSpace(toolResultHydrationPlaceholderForSession(m, sessionID, &ref, call)); placeholder != "" {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, "## Output", fencedToolDetailBlock("text", "("+placeholder+")"))
		}
	}
	if notice != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "_"+notice+"_")
	}
	return strings.Join(lines, "\n")
}
