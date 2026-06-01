package app

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
	"github.com/sageil/kodacode/internal/tool"
)

func providerStepExplorationTargets(toolName, arguments string) ([]string, bool) {
	switch strings.TrimSpace(toolName) {
	case "read":
		var raw struct {
			Path   string   `json:"path"`
			Paths  []string `json:"paths"`
			Offset *int     `json:"offset"`
			Limit  *int     `json:"limit"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return nil, false
		}
		paths := append([]string(nil), raw.Paths...)
		if len(paths) == 0 && strings.TrimSpace(raw.Path) != "" {
			paths = append(paths, raw.Path)
		}
		targets := make([]string, 0, len(paths))
		offset := 0
		if raw.Offset != nil && *raw.Offset > 0 {
			offset = *raw.Offset
		}
		limit := tool.DefaultReadLimit
		if raw.Limit != nil && *raw.Limit > 0 {
			limit = *raw.Limit
		}
		for _, path := range paths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			targets = append(targets, "read:path="+path+
				":offset="+intFingerprint(offset)+
				":limit="+intFingerprint(limit))
		}
		return uniqueSortedStrings(targets), len(targets) > 0
	case "locate":
		var raw struct {
			Path          string `json:"path"`
			Query         string `json:"query"`
			IncludeHidden *bool  `json:"include_hidden"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return nil, false
		}
		path := strings.TrimSpace(raw.Path)
		query := strings.TrimSpace(raw.Query)
		if path == "" || query == "" {
			return nil, false
		}
		return []string{
			"locate:path=" + path +
				":query=" + query +
				":include_hidden=" + boolFingerprint(boolPointerValue(raw.IncludeHidden)),
		}, true
	case "search":
		var raw struct {
			Path          string `json:"path"`
			Query         string `json:"query"`
			Glob          string `json:"glob"`
			Mode          string `json:"mode"`
			Regex         bool   `json:"regex"`
			CaseSensitive bool   `json:"case_sensitive"`
		}
		if err := json.Unmarshal([]byte(arguments), &raw); err != nil {
			return nil, false
		}
		path := strings.TrimSpace(raw.Path)
		query := strings.TrimSpace(raw.Query)
		if path == "" || query == "" {
			return nil, false
		}
		mode := strings.TrimSpace(raw.Mode)
		if mode == "" {
			mode = "auto"
		}
		glob := strings.TrimSpace(raw.Glob)
		return []string{
			"search:path=" + path +
				":query=" + query +
				":mode=" + mode +
				":glob=" + glob +
				":regex=" + boolFingerprint(raw.Regex) +
				":case_sensitive=" + boolFingerprint(raw.CaseSensitive),
		}, true
	default:
		return nil, false
	}
}

func explorationSummary(toolName, arguments, output, errorText string) string {
	label := explorationLabel(toolName, arguments)
	detail := explorationDetail(output, errorText)
	switch {
	case label == "" && detail == "":
		return ""
	case detail == "":
		return label
	case label == "":
		return detail
	default:
		return label + " -> " + detail
	}
}

func explorationLabel(toolName, arguments string) string {
	switch strings.TrimSpace(toolName) {
	case tool.ReadToolName:
		var input struct {
			Paths []string `json:"paths"`
		}
		if json.Unmarshal([]byte(arguments), &input) == nil && len(input.Paths) > 0 {
			return "read " + strings.Join(input.Paths, ", ")
		}
	case tool.SearchToolName:
		var input struct {
			Query string `json:"query"`
			Path  string `json:"path"`
		}
		if json.Unmarshal([]byte(arguments), &input) == nil {
			parts := filterNonEmpty([]string{strings.TrimSpace(input.Query), strings.TrimSpace(input.Path)})
			if len(parts) > 0 {
				return "search " + strings.Join(parts, " @ ")
			}
		}
	case tool.LocateToolName:
		var input struct {
			Query string `json:"query"`
			Path  string `json:"path"`
		}
		if json.Unmarshal([]byte(arguments), &input) == nil {
			parts := filterNonEmpty([]string{strings.TrimSpace(input.Query), strings.TrimSpace(input.Path)})
			if len(parts) > 0 {
				return "locate " + strings.Join(parts, " @ ")
			}
		}
	}
	return strings.TrimSpace(toolName)
}

func explorationDetail(output, errorText string) string {
	if text := strings.TrimSpace(errorText); text != "" {
		return truncateCompact(singleLineCompact(text))
	}
	if text := strings.TrimSpace(output); text != "" {
		return truncateCompact(singleLineCompact(text))
	}
	return ""
}

func currentTurnPureExchangeTool(name string) bool {
	switch strings.TrimSpace(name) {
	case tool.ReadToolName, tool.SearchToolName, tool.LocateToolName:
		return true
	default:
		return false
	}
}

func filterNonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		out = append(out, strings.TrimSpace(part))
	}
	return out
}

func handoffExplorationEntries(ctx context.Context, sessions *SessionService, sessionID, turnID string, turn *events.TurnState) ([]events.AgentHandoffExplorationEntry, error) {
	if turn == nil {
		return nil, nil
	}

	type orderedEntry struct {
		entry     events.AgentHandoffExplorationEntry
		lastOrder int
	}

	entriesByTarget := make(map[string]orderedEntry)
	order := 0
	for _, callID := range turn.ToolCallOrder {
		call := turn.ToolCalls[callID]
		if call == nil || !call.Completed || !currentTurnPureExchangeTool(call.ToolName) {
			continue
		}
		targets, ok := providerStepExplorationTargets(call.ToolName, call.Input)
		if !ok || len(targets) == 0 {
			continue
		}
		output := call.Output
		errorText := call.Error
		if sessions != nil && strings.TrimSpace(sessionID) != "" && strings.TrimSpace(turnID) != "" && call.Completed && strings.TrimSpace(output) == "" && strings.TrimSpace(errorText) == "" {
			detail, err := sessions.LoadToolResult(ctx, sessionID, turnID, call.CallID)
			if err != nil {
				return nil, err
			}
			output = detail.Output
			errorText = detail.Error
		}
		summary := explorationSummary(call.ToolName, call.Input, output, errorText)
		if summary == "" {
			continue
		}
		for _, target := range targets {
			entriesByTarget[target] = orderedEntry{
				entry: events.AgentHandoffExplorationEntry{
					ToolName: strings.TrimSpace(call.ToolName),
					Target:   strings.TrimSpace(target),
					Summary:  summary,
				},
				lastOrder: order,
			}
			order++
		}
	}
	if len(entriesByTarget) == 0 {
		return nil, nil
	}

	ordered := make([]orderedEntry, 0, len(entriesByTarget))
	for _, entry := range entriesByTarget {
		ordered = append(ordered, entry)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].lastOrder == ordered[j].lastOrder {
			return ordered[i].entry.Target < ordered[j].entry.Target
		}
		return ordered[i].lastOrder < ordered[j].lastOrder
	})

	out := make([]events.AgentHandoffExplorationEntry, 0, len(ordered))
	for _, entry := range ordered {
		out = append(out, entry.entry)
	}
	return out, nil
}

func cloneHandoffExplorationEntries(entries []events.AgentHandoffExplorationEntry) []events.AgentHandoffExplorationEntry {
	if len(entries) == 0 {
		return nil
	}
	return append([]events.AgentHandoffExplorationEntry(nil), entries...)
}

func sameHandoffExplorationEntries(left, right []events.AgentHandoffExplorationEntry) bool {
	left = compactHandoffExplorationEntries(left)
	right = compactHandoffExplorationEntries(right)
	if len(left) != len(right) {
		return false
	}
	for idx := range left {
		if left[idx] != right[idx] {
			return false
		}
	}
	return true
}

func compactHandoffExplorationEntries(entries []events.AgentHandoffExplorationEntry) []events.AgentHandoffExplorationEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]events.AgentHandoffExplorationEntry, 0, len(entries))
	for _, entry := range entries {
		toolName := strings.TrimSpace(entry.ToolName)
		target := strings.TrimSpace(entry.Target)
		summary := strings.TrimSpace(entry.Summary)
		if toolName == "" || target == "" || summary == "" {
			continue
		}
		out = append(out, events.AgentHandoffExplorationEntry{
			ToolName: toolName,
			Target:   target,
			Summary:  summary,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func handoffExplorationFragment(entries []events.AgentHandoffExplorationEntry) (prompt.Fragment, bool) {
	entries = compactHandoffExplorationEntries(entries)
	if len(entries) == 0 {
		return prompt.Fragment{}, false
	}
	lines := []string{
		"Parent turn exploration already completed.",
		"Reuse this context when sufficient; repeat `read`, `search`, or `locate` only after a stale-resource signal or explicit user recheck request.",
	}
	for _, entry := range entries {
		lines = append(lines, "- "+entry.Summary)
	}
	return prompt.Fragment{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Layer:     "handoff-exploration",
		Key:       "handoff-exploration",
		Label:     "handoff-exploration",
		Content:   strings.Join(lines, "\n"),
	}, true
}
