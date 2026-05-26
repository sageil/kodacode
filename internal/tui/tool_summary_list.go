package tui

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type listToolViewInput struct {
	Path          string `json:"path"`
	IncludeHidden bool   `json:"include_hidden"`
}

func parseListToolViewInput(raw string) (listToolViewInput, bool) {
	var wire struct {
		Path          string          `json:"path"`
		IncludeHidden json.RawMessage `json:"include_hidden"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return listToolViewInput{}, false
	}
	includeHidden, _ := parseToolViewOptionalBool(wire.IncludeHidden)
	input := listToolViewInput{
		Path:          strings.TrimSpace(wire.Path),
		IncludeHidden: includeHidden,
	}
	return input, input.Path != ""
}

func listToolListSummary(call *events.ToolCallState) string {
	input, ok := parseListToolViewInput(call.Input)
	if !ok {
		return ""
	}
	return strings.Join([]string{
		input.Path,
		"hidden: " + onOffLabel(input.IncludeHidden),
	}, "\n")
}
