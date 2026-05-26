package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type searchStructuredToolResult struct {
	Mode     string                           `json:"mode"`
	Notice   string                           `json:"notice"`
	Fallback bool                             `json:"fallback"`
	Results  []searchStructuredToolResultItem `json:"results"`
}

type searchStructuredToolResultItem struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
	Source  string `json:"source"`
}

type traceStructuredToolResult struct {
	Supported bool                            `json:"supported"`
	Found     bool                            `json:"found"`
	RootID    string                          `json:"root_id"`
	Nodes     []traceStructuredToolResultNode `json:"nodes"`
	Edges     []traceStructuredToolResultEdge `json:"edges"`
	Truncated bool                            `json:"truncated"`
}

type traceStructuredToolResultNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
}

type traceStructuredToolResultEdge struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
}

type refsStructuredToolResult struct {
	Supported                bool                          `json:"supported"`
	Found                    bool                          `json:"found"`
	Target                   traceStructuredToolResultNode `json:"target"`
	References               []refsStructuredToolReference `json:"references"`
	Truncated                bool                          `json:"truncated"`
	ClassificationSupported  bool                          `json:"classification_supported"`
	ClassificationIncomplete bool                          `json:"classification_incomplete"`
}

type refsStructuredToolReference struct {
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Character int    `json:"character"`
	Snippet   string `json:"snippet"`
}

type webSearchStructuredToolResult struct {
	Provider string                              `json:"provider"`
	Query    string                              `json:"query"`
	Notice   string                              `json:"notice"`
	Results  []webSearchStructuredToolResultItem `json:"results"`
}

type webSearchStructuredToolResultItem struct {
	Title       string  `json:"title"`
	URL         string  `json:"url"`
	Snippet     string  `json:"snippet"`
	Domain      string  `json:"domain"`
	PublishedAt string  `json:"published_at"`
	Author      string  `json:"author"`
	Score       float64 `json:"score"`
}

func parseSearchStructuredToolResult(call *events.ToolCallState) (searchStructuredToolResult, bool) {
	return parseStructuredToolResult[searchStructuredToolResult](call)
}

func parseTraceStructuredToolResult(call *events.ToolCallState) (traceStructuredToolResult, bool) {
	return parseStructuredToolResult[traceStructuredToolResult](call)
}

func parseRefsStructuredToolResult(call *events.ToolCallState) (refsStructuredToolResult, bool) {
	return parseStructuredToolResult[refsStructuredToolResult](call)
}

func parseWebSearchStructuredToolResult(call *events.ToolCallState) (webSearchStructuredToolResult, bool) {
	return parseStructuredToolResult[webSearchStructuredToolResult](call)
}

func parseStructuredToolResult[T any](call *events.ToolCallState) (T, bool) {
	var zero T
	if call == nil {
		return zero, false
	}
	trimmed := strings.TrimSpace(string(call.StructuredResult))
	if trimmed == "" || trimmed == "null" || !json.Valid([]byte(trimmed)) {
		return zero, false
	}
	var result T
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return zero, false
	}
	return result, true
}

func searchStructuredSourcesSummary(results []searchStructuredToolResultItem) string {
	if len(results) == 0 {
		return ""
	}
	counts := map[string]int{
		"lexical":  0,
		"semantic": 0,
		"merged":   0,
	}
	extra := make(map[string]int)
	for _, result := range results {
		source := strings.TrimSpace(result.Source)
		if source == "" {
			continue
		}
		if _, ok := counts[source]; ok {
			counts[source]++
			continue
		}
		extra[source]++
	}
	parts := make([]string, 0, len(counts)+len(extra))
	for _, source := range []string{"merged", "lexical", "semantic"} {
		if counts[source] > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", source, counts[source]))
		}
	}
	if len(extra) > 0 {
		keys := make([]string, 0, len(extra))
		for key := range extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s: %d", key, extra[key]))
		}
	}
	return strings.Join(parts, ", ")
}

func refsStructuredKindsSummary(references []refsStructuredToolReference) string {
	if len(references) == 0 {
		return ""
	}
	counts := map[string]int{
		"read":      0,
		"write":     0,
		"reference": 0,
	}
	extra := make(map[string]int)
	for _, reference := range references {
		kind := strings.TrimSpace(reference.Kind)
		if kind == "" {
			continue
		}
		if _, ok := counts[kind]; ok {
			counts[kind]++
			continue
		}
		extra[kind]++
	}
	parts := make([]string, 0, len(counts)+len(extra))
	for _, kind := range []string{"read", "write", "reference"} {
		if counts[kind] > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", kind, counts[kind]))
		}
	}
	if len(extra) > 0 {
		keys := make([]string, 0, len(extra))
		for key := range extra {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s: %d", key, extra[key]))
		}
	}
	return strings.Join(parts, ", ")
}

func traceStructuredStatus(result traceStructuredToolResult) string {
	switch {
	case !result.Supported:
		return "unsupported"
	case !result.Found:
		return "not found"
	case result.Truncated:
		return "truncated"
	default:
		return "ready"
	}
}

func refsStructuredStatus(result refsStructuredToolResult) string {
	switch {
	case !result.Supported:
		return "unsupported"
	case !result.Found:
		return "not found"
	case result.Truncated:
		return "truncated"
	default:
		return "ready"
	}
}

func webSearchStructuredDomainsSummary(results []webSearchStructuredToolResultItem) string {
	if len(results) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, result := range results {
		domain := strings.TrimSpace(result.Domain)
		if domain == "" {
			continue
		}
		counts[domain]++
	}
	if len(counts) == 0 {
		return ""
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] == counts[keys[j]] {
			return keys[i] < keys[j]
		}
		return counts[keys[i]] > counts[keys[j]]
	})
	const maxDomains = 3
	if len(keys) > maxDomains {
		keys = keys[:maxDomains]
	}
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		if counts[key] == 1 {
			parts = append(parts, key)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s (%d)", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}
