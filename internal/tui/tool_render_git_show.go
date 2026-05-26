package tui

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type gitShowToolViewInput struct {
	Rev string `json:"rev"`
}

func gitShowInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseGitShowToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	params := []inspectorParam{
		{Label: "Revision", Value: input.Rev},
		{Label: "Scope", Value: "workspace root"},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseGitShowToolViewInput(raw string) (gitShowToolViewInput, bool) {
	var input gitShowToolViewInput
	if json.Unmarshal([]byte(raw), &input) != nil {
		return input, false
	}
	input.Rev = strings.TrimSpace(input.Rev)
	if input.Rev == "" {
		return input, false
	}
	return input, true
}
