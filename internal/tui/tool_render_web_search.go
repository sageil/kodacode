package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

const webSearchToolViewDefaultLimit = 5

func webSearchInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, ok := parseWebSearchToolViewInput(call.Input)
	if !ok {
		return defaultInspectorParams(call)
	}

	params := []inspectorParam{
		{Label: "Query", Value: input.Query},
		{Label: "Limit", Value: fmt.Sprintf("%d", input.Limit)},
	}
	if len(input.Domains) > 0 {
		params = append(params, inspectorParam{Label: "Domains", Value: strings.Join(input.Domains, ", ")})
	}
	if len(input.ExcludeDomains) > 0 {
		params = append(params, inspectorParam{Label: "Exclude", Value: strings.Join(input.ExcludeDomains, ", ")})
	}
	if input.FreshnessDays > 0 {
		params = append(params, inspectorParam{Label: "Freshness", Value: fmt.Sprintf("%d days", input.FreshnessDays)})
	}
	if result, ok := parseWebSearchStructuredToolResult(call); ok {
		if provider := strings.TrimSpace(result.Provider); provider != "" {
			params = append(params, inspectorParam{Label: "Provider", Value: provider})
		}
		params = append(params, inspectorParam{Label: "Results", Value: fmt.Sprintf("%d", len(result.Results))})
		if domains := webSearchStructuredDomainsSummary(result.Results); domains != "" {
			params = append(params, inspectorParam{Label: "Result Domains", Value: domains})
		}
		if notice := strings.TrimSpace(result.Notice); notice != "" {
			params = append(params, inspectorParam{Label: "Notice", Value: notice})
		}
	} else if strings.TrimSpace(input.Provider) != "" {
		params = append(params, inspectorParam{Label: "Provider", Value: input.Provider})
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func parseWebSearchToolViewInput(raw string) (struct {
	Query          string
	Limit          int
	Domains        []string
	ExcludeDomains []string
	FreshnessDays  int
	Provider       string
}, bool) {
	var wire struct {
		Query          string          `json:"query"`
		Limit          json.RawMessage `json:"limit"`
		Domains        json.RawMessage `json:"domains"`
		ExcludeDomains json.RawMessage `json:"exclude_domains"`
		FreshnessDays  json.RawMessage `json:"freshness_days"`
		Provider       string          `json:"provider"`
	}
	if json.Unmarshal([]byte(raw), &wire) != nil || strings.TrimSpace(wire.Query) == "" {
		return struct {
			Query          string
			Limit          int
			Domains        []string
			ExcludeDomains []string
			FreshnessDays  int
			Provider       string
		}{}, false
	}
	limit := webSearchToolViewDefaultLimit
	if value, ok := parseToolViewOptionalInt(wire.Limit); ok && value > 0 {
		limit = value
	}
	freshnessDays := 0
	if value, ok := parseToolViewOptionalInt(wire.FreshnessDays); ok && value > 0 {
		freshnessDays = value
	}
	return struct {
		Query          string
		Limit          int
		Domains        []string
		ExcludeDomains []string
		FreshnessDays  int
		Provider       string
	}{
		Query:          strings.TrimSpace(wire.Query),
		Limit:          limit,
		Domains:        normalizeWebSearchToolViewDomains(wire.Domains),
		ExcludeDomains: normalizeWebSearchToolViewDomains(wire.ExcludeDomains),
		FreshnessDays:  freshnessDays,
		Provider:       strings.TrimSpace(wire.Provider),
	}, true
}

func normalizeWebSearchToolViewDomains(raw json.RawMessage) []string {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		return nil
	}

	var single string
	if json.Unmarshal(raw, &single) == nil {
		return normalizeWebSearchToolViewDomainList([]string{single})
	}

	var many []string
	if json.Unmarshal(raw, &many) == nil {
		return normalizeWebSearchToolViewDomainList(many)
	}
	return nil
}

func normalizeWebSearchToolViewDomainList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func webSearchToolListSummary(call *events.ToolCallState) string {
	input, ok := parseWebSearchToolViewInput(call.Input)
	if !ok {
		return ""
	}
	lines := []string{"query: " + input.Query}
	if input.Limit > 0 {
		lines = append(lines, fmt.Sprintf("limit: %d", input.Limit))
	}
	if len(input.Domains) > 0 {
		lines = append(lines, "domains: "+strings.Join(input.Domains, ", "))
	}
	if len(input.ExcludeDomains) > 0 {
		lines = append(lines, "exclude: "+strings.Join(input.ExcludeDomains, ", "))
	}
	if input.FreshnessDays > 0 {
		lines = append(lines, fmt.Sprintf("freshness: %d days", input.FreshnessDays))
	}
	if strings.TrimSpace(input.Provider) != "" {
		lines = append(lines, "provider: "+input.Provider)
	}
	return strings.Join(lines, "\n")
}
