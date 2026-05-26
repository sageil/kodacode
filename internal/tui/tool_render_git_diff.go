package tui

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type gitDiffToolViewInput struct {
	Staged bool `json:"staged"`
}

func gitDiffInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseGitDiffToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	params := []inspectorParam{
		{Label: "Scope", Value: gitDiffScopeLabel(input.Staged)},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseGitDiffToolViewInput(raw string) (gitDiffToolViewInput, bool) {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(raw) == "null" {
		return gitDiffToolViewInput{}, false
	}
	var wire struct {
		Staged json.RawMessage `json:"staged"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return gitDiffToolViewInput{}, false
	}
	staged, ok := parseToolViewOptionalBool(wire.Staged)
	if !ok {
		return gitDiffToolViewInput{}, false
	}
	return gitDiffToolViewInput{Staged: staged}, true
}

func gitDiffScopeLabel(staged bool) string {
	if staged {
		return "staged"
	}
	return "working"
}
