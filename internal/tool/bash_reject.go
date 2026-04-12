package tool

import (
	"regexp"
	"strings"
)

// heredocPattern matches heredoc syntax (<<EOF, <<'EOF', <<-EOF, << 'EOF').
var heredocPattern = regexp.MustCompile(`<<-?\s*'?\w+'?`)

// rejectBashFileOp returns a non-empty guidance message if the command uses a
// heredoc to write file content. Heredocs are almost never needed when the
// `write` tool is available. Other bash file operations (redirects, sed) are
// allowed since they have legitimate shell-specific uses.
func rejectBashFileOp(cmd string) string {
	if heredocPattern.MatchString(strings.TrimSpace(cmd)) {
		return "Use the `write` tool to create or rewrite files, or `edit`/`patch` to modify them. Do not use heredocs (<<EOF)."
	}
	return ""
}
