package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func isBinaryContent(s string) bool {
	if len(s) == 0 {
		return false
	}
	sample := s
	if len(sample) > 512 {
		sample = sample[:512]
	}
	nonPrintable := 0
	for _, b := range []byte(sample) {
		if b == 0 {
			return true
		}
		if b < 32 && b != '\n' && b != '\r' && b != '\t' {
			nonPrintable++
		}
	}
	return nonPrintable > len(sample)/4
}

func isMediaOutput(s string) bool {
	return strings.Contains(s, "<type>media</type>") || strings.Contains(s, "data:image/") || strings.Contains(s, "data:application/pdf")
}

func formatElapsed(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

const spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"

var (
	reError = regexp.MustCompile(`(?i)(error|fatal|panic|fail)`)
	reWarn  = regexp.MustCompile(`(?i)(warn(ing)?)`)
)

func colorizeOutputLine(line string, errColor, warnColor, subColor lipgloss.Style) string {
	switch {
	case reError.MatchString(line):
		return errColor.Render(line)
	case reWarn.MatchString(line):
		return warnColor.Render(line)
	default:
		return subColor.Render(line)
	}
}

func stripXMLContent(s string) string {
	const open = "<content>"
	const close = "</content>"
	_, after, ok := strings.Cut(s, open)
	if !ok {
		return s
	}
	inner, _, ok := strings.Cut(after, close)
	if !ok {
		return strings.TrimLeft(after, "\n")
	}
	return strings.TrimLeft(inner, "\n")
}

func toolDisplayName(name string) string {
	switch name {
	case "bash":
		return "Bash"
	case "read":
		return "Read"
	case "write":
		return "Write"
	case "edit":
		return "Edit"
	case "patch":
		return "Patch"
	case "glob":
		return "Glob"
	case "grep":
		return "Grep"
	case "search":
		return "Search"
	case "tree":
		return "Tree"
	case "lsp":
		return "LSP"
	case "git":
		return "Git"
	case "test":
		return "Test"
	case "task":
		return "Task"
	case "open":
		return "Open"
	case "web_fetch":
		return "Web Fetch"
	case "question":
		return "Question"
	case "subagent":
		return "Subagent"
	case "task_output":
		return "Background"
	case "skill":
		return "Skill"
	case "read_files":
		return "ReadFiles"
	case "code_action":
		return "CodeAction"
	case "rename_symbol":
		return "RenameSymbol"
	case "intent":
		return "Workflow"
	default:
		if len(name) == 0 {
			return name
		}
		if idx := strings.IndexByte(name, '_'); idx > 0 {
			name = name[:idx]
		}
		return titleCase(name)
	}
}

func titleCase(s string) string {
	words := strings.Split(s, "-")
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, "-")
}

func formatAgentName(id string) string {
	if len(id) == 0 {
		return id
	}
	return strings.ToUpper(id[:1]) + id[1:]
}

func collapseBlankLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	prevBlank := false
	for _, line := range lines {
		blank := strings.TrimSpace(line) == ""
		if blank && prevBlank {
			continue
		}
		out = append(out, line)
		prevBlank = blank
	}
	return out
}

func splitSymbolLine(line string) []string {
	var parts []string
	start := 0
	inSpaces := false
	spaceStart := 0
	for i, c := range line {
		if c == ' ' {
			if !inSpaces {
				inSpaces = true
				spaceStart = i
			}
		} else {
			if inSpaces && i-spaceStart >= 2 {
				parts = append(parts, strings.TrimSpace(line[start:spaceStart]))
				start = i
			}
			inSpaces = false
		}
	}
	if start < len(line) {
		parts = append(parts, strings.TrimSpace(line[start:]))
	}
	return parts
}

func splitPathName(path string) (dir, file string) {
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[:idx+1], path[idx+1:]
	}
	return "", path
}

func isMCPTool(name string) bool {
	if !strings.ContainsRune(name, '_') {
		return false
	}
	switch name {
	case "web_fetch", "task_output", "read_files":
		return false
	}
	return true
}

func isTaskListOutput(output string) bool {
	return strings.HasPrefix(output, "[")
}

func isReadOnlyTool(name string) bool {
	switch name {
	case "read", "grep", "glob", "search", "tree", "task", "task_output", "skill":
		return true
	}
	return false
}

func isReadOnlyToolCall(name, input string) bool {
	if isReadOnlyTool(name) {
		return true
	}
	if name != "bash" {
		return false
	}
	cmd := extractBashCommand(input)
	if cmd == "" {
		return false
	}
	return isReadOnlyBashCommand(cmd)
}

func isReadOnlyBashCommand(cmd string) bool {
	return tool.IsReadOnlyBashCommand(cmd)
}

func extractBashCommand(input string) string {
	var m struct {
		Command string `json:"command"`
	}
	if json.Unmarshal([]byte(input), &m) != nil {
		return ""
	}
	return strings.TrimSpace(m.Command)
}

func isStreamingDiffTool(name string) bool {
	return name == "write" || name == "edit" || name == "patch"
}

var reLineNum = regexp.MustCompile(`^(\s*\d+[:\t] ?)`)

func stripLineNumberPrefixes(lines []string) (prefixes []string, code []string) {
	prefixes = make([]string, len(lines))
	code = make([]string, len(lines))
	for i, line := range lines {
		if m := reLineNum.FindString(line); m != "" {
			prefixes[i] = m
			code[i] = line[len(m):]
		} else {
			code[i] = line
		}
	}
	return
}

func (m *Messages) highlightWithLineNumbers(body *strings.Builder, lines []string, lang string, lineNumStyle, dimStyle lipgloss.Style, textWidth int) {
	prefixes, codeLines := stripLineNumberPrefixes(lines)
	prefixWidth := 0
	for _, p := range prefixes {
		if len(p) > prefixWidth {
			prefixWidth = len(p)
		}
	}
	codeWidth := textWidth - prefixWidth - 2
	if codeWidth < 20 {
		codeWidth = 20
	}

	if lang != "" {
		highlighted := m.cachedHighlight(strings.Join(codeLines, "\n"), lang)
		hlLines := strings.Split(strings.TrimRight(highlighted, "\n"), "\n")
		for i, hl := range hlLines {
			prefix := ""
			if i < len(prefixes) && prefixes[i] != "" {
				prefix = lineNumStyle.Render(prefixes[i])
			}
			hl = ansi.Truncate(hl, codeWidth, "…")
			fmt.Fprintf(body, "  %s%s\n", prefix, hl)
		}
	} else {
		for i, cl := range codeLines {
			prefix := ""
			if i < len(prefixes) && prefixes[i] != "" {
				prefix = lineNumStyle.Render(prefixes[i])
			}
			cl = ansi.Truncate(cl, codeWidth, "…")
			fmt.Fprintf(body, "  %s%s\n", prefix, dimStyle.Render(cl))
		}
	}
}
