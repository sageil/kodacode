package app

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

const (
	maxInspectionProgressPromptEntries = 8
	maxInspectionProgressTargets       = 4
)

type inspectionProgressPromptEntry struct {
	ToolName string
	Key      string
	Summary  string
	LastSeq  int64
}

func collectInspectionProgressPromptEntries(state *events.SessionState) []inspectionProgressPromptEntry {
	if state == nil || len(state.TurnOrder) == 0 || len(state.Turns) == 0 {
		return nil
	}
	tools := map[string]tool.Tool{
		tool.SearchToolName: tool.NewSearchTool(),
		tool.LocateToolName: tool.NewLocateTool(),
	}
	byKey := make(map[string]inspectionProgressPromptEntry)
	orderIndex := int64(0)
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil || len(turn.ToolCallOrder) == 0 || len(turn.ToolCalls) == 0 {
			continue
		}
		for _, callID := range turn.ToolCallOrder {
			call := turn.ToolCalls[callID]
			if call == nil || !call.Completed || !call.Succeeded || strings.TrimSpace(call.Error) != "" {
				continue
			}
			toolName := strings.TrimSpace(call.ToolName)
			if toolName != tool.SearchToolName && toolName != tool.LocateToolName {
				continue
			}
			key, ok := normalizedToolInputKey(tools, toolName, json.RawMessage(call.Input))
			if !ok {
				continue
			}
			summary, ok := inspectionProgressSummary(call)
			if !ok {
				continue
			}
			orderIndex++
			callSeq := call.LastUpdatedSeq
			if callSeq <= 0 {
				callSeq = orderIndex
			}
			entryKey := toolName + "\x00" + key
			if existing, ok := byKey[entryKey]; ok && existing.LastSeq >= callSeq {
				continue
			}
			byKey[entryKey] = inspectionProgressPromptEntry{
				ToolName: toolName,
				Key:      key,
				Summary:  summary,
				LastSeq:  callSeq,
			}
		}
	}
	if len(byKey) == 0 {
		return nil
	}
	entries := make([]inspectionProgressPromptEntry, 0, len(byKey))
	for _, entry := range byKey {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].LastSeq != entries[j].LastSeq {
			return entries[i].LastSeq > entries[j].LastSeq
		}
		if entries[i].ToolName != entries[j].ToolName {
			return entries[i].ToolName < entries[j].ToolName
		}
		return entries[i].Summary < entries[j].Summary
	})
	if len(entries) > maxInspectionProgressPromptEntries {
		entries = entries[:maxInspectionProgressPromptEntries]
	}
	return entries
}

func collectCompactedInspectionProgressPromptEntries(state *events.SessionState) []inspectionProgressPromptEntry {
	if !sessionHasHistoryCompaction(state) {
		return nil
	}
	return collectInspectionProgressPromptEntries(state)
}

func sessionHasHistoryCompaction(state *events.SessionState) bool {
	if state == nil {
		return false
	}
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil || turn.Continuation == nil {
			continue
		}
		if turn.Continuation.ConsolidatedTurnCount > 0 && strings.TrimSpace(turn.Continuation.RenderedSummary) != "" {
			return true
		}
	}
	return false
}

func runtimeInspectionProgressFragmentContent(entries []inspectionProgressPromptEntry) (string, bool) {
	if len(entries) == 0 {
		return "", false
	}
	lines := []string{
		"Search/locate progress from successful deterministic inspection calls in this session:",
	}
	for _, entry := range entries {
		lines = append(lines, "- "+entry.Summary)
	}
	lines = append(lines, "Compaction may hide raw tool output from the visible transcript. Do not rerun exact search/locate calls after compaction just to recover that output.")
	lines = append(lines, "Use the summarized result targets, narrow the query/path, or read specific result paths when more detail is needed.")
	return strings.Join(lines, "\n"), true
}

func inspectionProgressSummary(call *events.ToolCallState) (string, bool) {
	switch strings.TrimSpace(call.ToolName) {
	case tool.SearchToolName:
		return searchProgressSummary(call)
	case tool.LocateToolName:
		return locateProgressSummary(call)
	default:
		return "", false
	}
}

func searchProgressSummary(call *events.ToolCallState) (string, bool) {
	var input struct {
		Query      string          `json:"query"`
		Path       string          `json:"path"`
		Mode       string          `json:"mode"`
		Glob       string          `json:"glob"`
		MaxMatches json.RawMessage `json:"max_matches"`
	}
	if err := json.Unmarshal([]byte(call.Input), &input); err != nil {
		return "", false
	}
	query := strings.TrimSpace(input.Query)
	path := strings.TrimSpace(input.Path)
	if query == "" || path == "" {
		return "", false
	}
	parts := []string{fmt.Sprintf("search %q under %s", query, path)}
	if mode := strings.TrimSpace(input.Mode); mode != "" {
		parts = append(parts, "mode "+mode)
	}
	if glob := strings.TrimSpace(input.Glob); glob != "" {
		parts = append(parts, "glob "+glob)
	}
	if maxMatches := inspectionProgressOptionalInt(input.MaxMatches); maxMatches > 0 {
		parts = append(parts, fmt.Sprintf("max %d", maxMatches))
	}
	targets, total := searchProgressTargets(call.StructuredResult)
	return inspectionSummaryWithTargets(strings.Join(parts, ", "), targets, total), true
}

func locateProgressSummary(call *events.ToolCallState) (string, bool) {
	var input struct {
		Query      string          `json:"query"`
		Path       string          `json:"path"`
		MaxMatches json.RawMessage `json:"max_matches"`
	}
	if err := json.Unmarshal([]byte(call.Input), &input); err != nil {
		return "", false
	}
	path := strings.TrimSpace(input.Path)
	if path == "" {
		return "", false
	}
	query := strings.TrimSpace(input.Query)
	label := "locate"
	if query != "" {
		label += fmt.Sprintf(" %q", query)
	}
	parts := []string{fmt.Sprintf("%s under %s", label, path)}
	if maxMatches := inspectionProgressOptionalInt(input.MaxMatches); maxMatches > 0 {
		parts = append(parts, fmt.Sprintf("max %d", maxMatches))
	}
	targets, total := locateProgressTargets(call.Output)
	return inspectionSummaryWithTargets(strings.Join(parts, ", "), targets, total), true
}

func inspectionSummaryWithTargets(prefix string, targets []string, total int) string {
	if len(targets) == 0 {
		return prefix + "."
	}
	suffix := strings.Join(targets, ", ")
	if total > len(targets) {
		suffix += fmt.Sprintf(" (+%d more)", total-len(targets))
	}
	return prefix + "; returned " + suffix + "."
}

func inspectionProgressOptionalInt(raw json.RawMessage) int {
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return number
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0
	}
	return parsed
}

func searchProgressTargets(raw json.RawMessage) ([]string, int) {
	var result struct {
		Results []struct {
			Path string `json:"path"`
			Line int    `json:"line"`
		} `json:"results"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &result) != nil || len(result.Results) == 0 {
		return nil, 0
	}
	total := len(result.Results)
	targets := make([]string, 0, min(total, maxInspectionProgressTargets))
	seen := make(map[string]bool)
	for _, candidate := range result.Results {
		path := strings.TrimSpace(candidate.Path)
		if path == "" {
			continue
		}
		target := path
		if candidate.Line > 0 {
			target += fmt.Sprintf(":%d", candidate.Line)
		}
		if seen[target] {
			continue
		}
		seen[target] = true
		targets = append(targets, target)
		if len(targets) >= maxInspectionProgressTargets {
			break
		}
	}
	return targets, total
}

func locateProgressTargets(output string) ([]string, int) {
	lines := strings.Split(output, "\n")
	targets := make([]string, 0, min(len(lines), maxInspectionProgressTargets))
	total := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "notice:") || strings.HasPrefix(trimmed, "no paths found") {
			continue
		}
		if strings.HasPrefix(trimmed, "results truncated") || strings.HasPrefix(trimmed, "matched path limit") {
			continue
		}
		total++
		if len(targets) < maxInspectionProgressTargets {
			targets = append(targets, trimmed)
		}
	}
	return targets, total
}
