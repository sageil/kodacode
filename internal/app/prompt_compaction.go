package app

import (
	"regexp"
	"strings"

	"github.com/sageil/kodacode/internal/prompt"
)

const (
	providerPromptCompactionMinBytes   = 256
	providerPromptCompactionMinSavings = 32
)

var orderedListMarkerPattern = regexp.MustCompile(`^\d+[.)]\s+`)

func compactProviderPromptFragments(fragments []prompt.Fragment) []prompt.Fragment {
	if len(fragments) == 0 {
		return nil
	}
	out := make([]prompt.Fragment, len(fragments))
	copy(out, fragments)
	for index := range out {
		out[index].ProviderContent = compactProviderPromptFragmentContent(out[index])
	}
	return out
}

func compactProviderPromptFragmentContent(fragment prompt.Fragment) string {
	content := strings.TrimSpace(fragment.Content)
	if !shouldCompactProviderPromptFragment(fragment, content) {
		return ""
	}
	compacted := strings.TrimSpace(compactProviderInstructionText(content))
	if compacted == "" || len(compacted)+providerPromptCompactionMinSavings > len(content) {
		return ""
	}
	return compacted
}

func shouldCompactProviderPromptFragment(fragment prompt.Fragment, content string) bool {
	if len(content) < providerPromptCompactionMinBytes {
		return false
	}
	switch fragment.Kind {
	case prompt.KindPolicy, prompt.KindRole, prompt.KindTooling, prompt.KindRepo, prompt.KindMemory, prompt.KindRuntime:
		return looksStructuredInstructionText(content)
	default:
		return false
	}
}

func looksStructuredInstructionText(text string) bool {
	headings := 0
	lists := 0
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case markdownHeadingText(trimmed) != "":
			headings++
		case looksLikeInstructionSectionLine(trimmed):
			headings++
		case instructionListItemText(trimmed) != "":
			lists++
		}
	}
	return headings+lists >= 3
}

func compactProviderInstructionText(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	currentSection := ""
	inCodeFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			continue
		case strings.HasPrefix(trimmed, "```"):
			inCodeFence = !inCodeFence
			continue
		case inCodeFence:
			appendCompactPromptLine(&out, seen, trimmed)
			continue
		case isInstructionHorizontalRule(trimmed):
			continue
		}

		if heading := markdownHeadingText(trimmed); heading != "" {
			currentSection = compactInstructionSectionLabel(heading)
			appendCompactPromptSection(&out, seen, currentSection)
			continue
		}
		if looksLikeInstructionSectionLine(trimmed) {
			currentSection = compactInstructionSectionLabel(strings.TrimSuffix(trimmed, ":"))
			appendCompactPromptSection(&out, seen, currentSection)
			continue
		}

		item := instructionListItemText(trimmed)
		if item == "" {
			item = trimmed
		}
		item = normalizePromptCompactionText(item)
		if item == "" || strings.EqualFold(item, "(none)") {
			continue
		}
		if currentSection != "" {
			appendCompactPromptLine(&out, seen, "- "+item)
			continue
		}
		appendCompactPromptLine(&out, seen, item)
	}
	return strings.Join(out, "\n")
}

func appendCompactPromptSection(out *[]string, seen map[string]struct{}, section string) {
	section = normalizePromptCompactionText(section)
	if section == "" {
		return
	}
	appendCompactPromptLine(out, seen, section+":")
}

func appendCompactPromptLine(out *[]string, seen map[string]struct{}, line string) {
	line = normalizePromptCompactionText(line)
	if line == "" {
		return
	}
	if _, ok := seen[line]; ok {
		return
	}
	seen[line] = struct{}{}
	*out = append(*out, line)
}

func markdownHeadingText(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"### ", "## ", "# "} {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func looksLikeInstructionSectionLine(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasSuffix(line, ":") {
		return false
	}
	line = strings.TrimSuffix(line, ":")
	if line == "" || strings.Contains(line, ".") {
		return false
	}
	return !strings.HasPrefix(line, "- ") && !strings.HasPrefix(line, "* ") && !strings.HasPrefix(line, "+ ")
}

func instructionListItemText(line string) string {
	line = strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(line, "- "):
		return strings.TrimSpace(strings.TrimPrefix(line, "- "))
	case strings.HasPrefix(line, "* "):
		return strings.TrimSpace(strings.TrimPrefix(line, "* "))
	case strings.HasPrefix(line, "+ "):
		return strings.TrimSpace(strings.TrimPrefix(line, "+ "))
	case orderedListMarkerPattern.MatchString(line):
		return strings.TrimSpace(orderedListMarkerPattern.ReplaceAllString(line, ""))
	default:
		return ""
	}
}

func isInstructionHorizontalRule(line string) bool {
	line = strings.TrimSpace(line)
	return line == "---" || line == "***"
}

func compactInstructionSectionLabel(label string) string {
	normalized := normalizePromptCompactionText(label)
	switch strings.ToLower(normalized) {
	case "core standard":
		return "Standard"
	case "non-negotiables":
		return "Non-negotiables"
	case "engineering priorities":
		return "Priorities"
	case "design rules":
		return "Design"
	case "cost and user value":
		return "Cost"
	case "implementation rules":
		return "Implementation"
	case "migration posture":
		return "Migration"
	case "constraints & preferences", "constraints and preferences":
		return "Constraints"
	case "key decisions":
		return "Decisions"
	case "next steps":
		return "Next"
	case "critical context":
		return "Context"
	case "relevant files":
		return "Files"
	default:
		return normalized
	}
}

func normalizePromptCompactionText(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}
