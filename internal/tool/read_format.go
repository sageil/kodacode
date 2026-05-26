package tool

import (
	"fmt"
	"strings"
)

func formatReadOutput(results []readResult, withHeaders bool) string {
	if len(results) == 0 {
		return ""
	}

	sections := make([]string, 0, len(results))
	for _, result := range results {
		sections = append(sections, formatReadSection(result, withHeaders))
	}
	return strings.Join(sections, "\n\n")
}

func formatReadSection(result readResult, withHeader bool) string {
	body := formatReadTaggedSection(result)
	if !withHeader {
		return body
	}
	return fmt.Sprintf("=== %s ===\n%s", result.path, body)
}

func formatReadTaggedSection(result readResult) string {
	return fmt.Sprintf("<path>%s</path>\n<type>file</type>\n<content>\n%s\n</content>", result.path, result.body)
}

func formatReadFailures(failures []readFailure) string {
	if len(failures) == 0 {
		return ""
	}
	lines := make([]string, 0, len(failures)+1)
	if len(failures) == 1 {
		lines = append(lines, "read failed for 1 path:")
	} else {
		lines = append(lines, fmt.Sprintf("read failed for %d paths:", len(failures)))
	}
	for _, failure := range failures {
		lines = append(lines, failure.path+": "+failure.error)
	}
	return strings.Join(lines, "\n")
}

func formatPartialReadOutput(output string, successful, requested int, failures []readFailure) string {
	if len(failures) == 0 {
		return output
	}
	lines := make([]string, 0, len(failures)+4)
	lines = append(lines, fmt.Sprintf("Partial read: %d of %d requested files read successfully.", successful, requested))
	if len(failures) == 1 {
		lines = append(lines, "Failed path:")
	} else {
		lines = append(lines, "Failed paths:")
	}
	for _, failure := range failures {
		lines = append(lines, "- "+failure.path+": "+failure.error)
	}
	lines = append(lines, "If needed, correct or replace only the failed path.")
	warning := strings.Join(lines, "\n")
	if strings.TrimSpace(output) == "" {
		return warning
	}
	return warning + "\n\n" + output
}

func readWindowFooter(startLine, endLine, totalLines int, truncated bool) string {
	if truncated {
		return fmt.Sprintf("(showing lines %d-%d of %d. Use offset=%d (0-based) to continue.)", startLine, endLine, totalLines, endLine)
	}
	if endLine >= totalLines {
		return fmt.Sprintf("(End of file - total %d lines; shown lines %d-%d)", totalLines, startLine, endLine)
	}
	return fmt.Sprintf("(showing lines %d-%d of %d)", startLine, endLine, totalLines)
}

func trimReadLine(rawLine string) string {
	line := strings.TrimSuffix(rawLine, "\n")
	return strings.TrimSuffix(line, "\r")
}
