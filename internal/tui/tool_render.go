package tui

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

// toolInspectorParams dispatches to tool-specific param renderers that return
// compact label/value pairs for the gutter-style inspector display.
func toolInspectorParams(call *events.ToolCallState) []inspectorParam {
	if presenter, ok := toolPresenterForCall(call); ok && presenter.InspectorParams != nil {
		return presenter.InspectorParams(call)
	}
	return defaultInspectorParams(call)
}

func defaultInspectorParams(call *events.ToolCallState) []inspectorParam {
	summary := strings.TrimSpace(toolPrimaryListSummary(call))
	if summary == "" {
		params := make([]inspectorParam, 0, 1)
		if errorText := summarizeBlock(call.Error); errorText != "" {
			params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
		}
		return params
	}
	params := []inspectorParam{{Label: "Details", Value: summary}}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func writeInspectorParams(call *events.ToolCallState) []inspectorParam {
	display, ok := mutationDisplayFromCall("", call)
	if !ok {
		return defaultInspectorParams(call)
	}
	return mutationInspectorParams(display)
}

func editInspectorParams(call *events.ToolCallState) []inspectorParam {
	display, ok := mutationDisplayFromCall("", call)
	if !ok {
		return defaultInspectorParams(call)
	}
	return mutationInspectorParams(display)
}

func mkdirInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseMkdirToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseWriteToolViewInput(raw string) (struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}, bool) {
	var input struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil || strings.TrimSpace(input.Path) == "" {
		return input, false
	}
	return input, true
}

type editToolViewInput struct {
	Path          string
	StartLine     int
	EndLine       int
	HasStartLine  bool
	HasEndLine    bool
	OldText       string
	NewText       string
	ReplaceAll    bool
	HasReplaceAll bool
}

func parseEditToolViewInput(raw string) (editToolViewInput, bool) {
	var wire struct {
		Path       string          `json:"path"`
		StartLine  json.RawMessage `json:"start_line"`
		EndLine    json.RawMessage `json:"end_line"`
		OldText    string          `json:"old_text"`
		NewText    string          `json:"new_text"`
		ReplaceAll json.RawMessage `json:"replace_all"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil || strings.TrimSpace(wire.Path) == "" {
		return editToolViewInput{}, false
	}
	startLine, hasStartLine := parseToolViewOptionalInt(wire.StartLine)
	endLine, hasEndLine := parseToolViewOptionalInt(wire.EndLine)
	replaceAll, hasReplaceAll := parseToolViewOptionalBool(wire.ReplaceAll)
	return editToolViewInput{
		Path:          wire.Path,
		StartLine:     startLine,
		EndLine:       endLine,
		HasStartLine:  hasStartLine,
		HasEndLine:    hasEndLine,
		OldText:       wire.OldText,
		NewText:       wire.NewText,
		ReplaceAll:    replaceAll,
		HasReplaceAll: hasReplaceAll,
	}, true
}

func parseMkdirToolViewInput(raw string) (struct {
	Path string `json:"path"`
}, bool) {
	var input struct {
		Path string `json:"path"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil || strings.TrimSpace(input.Path) == "" {
		return input, false
	}
	input.Path = strings.TrimSpace(input.Path)
	return input, true
}

func parseToolViewOptionalInt(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return 0, false
	}
	var value int
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		value, err := strconv.Atoi(strings.TrimSpace(text))
		if err == nil {
			return value, true
		}
	}
	return 0, false
}

func parseToolViewOptionalBool(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "null" {
		return false, false
	}
	var value bool
	if json.Unmarshal(raw, &value) == nil {
		return value, true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		switch {
		case strings.EqualFold(strings.TrimSpace(text), "true"):
			return true, true
		case strings.EqualFold(strings.TrimSpace(text), "false"):
			return false, true
		}
	}
	return false, false
}

func contentStatsLabel(content string) string {
	if content == "" {
		return "empty file"
	}
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	return fmt.Sprintf("%d lines • %d bytes", len(strings.Split(normalized, "\n")), len(content))
}
