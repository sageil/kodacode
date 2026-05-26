package tui

import "github.com/sageil/kodacode/internal/events"

func listInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseListToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
		{Label: "Hidden", Value: onOffLabel(input.IncludeHidden)},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}
