package tui

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

func toolHintText(msg Message) string {
	if !msg.ToolDone {
		return "running…"
	}
	if msg.ToolError != "" {
		if strings.Contains(msg.ToolError, "denied") {
			return "denied"
		}
		return "failed"
	}
	switch msg.ToolName {
	case "bash":
		if strings.HasPrefix(strings.TrimSpace(msg.ToolOutput), "error:") {
			return "failed"
		}
		return "done"

	case "task":
		return taskHintText(msg)

	case "task_output":
		if status := extractTaskStatusField(msg.ToolOutput, "status"); status != "" {
			return status
		}
		return "done"

	case "read":
		// Directory detection
		if strings.Contains(msg.ToolOutput, "<type>directory</type>") {
			content := stripXMLContent(msg.ToolOutput)
			n := len(strings.Split(strings.TrimRight(content, "\n"), "\n"))
			if content == "" {
				n = 0
			}
			return fmt.Sprintf("%d entries", n)
		}
		// File
		content := stripXMLContent(msg.ToolOutput)
		lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
		n := len(lines)
		if content == "" {
			n = 0
		}
		// Get file path from tool input for language detection
		var fields map[string]any
		lang := ""
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err == nil {
			if fp, ok := fields["filePath"].(string); ok {
				lang = detectLanguage(fp)
			}
		}
		if lang != "" {
			return fmt.Sprintf("%d lines · %s", n, lang)
		}
		return fmt.Sprintf("%d lines", n)

	case "write":
		// For write, extract line count from the "content" field of ToolInput.
		var fields map[string]any
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err != nil {
			return ""
		}
		newContent, _ := fields["content"].(string)
		if newContent == "" {
			return ""
		}
		newLines := len(strings.Split(strings.TrimRight(newContent, "\n"), "\n"))
		// When overwriting, ToolOutput contains the old content — show diff stats.
		if msg.ToolOutput != "" && msg.ToolOutput != "Wrote file successfully." {
			oldLines := len(strings.Split(strings.TrimRight(msg.ToolOutput, "\n"), "\n"))
			return fmt.Sprintf("-%d +%d lines", oldLines, newLines)
		}
		lang := ""
		if fp, ok := fields["filePath"].(string); ok {
			lang = detectLanguage(fp)
		}
		if lang != "" {
			return fmt.Sprintf("%d lines · %s", newLines, lang)
		}
		return fmt.Sprintf("%d lines", newLines)

	case "edit":
		// For edit, count changed lines from oldString/newString.
		var fields map[string]any
		if err := json.Unmarshal([]byte(msg.ToolInput), &fields); err != nil {
			return ""
		}
		removed, added := 0, 0
		if s, ok := fields["oldString"].(string); ok && s != "" {
			removed = len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
		}
		if s, ok := fields["newString"].(string); ok && s != "" {
			added = len(strings.Split(strings.TrimRight(s, "\n"), "\n"))
		}
		if removed == 0 && added == 0 {
			return "edited"
		}
		return fmt.Sprintf("-%d +%d lines", removed, added)

	case "lsp":
		out := strings.TrimSpace(msg.ToolOutput)
		if strings.HasPrefix(out, "No diagnostics") {
			return "clean"
		}
		first, _, _ := strings.Cut(out, "\n")
		if first != "" {
			return first
		}
		return "done"

	case "search":
		// Extract count from "Found N results for ..." header.
		if strings.HasPrefix(msg.ToolOutput, "Found ") {
			if idx := strings.Index(msg.ToolOutput, " results"); idx > 0 {
				count := msg.ToolOutput[len("Found "):idx]
				return count + " results"
			}
		}
		if strings.HasPrefix(msg.ToolOutput, "No results") {
			return "0 results"
		}
		return "done"

	case "git":
		return gitHintText(msg.ToolOutput)

	case "glob", "grep":
		if msg.ToolOutput == "" {
			return "0 results"
		}
		lines := strings.Split(strings.TrimRight(msg.ToolOutput, "\n"), "\n")
		return fmt.Sprintf("%d results", len(lines))

	case "subagent":
		var parts []string
		if steps := len(msg.SubagentActivities); steps > 0 {
			parts = append(parts, fmt.Sprintf("%d steps", steps))
		}
		if out := strings.TrimSpace(msg.ToolOutput); out != "" {
			if n := len([]rune(out)); n > 1000 {
				parts = append(parts, fmt.Sprintf("%.1fk chars", float64(n)/1000))
			} else {
				parts = append(parts, fmt.Sprintf("%d chars", n))
			}
		}
		if len(parts) == 0 {
			return "completed"
		}
		return "completed · " + strings.Join(parts, " · ")

	default:
		if msg.ToolOutput != "" {
			return "done"
		}
		return ""
	}
}

func gitHintText(output string) string {
	if output == "" {
		return "clean"
	}
	var modified, added, deleted, other int
	for line := range strings.SplitSeq(output, "\n") {
		if len(line) < 3 || line[2] != ' ' {
			continue
		}
		marker := line[:2]
		switch {
		case marker[0] == 'M' || marker[1] == 'M':
			modified++
		case marker[0] == 'A' || marker[1] == 'A':
			added++
		case marker[0] == 'D' || marker[1] == 'D':
			deleted++
		default:
			other++
		}
	}
	total := modified + added + deleted + other
	if total == 0 {
		return "done"
	}
	var parts []string
	if modified > 0 {
		parts = append(parts, fmt.Sprintf("%dM", modified))
	}
	if added > 0 {
		parts = append(parts, fmt.Sprintf("%dA", added))
	}
	if deleted > 0 {
		parts = append(parts, fmt.Sprintf("%dD", deleted))
	}
	if other > 0 {
		parts = append(parts, fmt.Sprintf("%d?", other))
	}
	return strings.Join(parts, " ") + fmt.Sprintf(" · %d files", total)
}

// toolSummary holds the parsed human-readable fields extracted from a tool's
// raw JSON input.
type toolSummary struct {
	summary string
	args    string
	command string
}

// parseToolSummary extracts a human-readable summary from a tool's raw JSON
// input string.
func parseToolSummary(toolName, input string) toolSummary {
	var fields map[string]any
	if err := json.Unmarshal([]byte(input), &fields); err != nil {
		// Partial JSON (still streaming) — try extracting known fields.
		partial := parsePartialToolInput(input)
		if fp, ok := partial["filePath"]; ok && fp != "" {
			return toolSummary{summary: (fp)}
		}
		if desc, ok := partial["description"]; ok && desc != "" {
			return toolSummary{summary: desc}
		}
		if action, ok := partial["action"]; ok && action != "" {
			summary := action
			if title, ok := partial["title"]; ok && title != "" {
				summary += ": " + truncate(title, 50)
			}
			return toolSummary{summary: summary}
		}
		return toolSummary{}
	}

	str := func(key string) string {
		if v, ok := fields[key]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}
	intField := func(key string) int {
		if v, ok := fields[key]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			case int:
				return n
			}
		}
		return 0
	}

	switch toolName {
	case "intent":
		return toolSummary{summary: "Selecting workflow"}

	case "bash":
		desc := str("description")
		cmd := str("command")
		hasDesc := desc != ""
		if !hasDesc {
			desc = truncate(cmd, 60)
		}
		if hasDesc {
			cmd = ""
		}
		return toolSummary{summary: desc, command: cmd}

	case "read":
		fp := str("filePath")
		s := (fp)
		var argParts []string
		if off := intField("offset"); off > 1 {
			argParts = append(argParts, fmt.Sprintf("offset=%d", off))
		}
		if lim := intField("limit"); lim > 0 && lim <= 200 {
			argParts = append(argParts, fmt.Sprintf("limit=%d", lim))
		}
		args := ""
		if len(argParts) > 0 {
			args = "[" + strings.Join(argParts, ", ") + "]"
		}
		return toolSummary{summary: s, args: args}

	case "write":
		return toolSummary{summary: (str("filePath"))}

	case "edit":
		return toolSummary{summary: (str("filePath"))}

	case "glob":
		pat := str("pattern")
		args := ""
		if p := str("path"); p != "" {
			args = "[path=" + p + "]"
		}
		return toolSummary{summary: pat, args: args}

	case "grep":
		pat := str("pattern")
		var argParts []string
		if p := str("path"); p != "" {
			argParts = append(argParts, "path="+p)
		}
		if inc := str("include"); inc != "" {
			argParts = append(argParts, inc)
		}
		args := ""
		if len(argParts) > 0 {
			args = "[" + strings.Join(argParts, ", ") + "]"
		}
		return toolSummary{summary: pat, args: args}

	case "tree":
		p := str("path")
		summary := filepath.Base(p)
		if summary == "." || summary == "" {
			summary = "tree"
		}
		args := ""
		var argParts []string
		if depth := intField("depth"); depth > 0 && depth != 4 {
			argParts = append(argParts, fmt.Sprintf("depth=%d", depth))
		}
		if inc := str("include"); inc != "" {
			argParts = append(argParts, inc)
		}
		if len(argParts) > 0 {
			args = "[" + strings.Join(argParts, ", ") + "]"
		}
		return toolSummary{summary: summary, args: args}

	case "patch":
		fp := str("filePath")
		summary := (fp)
		if summary == "" {
			summary = "patch"
		}
		args := ""
		if edits, ok := fields["edits"].([]any); ok {
			args = fmt.Sprintf("[%d edits]", len(edits))
		}
		return toolSummary{summary: summary, args: args}

	case "lsp":
		// Batch operations: show count.
		if ops, ok := fields["operations"].([]any); ok && len(ops) > 0 {
			return toolSummary{summary: fmt.Sprintf("%d operations", len(ops))}
		}
		action := str("action")
		if action == "" {
			action = "lsp"
		}
		args := ""
		fp := str("filePath")
		if fp != "" {
			base := (fp)
			if line := intField("line"); line > 0 {
				args = "[" + base + ":" + strconv.Itoa(line) + "]"
			} else {
				args = "[" + base + "]"
			}
		}
		return toolSummary{summary: action, args: args}

	case "git":
		action := str("action")
		if action == "" {
			action = "git"
		}
		args := ""
		if a := str("args"); a != "" {
			args = "[" + a + "]"
		}
		return toolSummary{summary: action, args: args}

	case "web_fetch":
		urlStr := str("url")
		summary := urlStr
		if urlStr != "" {
			if u, err := url.Parse(urlStr); err == nil {
				summary = u.Host + u.Path
			}
		}
		if summary == "" {
			summary = "web_fetch"
		}
		return toolSummary{summary: truncate(summary, 80)}

	case "task":
		action := str("action")
		switch action {
		case "create":
			summary := "create"
			if title := str("title"); title != "" {
				summary += ": " + truncate(title, 50)
			}
			return toolSummary{summary: summary}
		case "update":
			return toolSummary{summary: "update"}
		case "delete":
			return toolSummary{summary: "delete"}
		case "list":
			return toolSummary{summary: "list"}
		default:
			return toolSummary{}
		}

	case "test":
		cmd := str("command")
		path := str("path")
		summary := cmd
		if summary == "" {
			summary = path
		}
		if summary == "" {
			summary = "test"
		}
		args := ""
		if filter := str("filter"); filter != "" {
			args = "[" + filter + "]"
		}
		return toolSummary{summary: truncate(summary, 60), args: args}

	case "open":
		fp := str("filePath")
		summary := (fp)
		if summary == "" {
			summary = "open"
		}
		args := ""
		if line := intField("line"); line > 0 {
			args = "[Line=" + strconv.Itoa(line) + "]"
		}
		return toolSummary{summary: summary, args: args}

	case "code_action":
		fp := str("filePath")
		summary := fp
		if summary == "" {
			summary = "code_action"
		}
		var argParts []string
		if title := str("title"); title != "" {
			argParts = append(argParts, truncate(title, 40))
		} else if kind := str("kind"); kind != "" {
			argParts = append(argParts, kind)
		}
		if line := intField("startLine"); line > 0 {
			if endLine := intField("endLine"); endLine > 0 && endLine != line {
				argParts = append(argParts, fmt.Sprintf("L%d-%d", line, endLine))
			} else {
				argParts = append(argParts, fmt.Sprintf("L%d", line))
			}
		}
		args := ""
		if len(argParts) > 0 {
			args = "[" + strings.Join(argParts, ", ") + "]"
		}
		return toolSummary{summary: summary, args: args}

	case "rename_symbol":
		fp := str("filePath")
		summary := fp
		if summary == "" {
			summary = "rename_symbol"
		}
		var argParts []string
		if newName := str("newName"); newName != "" {
			argParts = append(argParts, "→ "+newName)
		}
		if line := intField("line"); line > 0 {
			argParts = append(argParts, fmt.Sprintf("L%d", line))
		}
		args := ""
		if len(argParts) > 0 {
			args = "[" + strings.Join(argParts, ", ") + "]"
		}
		return toolSummary{summary: summary, args: args}

	case "skill":
		name := str("name")
		if name == "" {
			name = "skill"
		}
		return toolSummary{summary: name}

	case "subagent":
		agentID := str("agent_id")
		for _, key := range []string{"agent", "tool", "name"} {
			if agentID != "" {
				break
			}
			agentID = str(key)
		}
		task := str("task")
		summary := agentID
		args := ""
		if task != "" {
			task = strings.ReplaceAll(task, "\n", " ")
			t := truncate(task, 60)
			args = `"` + t + `"`
		}
		return toolSummary{summary: summary, args: args}

	case "task_output":
		return toolSummary{}

	case "search":
		query := str("query")
		if query == "" {
			query = "search"
		}
		args := ""
		var argParts []string
		if p := str("path"); p != "" {
			argParts = append(argParts, "path="+p)
		}
		if inc := str("include"); inc != "" {
			argParts = append(argParts, inc)
		}
		if len(argParts) > 0 {
			args = "[" + strings.Join(argParts, ", ") + "]"
		}
		return toolSummary{summary: truncate(query, 60), args: args}

	case "read_files":
		if pattern := str("pattern"); pattern != "" {
			return toolSummary{summary: truncate(pattern, 60)}
		}
		if files, ok := fields["files"].([]any); ok && len(files) > 0 {
			first, _ := files[0].(string)
			if len(files) == 1 {
				return toolSummary{summary: truncate(first, 60)}
			}
			return toolSummary{summary: fmt.Sprintf("%d files", len(files))}
		}
		return toolSummary{summary: "read_files"}

	default:
		return toolSummary{}
	}
}

// taskHintText extracts a concise hint from the task tool output.
// Output format: "Created/Updated task <id>: <title> [<status>]"
func taskHintText(msg Message) string {
	out := msg.ToolOutput
	if out == "" {
		return "done"
	}
	// List output starts with status icons like [✓], [>], [ ], [!].
	if strings.HasPrefix(out, "[") {
		n := 0
		for line := range strings.SplitSeq(out, "\n") {
			if strings.HasPrefix(line, "[") {
				n++
			}
		}
		return fmt.Sprintf("%d tasks", n)
	}
	// Create/update/delete output: "Created task 1: Title [status]"
	if _, title, ok := strings.Cut(out, ": "); ok {
		if bracket := strings.LastIndex(title, " ["); bracket >= 0 {
			title = title[:bracket]
		}
		return truncate(title, 50)
	}
	return "done"
}

// extractTaskStatusField extracts a field value from the <task_status> block
// emitted by the task_output tool. Format: "  field: value\n".
func extractTaskStatusField(output, field string) string {
	prefix := "  " + field + ": "
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(line[len(prefix):])
		}
	}
	return ""
}
