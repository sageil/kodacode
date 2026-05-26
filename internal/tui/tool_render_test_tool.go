package tui

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/sageil/kodacode/internal/events"
)

type testToolViewInput struct {
	Command string `json:"command"`
	Path    string `json:"path"`
	Filter  string `json:"filter"`
	Timeout int    `json:"timeout"`
}

func parseTestToolViewInput(raw string) (testToolViewInput, bool) {
	var wire struct {
		Command *string         `json:"command"`
		Path    *string         `json:"path"`
		Filter  *string         `json:"filter"`
		Timeout json.RawMessage `json:"timeout"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return testToolViewInput{}, false
	}
	timeout, _ := parseToolViewOptionalInt(wire.Timeout)
	input := testToolViewInput{
		Command: strings.TrimSpace(searchViewString(wire.Command)),
		Path:    strings.TrimSpace(searchViewString(wire.Path)),
		Filter:  strings.TrimSpace(searchViewString(wire.Filter)),
		Timeout: timeout,
	}
	return input, true
}

func testInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseTestToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	command := testCommandDisplayName("", call)
	params := []inspectorParam{
		{Label: "Cmd", Value: command},
	}
	if input.Path != "" && input.Path != "." {
		params = append(params, inspectorParam{Label: "Path", Value: input.Path})
	}
	if input.Filter != "" {
		params = append(params, inspectorParam{Label: "Filter", Value: input.Filter})
	}
	if input.Timeout > 0 {
		params = append(params, inspectorParam{Label: "Timeout", Value: formatMilliseconds(input.Timeout)})
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func testCommandDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	if call != nil && call.Execution != nil && strings.TrimSpace(call.Execution.CommandPreview) != "" {
		return displayCommandPath(workspaceRoot, strings.TrimSpace(call.Execution.CommandPreview))
	}
	input, ok := parseTestToolViewInput(call.Input)
	if !ok {
		return "test"
	}
	if input.Command != "" {
		return displayCommandPath(workspaceRoot, input.Command)
	}
	if input.Path != "" && input.Path != "." {
		return "auto-detect @ " + displayToolPath(workspaceRoot, input.Path)
	}
	return "auto-detect"
}

func testToolDisplayName(workspaceRoot string, call *events.ToolCallState) string {
	command := strings.TrimSpace(testCommandDisplayName(workspaceRoot, call))
	if command == "" {
		return "test"
	}
	return "test " + command
}

func formatMilliseconds(value int) string {
	duration := time.Duration(value) * time.Millisecond
	if value%1000 == 0 {
		return duration.String()
	}
	return duration.Round(time.Millisecond).String()
}
