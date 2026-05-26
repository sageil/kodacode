package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func treeInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseTreeToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	hidden := "off"
	if input.IncludeHidden {
		hidden = "on"
	}
	params := []inspectorParam{
		{Label: "Path", Value: input.Path},
		{Label: "Max Depth", Value: fmt.Sprintf("%d", input.MaxDepth)},
		{Label: "Hidden", Value: hidden},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseTreeToolViewInput(raw string) (struct {
	Path          string `json:"path"`
	MaxDepth      int    `json:"max_depth"`
	IncludeHidden bool   `json:"include_hidden"`
}, bool) {
	var wire struct {
		Path          string          `json:"path"`
		MaxDepth      json.RawMessage `json:"max_depth"`
		IncludeHidden json.RawMessage `json:"include_hidden"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return struct {
			Path          string `json:"path"`
			MaxDepth      int    `json:"max_depth"`
			IncludeHidden bool   `json:"include_hidden"`
		}{}, false
	}
	maxDepth, hasMaxDepth := parseToolViewOptionalInt(wire.MaxDepth)
	includeHidden, _ := parseToolViewOptionalBool(wire.IncludeHidden)
	if strings.TrimSpace(wire.Path) == "" || !hasMaxDepth || maxDepth < 0 {
		return struct {
			Path          string `json:"path"`
			MaxDepth      int    `json:"max_depth"`
			IncludeHidden bool   `json:"include_hidden"`
		}{}, false
	}
	return struct {
		Path          string `json:"path"`
		MaxDepth      int    `json:"max_depth"`
		IncludeHidden bool   `json:"include_hidden"`
	}{
		Path:          strings.TrimSpace(wire.Path),
		MaxDepth:      maxDepth,
		IncludeHidden: includeHidden,
	}, true
}
