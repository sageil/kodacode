package app

import (
	"strings"

	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func sessionHistoryToolResultInputWithOutcome(callID, toolName, output, errorText string, succeeded bool) provider.Input {
	return providerToolResultInput(callID, toolName, provider.ToolKindFunction, output, errorText, succeeded)
}

func providerToolResultInput(callID, toolName string, toolKind provider.ToolKind, output, errorText string, succeeded bool) provider.Input {
	output, errorText = normalizeToolResultText(toolName, output, errorText, succeeded)
	return provider.Input{
		Kind:     provider.InputKindToolResult,
		CallID:   callID,
		ToolName: toolName,
		ToolKind: inputToolKindOrDefault(toolKind),
		Output:   output,
		Error:    errorText,
	}
}

func normalizeToolResultText(toolName, output, errorText string, succeeded bool) (string, string) {
	if strings.TrimSpace(errorText) != "" || !succeeded || strings.TrimSpace(output) != "" {
		return output, errorText
	}
	return emptySuccessfulToolResultSummary(toolName), ""
}

func emptySuccessfulToolResultSummary(toolName string) string {
	switch toolName {
	case tool.LocateToolName:
		return "no paths found"
	case tool.SearchToolName:
		return "no matches found"
	default:
		return "completed successfully with no output"
	}
}
