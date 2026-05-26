package app

import "strings"

func normalizeCompactionSectionLabel(line string) string {
	line = strings.TrimSpace(line)
	for _, prefix := range []string{"### ", "## ", "# "} {
		if strings.HasPrefix(line, prefix) {
			line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
			break
		}
	}
	line = strings.TrimSpace(strings.Trim(line, "*"))
	line = strings.TrimSuffix(line, ":")
	return strings.ToLower(strings.TrimSpace(line))
}

func parseCompactionSummarySection(line string) string {
	switch normalizeCompactionSectionLabel(line) {
	case "goal":
		return "goal"
	case "constraints & preferences", "constraints and preferences", "constraints", "preferences":
		return "constraint"
	case "progress":
		return "progress"
	case "done", "completed":
		return "done"
	case "in progress", "in-progress":
		return "in_progress"
	case "blocked":
		return "blocked"
	case "key decisions", "key decision", "decisions", "decision":
		return "decision"
	case "next", "next step", "next steps":
		return "next"
	case "critical context", "critical":
		return "critical"
	case "relevant files", "relevant file", "files", "file":
		return "file"
	default:
		return ""
	}
}

func parseCompactionSummaryLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "- ")
	line = strings.TrimPrefix(line, "* ")
	lower := strings.ToLower(line)
	switch {
	case strings.HasPrefix(lower, "goal:"):
		return "goal", strings.TrimSpace(line[len("goal:"):]), true
	case strings.HasPrefix(lower, "constraints & preferences:"):
		return "constraint", strings.TrimSpace(line[len("constraints & preferences:"):]), true
	case strings.HasPrefix(lower, "constraints and preferences:"):
		return "constraint", strings.TrimSpace(line[len("constraints and preferences:"):]), true
	case strings.HasPrefix(lower, "constraints:"):
		return "constraint", strings.TrimSpace(line[len("constraints:"):]), true
	case strings.HasPrefix(lower, "constraint:"):
		return "constraint", strings.TrimSpace(line[len("constraint:"):]), true
	case strings.HasPrefix(lower, "preferences:"):
		return "constraint", strings.TrimSpace(line[len("preferences:"):]), true
	case strings.HasPrefix(lower, "preference:"):
		return "constraint", strings.TrimSpace(line[len("preference:"):]), true
	case strings.HasPrefix(lower, "done:"):
		return "done", strings.TrimSpace(line[len("done:"):]), true
	case strings.HasPrefix(lower, "completed:"):
		return "done", strings.TrimSpace(line[len("completed:"):]), true
	case strings.HasPrefix(lower, "in progress:"):
		return "in_progress", strings.TrimSpace(line[len("in progress:"):]), true
	case strings.HasPrefix(lower, "blocked:"):
		return "blocked", strings.TrimSpace(line[len("blocked:"):]), true
	case strings.HasPrefix(lower, "pending user decision:"):
		return "blocked", strings.TrimSpace(line[len("pending user decision:"):]), true
	case strings.HasPrefix(lower, "pending decision:"):
		return "blocked", strings.TrimSpace(line[len("pending decision:"):]), true
	case strings.HasPrefix(lower, "key decisions:"):
		return "decision", strings.TrimSpace(line[len("key decisions:"):]), true
	case strings.HasPrefix(lower, "key decision:"):
		return "decision", strings.TrimSpace(line[len("key decision:"):]), true
	case strings.HasPrefix(lower, "decisions:"):
		return "decision", strings.TrimSpace(line[len("decisions:"):]), true
	case strings.HasPrefix(lower, "decision:"):
		return "decision", strings.TrimSpace(line[len("decision:"):]), true
	case strings.HasPrefix(lower, "critical context:"):
		return "critical", strings.TrimSpace(line[len("critical context:"):]), true
	case strings.HasPrefix(lower, "critical:"):
		return "critical", strings.TrimSpace(line[len("critical:"):]), true
	case strings.HasPrefix(lower, "fact:"):
		return "critical", strings.TrimSpace(line[len("fact:"):]), true
	case strings.HasPrefix(lower, "facts:"):
		return "critical", strings.TrimSpace(line[len("facts:"):]), true
	case strings.HasPrefix(lower, "state:"):
		return "state", strings.TrimSpace(line[len("state:"):]), true
	case strings.HasPrefix(lower, "issues:"):
		return "issue", strings.TrimSpace(line[len("issues:"):]), true
	case strings.HasPrefix(lower, "issue:"):
		return "issue", strings.TrimSpace(line[len("issue:"):]), true
	case strings.HasPrefix(lower, "risks:"):
		return "issue", strings.TrimSpace(line[len("risks:"):]), true
	case strings.HasPrefix(lower, "risk:"):
		return "issue", strings.TrimSpace(line[len("risk:"):]), true
	case strings.HasPrefix(lower, "relevant files:"):
		return "file", strings.TrimSpace(line[len("relevant files:"):]), true
	case strings.HasPrefix(lower, "relevant file:"):
		return "file", strings.TrimSpace(line[len("relevant file:"):]), true
	case strings.HasPrefix(lower, "file:"):
		return "file", strings.TrimSpace(line[len("file:"):]), true
	case strings.HasPrefix(lower, "files:"):
		return "file", strings.TrimSpace(line[len("files:"):]), true
	case strings.HasPrefix(lower, "next step:"):
		return "next", strings.TrimSpace(line[len("next step:"):]), true
	case strings.HasPrefix(lower, "next steps:"):
		return "next", strings.TrimSpace(line[len("next steps:"):]), true
	case strings.HasPrefix(lower, "next:"):
		return "next", strings.TrimSpace(line[len("next:"):]), true
	default:
		return "", "", false
	}
}

func isCompactionMetadataLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	switch {
	case lower == "":
		return true
	case strings.HasPrefix(lower, strings.ToLower(historyContinuationSummaryHeader)):
		return true
	case strings.HasPrefix(lower, "prior-turn context:"):
		return true
	case strings.HasPrefix(lower, "scope:"):
		return true
	case strings.HasPrefix(lower, "files touched:"):
		return true
	case strings.HasPrefix(lower, "measurement:"):
		return true
	case strings.HasPrefix(lower, "summary source:"):
		return true
	case strings.HasPrefix(lower, "previously compacted context:"):
		return true
	case strings.HasPrefix(lower, "newly compacted turns:"):
		return true
	case strings.HasPrefix(lower, "newly compacted facts:"):
		return true
	case looksLikeCompactionTokenDelta(lower):
		return true
	default:
		return false
	}
}

func looksLikeCompactionTokenDelta(line string) bool {
	if !strings.Contains(line, "->") {
		return false
	}
	if !strings.Contains(line, "token") {
		return false
	}
	return true
}
