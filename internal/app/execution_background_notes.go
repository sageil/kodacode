package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func executionBackgroundReadyNote(execution *replayedExecution, payload events.ExecutionBackgroundReadyPayload) string {
	command := backgroundExecutionCommandLabel(execution, payload.ToolName)
	directory := backgroundExecutionDirectoryLabel(execution)
	note := fmt.Sprintf("Runtime note: Background command %q from %s reported ready.", command, directory)
	message := strings.TrimSpace(payload.Message)
	if message != "" {
		note += " " + message
		if !strings.HasSuffix(message, ".") {
			note += "."
		}
	}
	if payload.Port > 0 && !strings.Contains(message, strconv.Itoa(payload.Port)) {
		note += fmt.Sprintf(" Port %d.", payload.Port)
	}
	return note
}

func executionBackgroundExitedNote(execution *replayedExecution, payload events.ExecutionBackgroundExitedPayload) string {
	command := backgroundExecutionCommandLabel(execution, payload.ToolName)
	directory := backgroundExecutionDirectoryLabel(execution)
	parts := []string{
		fmt.Sprintf("Runtime note: Background command %q from %s exited.", command, directory),
	}
	if payload.ExitCode != nil {
		parts = append(parts, fmt.Sprintf("Exit code: %d.", *payload.ExitCode))
	}
	if message := strings.TrimSpace(payload.Error); message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

func executionBackgroundLostNote(execution *replayedExecution, payload events.ExecutionBackgroundLostPayload) string {
	command := backgroundExecutionCommandLabel(execution, payload.ToolName)
	directory := backgroundExecutionDirectoryLabel(execution)
	parts := []string{
		fmt.Sprintf("Runtime note: Background command %q from %s lost runtime supervision.", command, directory),
		"Do not assume it is still available without checking.",
	}
	if message := strings.TrimSpace(payload.Error); message != "" {
		parts = append(parts, message)
	}
	return strings.Join(parts, " ")
}

func backgroundExecutionCommandLabel(execution *replayedExecution, toolName string) string {
	if execution != nil {
		if preview := strings.TrimSpace(execution.CommandPreview); preview != "" {
			return preview
		}
		if name := strings.TrimSpace(execution.ToolName); name != "" {
			return name
		}
	}
	if name := strings.TrimSpace(toolName); name != "" {
		return name
	}
	return "background command"
}

func backgroundExecutionDirectoryLabel(execution *replayedExecution) string {
	if execution == nil {
		return "its working directory"
	}
	if workingDirectory := strings.TrimSpace(execution.WorkingDirectory); workingDirectory != "" {
		return workingDirectory
	}
	return "its working directory"
}
