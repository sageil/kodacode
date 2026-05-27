package tui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

var (
	readToolResultFooterPattern    = regexp.MustCompile(`^\(showing lines (\d+)-(\d+) of (\d+)`)
	readToolResultEOFFooterPattern = regexp.MustCompile(`^\(End of file - total (\d+) lines; shown lines (\d+)-(\d+)\)$`)
)

func groupedToolItemResultDetail(call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	if errorText := strings.TrimSpace(call.Error); errorText != "" {
		if isTaskToolCall(call) {
			return taskToolErrorSummary(call, errorText)
		}
		if failure := condenseMutationFailure(call); failure != nil {
			return strings.TrimSpace(failure.Status)
		}
		return ""
	}
	if call.Executing {
		return "(running...)"
	}
	if isTaskToolListCall(call) {
		return strings.TrimSpace(taskToolListSummary(call))
	}
	if presenter, ok := toolPresenterForCall(call); ok && presenter.ResultDetail != nil {
		return presenter.ResultDetail(call)
	}
	return ""
}

func readToolResultDetail(call *events.ToolCallState) string {
	if reused := reusedToolCallLabel(call); reused != "" {
		return strings.Replace(reused, ":", "", 1)
	}
	if call != nil {
		if input, ok := parseReadToolViewInput(call.Input); ok {
			if detail := readToolExplicitWindowDetail(input); detail != "" {
				return detail
			}
			if len(input.Paths) > 1 {
				return ""
			}
		}
	}
	output := strings.TrimSpace(call.Output)
	if output == "" {
		return ""
	}
	lines := strings.Split(output, "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if matches := readToolResultFooterPattern.FindStringSubmatch(last); len(matches) == 4 {
		return fmt.Sprintf("lines %s-%s", matches[1], matches[2])
	}
	if matches := readToolResultEOFFooterPattern.FindStringSubmatch(last); len(matches) == 4 {
		return fmt.Sprintf("lines %s-%s", matches[2], matches[3])
	}

	headerCount := 0
	numberedCount := 0
	firstNumberedLine := 0
	lastNumberedLine := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "=== ") && strings.HasSuffix(trimmed, " ==="):
			headerCount++
		case readToolResultIsNumberedLine(trimmed):
			lineNumber, _ := readToolResultNumberedLineNumber(trimmed)
			if firstNumberedLine == 0 {
				firstNumberedLine = lineNumber
			}
			lastNumberedLine = lineNumber
			numberedCount++
		}
	}
	switch {
	case headerCount <= 1 && firstNumberedLine > 0 && lastNumberedLine > 0:
		return fmt.Sprintf("lines %d-%d", firstNumberedLine, lastNumberedLine)
	case headerCount > 1 && numberedCount > 0:
		return fmt.Sprintf("%d files · %s returned", headerCount, pluralize(numberedCount, "line"))
	case headerCount > 1:
		return pluralize(headerCount, "file")
	case numberedCount > 0:
		return pluralize(numberedCount, "line") + " returned"
	default:
		return summarizeInlineValue(output)
	}
}

func readToolExplicitWindowDetail(input readToolViewInput) string {
	var parts []string
	if input.HasOffset {
		parts = append(parts, fmt.Sprintf("offset %d", input.Offset))
	}
	if input.HasLimit {
		parts = append(parts, fmt.Sprintf("limit %d", input.Limit))
	}
	return strings.Join(parts, " · ")
}

func readToolResultIsNumberedLine(line string) bool {
	if line == "" {
		return false
	}
	digits := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}
	return digits > 0 && digits+1 < len(line) && line[digits] == ':' && line[digits+1] == ' '
}

func readToolResultNumberedLineNumber(line string) (int, bool) {
	digits := 0
	value := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		value = value*10 + int(line[digits]-'0')
		digits++
	}
	if digits == 0 || digits+1 >= len(line) || line[digits] != ':' || line[digits+1] != ' ' {
		return 0, false
	}
	return value, true
}

func searchToolResultDetail(call *events.ToolCallState) string {
	if result, ok := parseSearchStructuredToolResult(call); ok {
		detail := searchMatchCountLabel(len(result.Results))
		if len(result.Results) == 0 {
			detail = "no matches"
		}
		if result.Fallback {
			return detail + " · fallback"
		}
		return detail
	}
	output := strings.TrimSpace(call.Output)
	if output == "" {
		return ""
	}
	lines := searchNonNoticeLines(output)
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 && strings.EqualFold(strings.TrimSpace(lines[0]), "no matches found") {
		return "no matches"
	}
	return searchMatchCountLabel(len(lines))
}

func webSearchToolResultDetail(call *events.ToolCallState) string {
	result, ok := parseWebSearchStructuredToolResult(call)
	if !ok {
		return ""
	}
	if len(result.Results) == 0 {
		if provider := strings.TrimSpace(result.Provider); provider != "" {
			return "no results · " + provider
		}
		return "no results"
	}
	parts := []string{pluralize(len(result.Results), "result")}
	if provider := strings.TrimSpace(result.Provider); provider != "" {
		parts = append(parts, provider)
	}
	if strings.TrimSpace(result.Notice) != "" {
		parts = append(parts, "notice")
	}
	return strings.Join(parts, " · ")
}

func traceToolResultDetail(call *events.ToolCallState) string {
	result, ok := parseTraceStructuredToolResult(call)
	if !ok {
		return ""
	}
	switch {
	case !result.Supported:
		return "unsupported"
	case !result.Found:
		return "not found"
	}
	parts := []string{pluralize(len(result.Nodes), "node")}
	if len(result.Edges) > 0 {
		parts = append(parts, pluralize(len(result.Edges), "edge"))
	}
	if result.Truncated {
		parts = append(parts, "truncated")
	}
	return strings.Join(parts, " · ")
}

func refsToolResultDetail(call *events.ToolCallState) string {
	result, ok := parseRefsStructuredToolResult(call)
	if !ok {
		return ""
	}
	switch {
	case !result.Supported:
		return "unsupported"
	case !result.Found:
		return "not found"
	}
	parts := []string{pluralize(len(result.References), "ref")}
	if result.Truncated {
		parts = append(parts, "truncated")
	}
	if result.ClassificationIncomplete {
		parts = append(parts, "partial")
	}
	return strings.Join(parts, " · ")
}

func searchMatchCountLabel(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

func locateToolResultDetail(call *events.ToolCallState) string {
	output := strings.TrimSpace(call.Output)
	if output == "" {
		return ""
	}
	lines := searchNonNoticeLines(output)
	if len(lines) == 0 {
		return ""
	}
	if len(lines) == 1 && strings.EqualFold(strings.TrimSpace(lines[0]), "no paths found") {
		return "no paths"
	}
	return pluralize(len(lines), "path")
}

func listToolResultDetail(call *events.ToolCallState) string {
	lines := nonEmptyOutputLines(call.Output)
	if len(lines) == 0 {
		return ""
	}
	return pluralize(len(lines), "entry")
}

func treeToolResultDetail(call *events.ToolCallState) string {
	lines := nonEmptyOutputLines(call.Output)
	if len(lines) == 0 {
		return ""
	}
	entryCount := 0
	truncated := false
	for idx, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case idx == 0:
			continue
		case strings.Contains(trimmed, "truncated after"):
			truncated = true
		default:
			entryCount++
		}
	}
	if entryCount <= 0 {
		if truncated {
			return "truncated"
		}
		return ""
	}
	if truncated {
		return fmt.Sprintf("%s shown", pluralize(entryCount, "entry"))
	}
	return pluralize(entryCount, "entry")
}

func gitStatusToolResultDetail(call *events.ToolCallState) string {
	lines := nonEmptyOutputLines(call.Output)
	if len(lines) == 0 {
		return ""
	}
	for _, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "status: clean") {
			return "clean"
		}
	}
	statusIndex := -1
	for idx, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "status:") {
			statusIndex = idx
			break
		}
	}
	if statusIndex < 0 || statusIndex+1 >= len(lines) {
		return ""
	}
	return pluralize(len(lines)-statusIndex-1, "entry")
}

func gitDiffToolResultDetail(call *events.ToolCallState) string {
	output := strings.TrimSpace(call.Output)
	switch {
	case output == "":
		return ""
	case strings.Contains(output, "status: clean"):
		return "clean"
	case strings.Contains(output, "[output truncated]"):
		return "diff truncated"
	default:
		return "diff available"
	}
}

func gitShowToolResultDetail(call *events.ToolCallState) string {
	output := strings.TrimSpace(call.Output)
	switch {
	case output == "":
		return ""
	case strings.Contains(output, "status: no workspace-scoped output"):
		return "no workspace-scoped output"
	case strings.Contains(output, "[output truncated]"):
		return "output truncated"
	default:
		return "output available"
	}
}

func searchNonNoticeLines(output string) []string {
	lines := nonEmptyOutputLines(output)
	if len(lines) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "notice: ") {
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func nonEmptyOutputLines(output string) []string {
	raw := strings.Split(strings.TrimSpace(output), "\n")
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}
