package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func isGitStatusOutput(output string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if len(t) < 3 || t[2] != ' ' {
			return false
		}
	}
	return true
}

func renderGitStatus(body *strings.Builder, output string, successStyle, errorStyle, warnStyle, dimStyle, nameStyle lipgloss.Style) {
	boldSuccess := successStyle.Bold(true)
	boldWarn := warnStyle.Bold(true)
	boldError := errorStyle.Bold(true)

	for line := range strings.SplitSeq(strings.TrimRight(output, "\n"), "\n") {
		if len(line) < 3 {
			fmt.Fprintf(body, "  %s\n", dimStyle.Render(line))
			continue
		}
		marker := line[:2]
		filePath := line[3:]
		var styledMarker string
		switch {
		case marker == "??":
			styledMarker = boldWarn.Render("?")
		case marker[0] == 'A' || marker[1] == 'A':
			styledMarker = boldSuccess.Render("A")
		case marker[0] == 'M' || marker[1] == 'M':
			styledMarker = boldWarn.Render("M")
		case marker[0] == 'D' || marker[1] == 'D':
			styledMarker = boldError.Render("D")
		case marker[0] == 'R' || marker[1] == 'R':
			styledMarker = boldWarn.Render("R")
		case marker[0] == 'C' || marker[1] == 'C':
			styledMarker = dimStyle.Render("C")
		default:
			styledMarker = dimStyle.Render(strings.TrimSpace(marker))
		}
		dir, name := filepath.Split(filePath)
		fmt.Fprintf(body, "  %s %s%s\n", styledMarker, dimStyle.Render(dir), nameStyle.Render(name))
	}
}

func (m *Messages) renderBulkRead(body *strings.Builder, output string, textWidth int) {
	dimStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241")))
	lineNumStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("239")))
	barStyle := lipgloss.NewStyle().
		Foreground(colorFrom(m.theme, "primary", lipgloss.Color("255"))).
		Background(colorFrom(m.theme, "surface", lipgloss.Color("236"))).
		Bold(true).
		PaddingLeft(1).PaddingRight(1)
	barDimStyle := lipgloss.NewStyle().
		Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241"))).
		Background(colorFrom(m.theme, "surface", lipgloss.Color("236")))
	barAccent := lipgloss.NewStyle().
		Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141"))).
		Background(colorFrom(m.theme, "surface", lipgloss.Color("236")))

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	var currentLang string
	var codeBlock []string
	flushCode := func() {
		if len(codeBlock) == 0 {
			return
		}
		m.highlightWithLineNumbers(body, codeBlock, currentLang, lineNumStyle, dimStyle, textWidth)
		codeBlock = nil
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "── ") {
			flushCode()

			header := strings.TrimPrefix(line, "── ")
			fname := header
			info := ""
			if idx := strings.Index(header, " ("); idx > 0 {
				fname = header[:idx]
				info = strings.TrimSuffix(strings.TrimPrefix(header[idx+2:], ""), ")")
			}

			currentLang = detectLanguage(fname)
			dir, name := filepath.Split(fname)

			left := barAccent.Render("▎") + barDimStyle.Render(dir) + barStyle.Render(name)
			right := ""
			if info != "" {
				right = barDimStyle.Render(info + " ")
			}
			pad := textWidth - lipgloss.Width(left) - lipgloss.Width(right)
			if pad < 0 {
				pad = 0
			}
			bg := lipgloss.NewStyle().Background(colorFrom(m.theme, "surface", lipgloss.Color("236")))
			fmt.Fprintf(body, "  %s%s%s\n", left, bg.Render(strings.Repeat(" ", pad)), right)
			continue
		}

		codeBlock = append(codeBlock, line)
	}
	flushCode()
}

func (m *Messages) renderTree(body *strings.Builder, output string, _ int) {
	dirStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141"))).Bold(true)
	fileStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "primary", lipgloss.Color("255")))
	connectorStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("239")))

	for line := range strings.SplitSeq(strings.TrimRight(output, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			body.WriteByte('\n')
			continue
		}
		var prefix, entry string
		if i := strings.LastIndex(line, "── "); i >= 0 {
			prefix = line[:i+len("── ")]
			entry = line[i+len("── "):]
		} else {
			entry = strings.TrimSpace(line)
		}

		styledPrefix := connectorStyle.Render(prefix)
		if strings.HasSuffix(entry, "/") {
			fmt.Fprintf(body, "  %s%s\n", styledPrefix, dirStyle.Render(entry))
		} else {
			fmt.Fprintf(body, "  %s%s\n", styledPrefix, fileStyle.Render(entry))
		}
	}
}

func renderGlobResults(body *strings.Builder, output string, dimStyle, nameStyle lipgloss.Style) {
	const maxVisible = 15
	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	// Find the longest common directory prefix.
	prefix := ""
	if len(lines) > 0 {
		prefix = lines[0]
		if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
			prefix = prefix[:idx+1]
		}
		for _, l := range lines[1:] {
			for !strings.HasPrefix(l, prefix) {
				trimmed := strings.TrimSuffix(prefix, "/")
				if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
					prefix = trimmed[:idx+1]
				} else {
					prefix = ""
					break
				}
			}
			if prefix == "" {
				break
			}
		}
	}

	show := lines
	truncated := false
	if len(show) > maxVisible {
		show = show[:maxVisible]
		truncated = true
	}
	for _, l := range show {
		rel := strings.TrimPrefix(l, prefix)
		if idx := strings.LastIndex(rel, "/"); idx >= 0 {
			dir := rel[:idx+1]
			name := rel[idx+1:]
			fmt.Fprintf(body, "  %s%s\n", dimStyle.Render(dir), nameStyle.Render(name))
		} else {
			fmt.Fprintf(body, "  %s\n", nameStyle.Render(rel))
		}
	}
	if truncated {
		fmt.Fprintf(body, "  %s\n", dimStyle.Render(fmt.Sprintf("… and %d more files", len(lines)-maxVisible)))
	}
}

func (m *Messages) renderGrepResults(body *strings.Builder, output string, textWidth int) {
	lineNumStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141")))
	dimStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241")))
	barStyle := lipgloss.NewStyle().
		Foreground(colorFrom(m.theme, "primary", lipgloss.Color("255"))).
		Background(colorFrom(m.theme, "surface", lipgloss.Color("236"))).
		Bold(true).
		PaddingLeft(1).PaddingRight(1)
	barDimStyle := lipgloss.NewStyle().
		Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241"))).
		Background(colorFrom(m.theme, "surface", lipgloss.Color("236")))
	barAccent := lipgloss.NewStyle().
		Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141"))).
		Background(colorFrom(m.theme, "surface", lipgloss.Color("236")))
	bg := lipgloss.NewStyle().Background(colorFrom(m.theme, "surface", lipgloss.Color("236")))

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "(Results truncated") {
			fmt.Fprintf(body, "  %s\n", dimStyle.Render(trimmed))
			continue
		}

		if !strings.HasPrefix(line, "  ") && strings.HasSuffix(trimmed, ":") {
			fname := strings.TrimSuffix(trimmed, ":")
			dir, name := splitPathName(fname)
			left := barAccent.Render("▎") + barDimStyle.Render(dir) + barStyle.Render(name)
			pad := textWidth - lipgloss.Width(left)
			if pad < 0 {
				pad = 0
			}
			fmt.Fprintf(body, "  %s%s\n", left, bg.Render(strings.Repeat(" ", pad)))
			continue
		}

		if strings.HasPrefix(line, "  ") && strings.HasPrefix(trimmed, "Line ") {
			rest := strings.TrimPrefix(trimmed, "Line ")
			if colonIdx := strings.Index(rest, ": "); colonIdx > 0 {
				num := rest[:colonIdx]
				text := rest[colonIdx+2:]
				text = ansi.Truncate(text, textWidth-10, "…")
				fmt.Fprintf(body, "    %s %s\n", lineNumStyle.Render(num+":"), dimStyle.Render(text))
			} else {
				fmt.Fprintf(body, "    %s\n", dimStyle.Render(ansi.Truncate(trimmed, textWidth-4, "…")))
			}
			continue
		}

		fmt.Fprintf(body, "  %s\n", dimStyle.Render(ansi.Truncate(trimmed, textWidth-2, "…")))
	}
}

func (m *Messages) renderSearchResults(body *strings.Builder, output string, textWidth int) {
	dimStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "subtext", lipgloss.Color("241")))
	accentStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141")))
	nameStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "primary", lipgloss.Color("255"))).Bold(true)
	lineNumStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141")))
	kindStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141")))
	pathStyle := dimStyle

	lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
	section := ""

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "Found ") && strings.Contains(line, " results for ") {
			continue
		}

		trimmed := strings.TrimSpace(line)
		switch {
		case strings.Contains(trimmed, "Symbol Matches"):
			section = "symbols"
			fmt.Fprintf(body, "  %s\n", accentStyle.Render("◆ Symbols"))
			continue
		case strings.Contains(trimmed, "File Matches"):
			section = "files"
			fmt.Fprintf(body, "\n  %s\n", accentStyle.Render("◇ Files"))
			continue
		case strings.Contains(trimmed, "Content Matches"):
			section = "content"
			fmt.Fprintf(body, "\n  %s\n", accentStyle.Render("▪ Content"))
			continue
		}

		if trimmed == "" {
			continue
		}

		switch section {
		case "symbols":
			if strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t") {
				text := strings.TrimSpace(line)
				fmt.Fprintf(body, "    %s\n", dimStyle.Render(text))
				continue
			}
			parts := splitSymbolLine(line)
			if len(parts) >= 3 {
				fmt.Fprintf(body, "  %s  %s  %s\n",
					nameStyle.Render(parts[2]),
					kindStyle.Render(parts[1]),
					pathStyle.Render(parts[0]))
			} else {
				fmt.Fprintf(body, "  %s\n", dimStyle.Render(line))
			}

		case "files":
			dir, file := splitPathName(trimmed)
			if dir != "" {
				fmt.Fprintf(body, "  %s%s\n", dimStyle.Render(dir), nameStyle.Render(file))
			} else {
				fmt.Fprintf(body, "  %s\n", nameStyle.Render(file))
			}

		case "content":
			if strings.HasPrefix(line, "  ") {
				rest := strings.TrimLeft(line, " ")
				if colonIdx := strings.Index(rest, ": "); colonIdx > 0 {
					num := rest[:colonIdx]
					text := rest[colonIdx+2:]
					text = ansi.Truncate(text, textWidth-10, "…")
					fmt.Fprintf(body, "    %s %s\n",
						lineNumStyle.Render(num+":"),
						dimStyle.Render(text))
				} else {
					fmt.Fprintf(body, "    %s\n", dimStyle.Render(rest))
				}
			} else {
				dir, file := splitPathName(trimmed)
				if dir != "" {
					fmt.Fprintf(body, "  %s%s\n", dimStyle.Render(dir), nameStyle.Render(file))
				} else {
					fmt.Fprintf(body, "  %s\n", nameStyle.Render(file))
				}
			}

		default:
			if strings.HasPrefix(trimmed, "(Results truncated") {
				fmt.Fprintf(body, "  %s\n", dimStyle.Render(trimmed))
			}
		}
	}
}

func (m *Messages) renderMCPToolBody(body *strings.Builder, msg Message, dimStyle lipgloss.Style, textWidth int) {
	var fields map[string]any
	if json.Unmarshal([]byte(msg.ToolInput), &fields) != nil {
		return
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s, ok := fields[k].(string)
		if !ok || s == "" {
			continue
		}
		lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
		for _, line := range lines {
			line = ansi.Truncate(line, textWidth, "…")
			fmt.Fprintf(body, "  %s\n", dimStyle.Render(line))
		}
	}
}

func renderTaskList(body *strings.Builder, output string, accentStyle, dimStyle lipgloss.Style) {
	for line := range strings.SplitSeq(strings.TrimRight(output, "\n"), "\n") {
		if !strings.HasPrefix(line, "[") {
			continue
		}
		if idx := strings.IndexByte(line, ']'); idx >= 0 {
			icon := line[:idx+1]
			rest := strings.TrimSpace(line[idx+1:])
			fmt.Fprintf(body, "  %s %s\n", accentStyle.Render(icon), dimStyle.Render(rest))
		}
	}
}

func (m *Messages) renderSubagentBody(
	body *strings.Builder, msg Message, _ int,
	nameStyle, dimStyle, accentStyle, errorStyle, _, _ lipgloss.Style,
	_, innerWidth int,
) {
	body.WriteString("\n")

	for _, act := range msg.SubagentActivities {
		var icon string
		switch {
		case !act.Done:
			frame := (pulseTick % 10) * 3
			icon = accentStyle.Render(spinnerFrames[frame : frame+3])
		case act.Error:
			icon = errorStyle.Render("⊘")
		default:
			icon = accentStyle.Render("⦿")
		}

		expandHint := ""
		if act.Done && act.Output != "" {
			if act.Expanded {
				expandHint = " " + dimStyle.Render("▾")
			} else {
				expandHint = " " + dimStyle.Render("▸")
			}
		}

		line := "    " + icon + " " + nameStyle.Render(toolDisplayName(act.Tool))
		if act.Summary != "" {
			line += "  " + dimStyle.Render(act.Summary)
		}
		if act.Args != "" {
			line += "  " + dimStyle.Render(act.Args)
		}
		if act.Done && act.Elapsed > 0 {
			line += "  " + dimStyle.Render(formatElapsed(act.Elapsed))
		} else if !act.Done {
			line += "  " + dimStyle.Render("running…")
		}
		line += expandHint
		if lipgloss.Width(line) > innerWidth {
			line = ansi.Truncate(line, innerWidth, "…")
		}
		body.WriteString(line + "\n")

		if act.Expanded && act.Output != "" {
			body.WriteByte('\n')
			m.renderSubagentActivityOutput(body, act, innerWidth-4)
			body.WriteByte('\n')
		}
	}

	if msg.ToolOutput == "" {
		return
	}

	body.WriteString("\n")
	if msg.ToolDone {
		rendered := m.cachedMarkdownPreserveSoftBreaks(msg.ToolOutput)
		for line := range strings.SplitSeq(strings.TrimRight(rendered, "\n"), "\n") {
			fmt.Fprintf(body, "  %s\n", line)
		}
	} else {
		lines := strings.Split(strings.TrimRight(msg.ToolOutput, "\n"), "\n")
		if len(lines) > 8 {
			lines = lines[len(lines)-8:]
		}
		for _, line := range lines {
			fmt.Fprintf(body, "  %s\n", dimStyle.Render(line))
		}
	}
}

func (m *Messages) renderSubagentActivityOutput(body *strings.Builder, act SubagentActivity, textWidth int) {
	s := m.getStyles()
	msg := Message{
		ToolName:   act.Tool,
		ToolInput:  act.Input,
		ToolOutput: act.Output,
		ToolDone:   true,
	}
	var inner strings.Builder
	m.renderToolPanelContent(&inner, msg, -1, s.dim, s.err, s.success, s.warn, textWidth-4)
	for line := range strings.SplitSeq(strings.TrimRight(inner.String(), "\n"), "\n") {
		fmt.Fprintf(body, "      %s\n", line)
	}
}
