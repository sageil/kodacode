package tui

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type searchSkillsToolViewInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type skillToolViewInput struct {
	ID      string `json:"id"`
	Section string `json:"section"`
}

func parseSearchSkillsToolViewInput(raw string) (searchSkillsToolViewInput, bool) {
	var wire struct {
		Query string          `json:"query"`
		Limit json.RawMessage `json:"limit"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil {
		return searchSkillsToolViewInput{}, false
	}
	limit, _ := parseToolViewOptionalInt(wire.Limit)
	input := searchSkillsToolViewInput{
		Query: strings.TrimSpace(wire.Query),
		Limit: limit,
	}
	return input, input.Query != ""
}

func parseSkillToolViewInput(raw string) (skillToolViewInput, bool) {
	var input skillToolViewInput
	if json.Unmarshal([]byte(raw), &input) != nil {
		return input, false
	}
	input.ID = strings.TrimSpace(input.ID)
	input.Section = strings.TrimSpace(input.Section)
	return input, input.ID != ""
}

func searchSkillsToolDisplayName(call *events.ToolCallState) string {
	input, ok := parseSearchSkillsToolViewInput(call.Input)
	if !ok {
		return "search skills"
	}
	return "search skills for " + input.Query
}

func skillToolDisplayName(call *events.ToolCallState) string {
	input, ok := parseSkillToolViewInput(call.Input)
	if !ok {
		return "skill"
	}
	if input.Section != "" {
		return "load skill " + input.ID + " · " + input.Section
	}
	return "load skill " + input.ID
}

func searchSkillsToolListSummary(call *events.ToolCallState) string {
	input, ok := parseSearchSkillsToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{"query: " + input.Query}
	if input.Limit > 0 {
		lines = append(lines, "limit: "+strconv.Itoa(input.Limit))
	}
	return strings.Join(lines, "\n")
}

func skillToolListSummary(call *events.ToolCallState) string {
	input, ok := parseSkillToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{"id: " + input.ID}
	if input.Section != "" {
		lines = append(lines, "section: "+input.Section)
	}
	return strings.Join(lines, "\n")
}

func searchSkillsInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseSearchSkillsToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{{Label: "Query", Value: input.Query}}
	if input.Limit > 0 {
		params = append(params, inspectorParam{Label: "Limit", Value: strconv.Itoa(input.Limit)})
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func skillInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseSkillToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}
	params := []inspectorParam{{Label: "Skill", Value: input.ID}}
	if input.Section != "" {
		params = append(params, inspectorParam{Label: "Section", Value: input.Section})
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}
