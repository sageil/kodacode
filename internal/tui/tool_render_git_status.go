package tui

import "github.com/sageil/kodacode/internal/events"

func gitStatusInspectorParams(call *events.ToolCallState) []inspectorParam {
	params := []inspectorParam{{Label: "Scope", Value: "workspace root"}}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}
