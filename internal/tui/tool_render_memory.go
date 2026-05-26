package tui

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type memoryToolViewInput struct {
	Action  string `json:"action"`
	Content string `json:"content"`
	ID      string `json:"id"`
}

func parseMemoryToolViewInput(raw string) (memoryToolViewInput, bool) {
	var input memoryToolViewInput
	if json.Unmarshal([]byte(raw), &input) != nil {
		return input, false
	}
	input.Action = strings.TrimSpace(input.Action)
	input.Content = strings.TrimSpace(input.Content)
	input.ID = strings.TrimSpace(input.ID)
	return input, input.Action != ""
}

func memoryToolDisplayName(call *events.ToolCallState) string {
	input, ok := parseMemoryToolViewInput(call.Input)
	if !ok {
		return "memory"
	}
	switch input.Action {
	case "save":
		return "save project memory"
	case "list":
		return "list project memories"
	case "delete":
		if input.ID != "" {
			return "delete project memory " + input.ID
		}
		return "delete project memory"
	default:
		return "memory " + input.Action
	}
}

func memoryToolListSummary(call *events.ToolCallState) string {
	input, ok := parseMemoryToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{"action: " + input.Action}
	if input.ID != "" {
		lines = append(lines, "id: "+input.ID)
	}
	if input.Content != "" {
		lines = append(lines, "content: "+summarizeInlineValue(input.Content))
	}
	return strings.Join(lines, "\n")
}

func memoryInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseMemoryToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{{Label: "Action", Value: input.Action}}
	if input.ID != "" {
		params = append(params, inspectorParam{Label: "ID", Value: input.ID})
	}
	if input.Content != "" {
		params = append(params, inspectorParam{Label: "Content", Value: summarizeInlineValue(input.Content)})
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}
