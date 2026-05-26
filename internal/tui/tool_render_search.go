package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

const searchToolViewMaxMatchesLimit = 200
const searchToolViewDefaultMaxMatches = searchToolViewMaxMatchesLimit

func searchInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseSearchToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	caseMode := "off"
	if input.CaseSensitive {
		caseMode = "on"
	}
	params := []inspectorParam{
		{Label: "Query", Value: input.Query},
		{Label: "Path", Value: input.Path},
		{Label: "Mode", Value: input.Mode},
		{Label: "Regex", Value: onOffLabel(input.Regex)},
		{Label: "Case", Value: caseMode},
		{Label: "Max Matches", Value: fmt.Sprintf("%d", input.MaxMatches)},
	}
	if strings.TrimSpace(input.Glob) != "" {
		params = append(params, inspectorParam{Label: "File Filter", Value: input.Glob})
	}
	if result, ok := parseSearchStructuredToolResult(call); ok {
		if mode := strings.TrimSpace(result.Mode); mode != "" {
			params = append(params, inspectorParam{Label: "Result Mode", Value: mode})
		}
		params = append(params, inspectorParam{Label: "Matches", Value: fmt.Sprintf("%d", len(result.Results))})
		if result.Fallback {
			params = append(params, inspectorParam{Label: "Fallback", Value: onOffLabel(true)})
		}
		if sources := searchStructuredSourcesSummary(result.Results); sources != "" {
			params = append(params, inspectorParam{Label: "Sources", Value: sources})
		}
		if notice := strings.TrimSpace(result.Notice); notice != "" {
			params = append(params, inspectorParam{Label: "Notice", Value: notice})
		}
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseSearchToolViewInput(raw string) (struct {
	Query         string `json:"query"`
	Path          string `json:"path"`
	Mode          string `json:"mode"`
	Glob          string `json:"glob"`
	Regex         bool   `json:"regex"`
	CaseSensitive bool   `json:"case_sensitive"`
	MaxMatches    int    `json:"max_matches"`
}, bool) {
	var rawInput struct {
		Query         string          `json:"query"`
		Path          string          `json:"path"`
		Mode          *string         `json:"mode"`
		Glob          *string         `json:"glob"`
		Regex         json.RawMessage `json:"regex"`
		CaseSensitive json.RawMessage `json:"case_sensitive"`
		MaxMatches    json.RawMessage `json:"max_matches"`
	}
	if json.Unmarshal([]byte(raw), &rawInput) != nil || strings.TrimSpace(rawInput.Query) == "" || strings.TrimSpace(rawInput.Path) == "" {
		return struct {
			Query         string `json:"query"`
			Path          string `json:"path"`
			Mode          string `json:"mode"`
			Glob          string `json:"glob"`
			Regex         bool   `json:"regex"`
			CaseSensitive bool   `json:"case_sensitive"`
			MaxMatches    int    `json:"max_matches"`
		}{}, false
	}
	mode := "auto"
	if rawInput.Mode != nil && strings.TrimSpace(*rawInput.Mode) != "" {
		mode = strings.TrimSpace(*rawInput.Mode)
	}
	regex, hasRegex := parseToolViewOptionalBool(rawInput.Regex)
	if !hasRegex {
		regex = false
	}
	caseSensitive, hasCaseSensitive := parseToolViewOptionalBool(rawInput.CaseSensitive)
	if !hasCaseSensitive {
		caseSensitive = false
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
	input := struct {
		Query         string `json:"query"`
		Path          string `json:"path"`
		Mode          string `json:"mode"`
		Glob          string `json:"glob"`
		Regex         bool   `json:"regex"`
		CaseSensitive bool   `json:"case_sensitive"`
		MaxMatches    int    `json:"max_matches"`
	}{
		Query:         rawInput.Query,
		Path:          rawInput.Path,
		Mode:          mode,
		Glob:          strings.TrimSpace(searchViewString(rawInput.Glob)),
		Regex:         regex,
		CaseSensitive: caseSensitive,
		MaxMatches:    maxMatches,
	}
	if strings.TrimSpace(input.Mode) == "" || input.MaxMatches <= 0 {
		return input, false
	}
	return input, true
}

func searchViewString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
