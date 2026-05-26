package tui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var assistantFindingRefPattern = regexp.MustCompile(`[A-Za-z0-9_./-]+\.[A-Za-z0-9_+-]+:\d+`)

type assistantFinding struct {
	title   string
	details []string
}

func renderAssistantContentLines(m Model, body string, width int, bg string) []string {
	return renderAssistantContentLinesWithStreamKey(m, body, width, bg, "")
}

func renderAssistantContentLinesWithStreamKey(m Model, body string, width int, bg string, streamKey string) []string {
	return cachedAssistantContentLines(m, body, width, bg, streamKey)
}

func renderAssistantContentLinesUncachedWithStreamKey(m Model, body string, width int, bg string, streamKey string) []string {
	blocks := splitAssistantBlocks(body)
	if len(blocks) == 0 {
		return renderAssistantMarkdownBlockWithStreamKey(m, body, width, bg, assistantContentBlockStreamKey(streamKey, 0))
	}

	lines := make([]string, 0, len(blocks)*4)
	for idx, block := range blocks {
		if idx > 0 && len(lines) > 0 {
			lines = append(lines, "")
		}
		if findings, ok := extractAssistantFindingsBlock(block); ok {
			lines = append(lines, renderAssistantFindingsBlock(m, findings, width, bg)...)
			continue
		}
		if looksLikeLiteralAssistantBlock(block) {
			lines = append(lines, renderLiteralLinesOnSurface(m, block, width, bg)...)
			continue
		}
		lines = append(lines, renderAssistantMarkdownBlockWithStreamKey(m, block, width, bg, assistantContentBlockStreamKey(streamKey, idx))...)
	}
	return lines
}

func looksLikeLiteralAssistantBlock(block string) bool {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	nonEmptyCount := 0
	commentCount := 0
	commandLikeCount := 0
	codeLikeComment := false
	sawHeredoc := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		nonEmptyCount++
		if looksLikeLiteralBoundary(trimmed) {
			sawHeredoc = true
			continue
		}
		if isLikelyShellCommandLine(trimmed) || looksLikeLiteralAssignment(trimmed) {
			commandLikeCount++
			continue
		}
		if strings.HasPrefix(trimmed, "# ") {
			commentCount++
			body := strings.TrimSpace(trimmed[2:])
			if body != "" && (startsWithDigit(body) || looksLikeSourceSnippet(body)) {
				codeLikeComment = true
			}
		}
	}

	if nonEmptyCount < 2 {
		return false
	}
	if sawHeredoc {
		return true
	}
	if commandLikeCount >= 2 {
		return true
	}
	if commandLikeCount > 0 && commentCount > 0 {
		return true
	}
	if commentCount == nonEmptyCount && codeLikeComment {
		return true
	}
	return false
}

func looksLikeLiteralBoundary(line string) bool {
	if strings.Contains(line, "<<") {
		return true
	}
	switch strings.TrimSpace(line) {
	case "EOF", "EOS", "EOT":
		return true
	default:
		return false
	}
}

func isLikelyShellCommandLine(line string) bool {
	for _, prefix := range []string{
		"cat ", "npm ", "pnpm ", "yarn ", "npx ", "node ", "go ", "git ",
		"mkdir ", "cp ", "mv ", "rm ", "touch ", "sed ", "awk ", "chmod ",
		"curl ", "docker ", "kubectl ", "uv ", "cargo ", "make ", "task ",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func looksLikeLiteralAssignment(line string) bool {
	if line == "" {
		return false
	}
	eq := strings.IndexByte(line, '=')
	if eq <= 0 {
		return false
	}
	for i := 0; i < eq; i++ {
		c := line[i]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			continue
		}
		return false
	}
	return true
}

func looksLikeSourceSnippet(line string) bool {
	return strings.ContainsAny(line, "{};") ||
		strings.Contains(line, "=>") ||
		strings.Contains(line, "::") ||
		strings.Contains(line, ".") && strings.Contains(line, "(") ||
		strings.Contains(line, "`")
}

func startsWithDigit(text string) bool {
	if text == "" {
		return false
	}
	return text[0] >= '0' && text[0] <= '9'
}

func splitAssistantBlocks(body string) []string {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	lines := strings.Split(body, "\n")
	blocks := make([]string, 0, len(lines)/2)
	current := make([]string, 0, len(lines))
	inFence := false

	flush := func() {
		if len(current) == 0 {
			return
		}
		block := strings.TrimSpace(strings.Join(current, "\n"))
		if block != "" {
			blocks = append(blocks, block)
		}
		current = current[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			current = append(current, line)
			inFence = !inFence
			continue
		}
		if !inFence && trimmed == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

func extractAssistantFindingsBlock(block string) ([]assistantFinding, bool) {
	lines := strings.Split(strings.TrimSpace(block), "\n")
	findings := make([]assistantFinding, 0, len(lines))
	denseCount := 0

	for i := 0; i < len(lines); {
		marker, titleText, ok := parseAssistantFindingStart(lines[i])
		_ = marker
		if !ok {
			return nil, false
		}
		originalTitle := titleText
		title, leadDetail := splitAssistantFindingLead(titleText)
		finding := assistantFinding{title: title}
		if leadDetail != "" {
			finding.details = append(finding.details, leadDetail)
		}

		i++
		for i < len(lines) {
			if _, _, ok := parseAssistantFindingStart(lines[i]); ok {
				break
			}
			detail := normalizeAssistantFindingText(stripAssistantFindingDetailPrefix(lines[i]))
			if detail != "" {
				finding.details = append(finding.details, detail)
			}
			i++
		}

		finding.details = normalizeAssistantFindingDetails(finding.details)
		if finding.title == "" {
			return nil, false
		}
		if len(finding.details) > 0 || assistantFindingRefPattern.MatchString(originalTitle) || len(originalTitle) >= 72 {
			denseCount++
		}
		findings = append(findings, finding)
	}

	if len(findings) < 2 || denseCount < 2 {
		return nil, false
	}
	return findings, true
}

func parseAssistantFindingStart(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* "):
		return trimmed[:1], normalizeAssistantFindingText(trimmed[2:]), true
	default:
		marker, rest := parseNumberedListItem(trimmed)
		if marker == "" {
			return "", "", false
		}
		return marker, normalizeAssistantFindingText(rest), true
	}
}

func splitAssistantFindingLead(line string) (string, string) {
	line = normalizeAssistantFindingText(line)
	for _, separator := range []string{": ", " - ", " – "} {
		if idx := strings.Index(line, separator); idx > 0 {
			return strings.TrimSpace(line[:idx]), strings.TrimSpace(line[idx+len(separator):])
		}
	}
	return line, ""
}

func stripAssistantFindingDetailPrefix(line string) string {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		return strings.TrimSpace(trimmed[2:])
	default:
		if _, rest, ok := parseAssistantFindingStart(trimmed); ok {
			return rest
		}
		return trimmed
	}
}

func normalizeAssistantFindingText(text string) string {
	text = strings.ReplaceAll(text, "\n", " ")
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func normalizeAssistantFindingDetails(details []string) []string {
	if len(details) == 0 {
		return nil
	}
	out := make([]string, 0, len(details))
	for _, detail := range details {
		detail = normalizeAssistantFindingText(detail)
		if detail != "" {
			out = append(out, detail)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func renderAssistantFindingsBlock(m Model, findings []assistantFinding, width int, bg string) []string {
	titleStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff"))).
		Bold(true)
	detailStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca")))
	bulletStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "secondary", "#7dcfff")))
	if strings.TrimSpace(bg) != "" {
		bgColor := lipgloss.Color(bg)
		titleStyle = titleStyle.Background(bgColor)
		detailStyle = detailStyle.Background(bgColor)
		bulletStyle = bulletStyle.Background(bgColor)
	}

	lines := make([]string, 0, len(findings)*3)
	for idx, finding := range findings {
		if idx > 0 {
			lines = append(lines, "")
		}
		lines = appendWrappedAssistantLine(lines, bulletStyle.Render("•")+" "+titleStyle.Render(renderInlineMarkdownOnSurface(m, finding.title, bg)), width, 0)
		for _, detail := range finding.details {
			lines = appendWrappedAssistantLine(lines, detailStyle.Render(renderInlineMarkdownOnSurface(m, detail, bg)), width, 2)
		}
	}
	return lines
}

func appendWrappedAssistantLine(lines []string, content string, width, indent int) []string {
	if strings.TrimSpace(ansi.Strip(content)) == "" {
		return lines
	}
	for _, line := range splitWrappedStyledLines(content, max(width-indent, 1)) {
		lines = append(lines, strings.Repeat(" ", indent)+line)
	}
	return lines
}
