package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func locateInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseLocateToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	params := []inspectorParam{
		{Label: "Query", Value: input.Query},
		{Label: "Path", Value: input.Path},
		{Label: "Hidden", Value: onOffLabel(input.IncludeHidden)},
		{Label: "Max Matches", Value: fmt.Sprintf("%d", input.MaxMatches)},
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseLocateToolViewInput(raw string) (struct {
	Query         string `json:"query"`
	Path          string `json:"path"`
	IncludeHidden bool   `json:"include_hidden"`
	MaxMatches    int    `json:"max_matches"`
}, bool) {
	var rawInput struct {
		Query         string          `json:"query"`
		Path          string          `json:"path"`
		IncludeHidden json.RawMessage `json:"include_hidden"`
		MaxMatches    json.RawMessage `json:"max_matches"`
	}
	if json.Unmarshal([]byte(raw), &rawInput) != nil || strings.TrimSpace(rawInput.Path) == "" {
		return struct {
			Query         string `json:"query"`
			Path          string `json:"path"`
			IncludeHidden bool   `json:"include_hidden"`
			MaxMatches    int    `json:"max_matches"`
		}{}, false
	}

	maxMatches := searchToolViewDefaultMaxMatches
	if value, ok := parseToolViewOptionalInt(rawInput.MaxMatches); ok {
		maxMatches = value
	}
	if maxMatches <= 0 {
		maxMatches = searchToolViewDefaultMaxMatches
	}
	if maxMatches > searchToolViewMaxMatchesLimit {
		maxMatches = searchToolViewMaxMatchesLimit
	}

	includeHidden, hasIncludeHidden := parseToolViewOptionalBool(rawInput.IncludeHidden)
	if !hasIncludeHidden {
		includeHidden = false
	}
	return struct {
		Query         string `json:"query"`
		Path          string `json:"path"`
		IncludeHidden bool   `json:"include_hidden"`
		MaxMatches    int    `json:"max_matches"`
	}{
		Query:         strings.TrimSpace(rawInput.Query),
		Path:          strings.TrimSpace(rawInput.Path),
		IncludeHidden: includeHidden,
		MaxMatches:    maxMatches,
	}, true
}
