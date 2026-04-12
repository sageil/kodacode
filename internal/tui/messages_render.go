package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/v1/internal/logging"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

// toolStyles caches lipgloss styles derived from the theme. Rebuilt on theme change.
type toolStyles struct {
	label     lipgloss.Style
	subtext   lipgloss.Style
	dim       lipgloss.Style
	success   lipgloss.Style
	err       lipgloss.Style
	warn      lipgloss.Style
	secondary lipgloss.Style
}

func newToolStyles(t *theme.Theme) *toolStyles {
	subtext := lipgloss.NewStyle().Foreground(colorFrom(t, "subtext", lipgloss.Color("241")))
	return &toolStyles{
		label:     lipgloss.NewStyle().Bold(true).Foreground(colorFrom(t, "primary", lipgloss.Color("62"))),
		subtext:   subtext,
		dim:       subtext.Faint(true),
		success:   lipgloss.NewStyle().Foreground(colorFrom(t, "success", lipgloss.Color("2"))),
		err:       lipgloss.NewStyle().Foreground(colorFrom(t, "error", lipgloss.Color("1"))),
		warn:      lipgloss.NewStyle().Foreground(colorFrom(t, "warning", lipgloss.Color("3"))),
		secondary: lipgloss.NewStyle().Foreground(colorFrom(t, "secondary", lipgloss.Color("4"))),
	}
}

func (m *Messages) getStyles() *toolStyles {
	if m.styles == nil {
		m.styles = newToolStyles(m.theme)
	}
	return m.styles
}

// renderMessageAt formats a single message for display.
// i is the index of msg in m.messages, used to determine focus state.
func (m *Messages) renderMessageAt(i int, msg Message) string {
	boxWidth := max(m.vp.Width(), 1)

	var sb strings.Builder
	switch msg.Role {
	case "user":
		accentColor := colorFrom(m.theme, "primary", lipgloss.Color("62"))
		innerWidth := max(boxWidth-2, 1)
		content := ansi.Wrap(msg.Content, innerWidth, "")

		circle := lipgloss.NewStyle().Foreground(accentColor).Render("●")
		lines := strings.Split(content, "\n")
		for j, line := range lines {
			if j == 0 {
				lines[j] = circle + " " + line
			} else {
				lines[j] = "  " + line
			}
		}
		if !msg.Timestamp.IsZero() {
			ts := ansiDim + msg.Timestamp.Format("3:04 PM") + ansiReset
			tsWidth := len(msg.Timestamp.Format("3:04 PM"))
			lastLine := lines[len(lines)-1]
			lastLineW := ansi.StringWidth(lastLine)
			gap := max(boxWidth-lastLineW-tsWidth, 2)
			lines[len(lines)-1] = lastLine + strings.Repeat(" ", gap) + ts
		}
		sb.WriteString(strings.Join(lines, "\n") + "\n")

	case "assistant":
		if msg.Reasoning != "" {
			logging.Debugf("[8-render] renderMessageAt[%d]: reasoning=%d chars, content=%d chars, reasoningDone=%v collapsed=%v", i, len(msg.Reasoning), len(msg.Content), msg.ReasoningDone, msg.ReasoningCollapsed)
			sb.WriteString(m.renderReasoning(msg.Reasoning, boxWidth, msg))
		}
		if strings.TrimSpace(msg.Content) == "" {
			break
		}
		content := msg.Content
		content = m.cachedMarkdown(content)
		wrapWidth := max(boxWidth-2, 1)
		content = ansi.Wrap(content, wrapWidth, "")

		assistantCircle := lipgloss.NewStyle().
			Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("4"))).
			Render("●")

		if msg.Collapsed && !msg.Streaming {
			allLines := strings.Split(strings.TrimRight(content, "\n"), "\n")
			firstLine := ""
			if len(allLines) > 0 {
				firstLine = allLines[0]
			}
			collapsedContent := firstLine
			if len(allLines) > 1 {
				collapsedContent += ansiDim + fmt.Sprintf(" [+%d lines]", len(allLines)-1) + ansiReset
			}
			sb.WriteString(assistantCircle + " " + collapsedContent + "\n")
		} else {
			lines := strings.Split(strings.TrimRight(content, "\n"), "\n")
			for j, line := range lines {
				if j == 0 {
					sb.WriteString(assistantCircle + " " + line + "\n")
				} else {
					sb.WriteString("  " + line + "\n")
				}
			}
		}
		if !msg.Timestamp.IsZero() && !msg.Streaming {
			ts := msg.Timestamp.Format("3:04 PM")
			pad := max(boxWidth-len(ts), 0)
			sb.WriteString(ansiDim + strings.Repeat(" ", pad) + ts + ansiReset + "\n")
		}

	case "system":
		s := m.getStyles()
		accentStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("4")))
		parts := strings.SplitN(msg.Content, "\n", 2)
		title := strings.TrimSpace(parts[0])
		body := ""
		if len(parts) > 1 {
			body = parts[1]
		}
		if title == "" {
			title = "System"
		}
		renderTitleInHeader := lipgloss.Width(" "+title+" ") <= max(boxWidth-4, 1)
		headerTitle := title
		if !renderTitleInHeader {
			headerTitle = "System"
			if strings.TrimSpace(body) != "" {
				body = title + "\n" + body
			} else {
				body = title
			}
		}
		headerText := " " + headerTitle + " "
		headerW := lipgloss.Width(headerText)
		sideW := max((boxWidth-headerW)/2, 0)
		leftLine := s.dim.Render(strings.Repeat("─", sideW))
		rightLine := s.dim.Render(strings.Repeat("─", max(boxWidth-sideW-headerW, 0)))
		header := leftLine + accentStyle.Render(headerText) + rightLine
		sb.WriteString(header + "\n")
		if strings.TrimSpace(body) != "" {
			body = m.cachedMarkdown(body)
			wrapWidth := max(boxWidth-2, 1)
			body = ansi.Wrap(body, wrapWidth, "")
			for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
				sb.WriteString("  " + line + "\n")
			}
		}

	case "error":
		s := m.getStyles()
		errStyle := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "error", lipgloss.Color("196")))
		header := errStyle.Render("  ⊘ Error")
		sb.WriteString(header + "\n")
		body := ansi.Wrap(msg.Content, boxWidth-4, "")
		for line := range strings.SplitSeq(strings.TrimRight(body, "\n"), "\n") {
			sb.WriteString("  " + s.dim.Render(line) + "\n")
		}

	case "tool_call":
		if msg.ToolName == "question" {
			m.renderQuestionToolCall(&sb, msg, boxWidth)
			break
		}
		if msg.ToolName == "task" {
			return ""
		}
		if shouldAutoHide(msg) {
			return ""
		}
		rendered, reviewMeta := m.renderToolCall(msg, i == m.focusedTool, i)
		if reviewMeta != nil {
			m.pendingMetas = append(m.pendingMetas, *reviewMeta)
			m.diffReviewMetas[i] = reviewMeta
		}
		sb.WriteString(rendered)

	default:
		sb.WriteString("  " + msg.Content + "\n")
	}
	result := sb.String()
	if m.searchActive && m.searchQuery != "" {
		result = highlightMatches(result, m.searchQuery)
	}
	return result
}

// highlightMatches performs case-insensitive highlighting of query matches
// in content by wrapping them with reverse-video ANSI escapes.
// It strips ANSI escapes before searching to avoid corrupting escape sequences.
func highlightMatches(content, query string) string {
	if query == "" {
		return content
	}
	plain := ansi.Strip(content)
	lower := strings.ToLower(plain)
	lowerQ := strings.ToLower(query)
	if !strings.Contains(lower, lowerQ) {
		return content
	}
	qLen := len(lowerQ)
	var sb strings.Builder
	sb.Grow(len(plain) + len(plain)/4)
	pos := 0
	for {
		idx := strings.Index(lower[pos:], lowerQ)
		if idx < 0 {
			sb.WriteString(plain[pos:])
			break
		}
		sb.WriteString(plain[pos : pos+idx])
		sb.WriteString(ansiReverse)
		sb.WriteString(plain[pos+idx : pos+idx+qLen])
		sb.WriteString(ansiReset)
		pos += idx + qLen
	}
	return sb.String()
}

// dashWaveFrames generates the dash wave animation. Each dash fades
// independently based on pulseTick, producing a slow wave effect.
func dashWaveFrames(tick int64) string {
	var out [4]byte
	for i := range 4 {
		phase := (tick + int64(i)*2) % 16
		if phase < 4 || phase >= 12 {
			out[i] = '-'
		} else {
			out[i] = ' '
		}
	}
	return string(out[:])
}

// renderQuestionToolCall renders a question tool_call as a question card,
// parsing question and answer from the tool's output ("question\n> answer").
func (m *Messages) renderQuestionToolCall(sb *strings.Builder, msg Message, boxWidth int) {
	s := m.getStyles()
	accent := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "secondary", lipgloss.Color("141")))
	answered := lipgloss.NewStyle().Foreground(colorFrom(m.theme, "success", lipgloss.Color("71")))
	textWidth := max(boxWidth-8, 20)

	question, answer := parseQuestionOutput(msg.ToolOutput)
	if question == "" {
		// Still running or no output yet — show spinner.
		if !msg.ToolDone {
			frame := (pulseTick % 10) * 3
			icon := accent.Render(spinnerFrames[frame : frame+3])
			sb.WriteString("  " + accent.Render("│") + " " + icon + " " + accent.Render("asking…") + "\n")
		}
		return
	}

	bar := s.dim.Render("│")
	if msg.Collapsed {
		summary := question
		if len(summary) > textWidth-20 {
			summary = summary[:textWidth-20] + "…"
		}
		line := "  " + bar + " " + accent.Render("▸ Question") + "  " + s.dim.Render(summary)
		if answer != "" {
			line += "  " + answered.Render("answered")
		}
		sb.WriteString(line + "\n")
		return
	}

	sb.WriteString("  " + bar + " " + accent.Render("QUESTION") + "\n")
	for _, wl := range wrapText(question, textWidth) {
		sb.WriteString("  " + bar + " " + wl + "\n")
	}
	if answer != "" {
		sb.WriteString("  " + bar + " " + s.dim.Render("─") + "\n")
		sb.WriteString("  " + bar + " " + answered.Render("ANSWER") + "\n")
		for _, wl := range wrapText(answer, textWidth) {
			sb.WriteString("  " + bar + " " + s.dim.Render(wl) + "\n")
		}
	} else {
		sb.WriteString("  " + bar + " " + s.dim.Render("awaiting response…") + "\n")
	}
}

// parseQuestionOutput splits the question tool's output ("question\n> answer")
// into question and answer strings.
func parseQuestionOutput(output string) (question, answer string) {
	if output == "" {
		return "", ""
	}
	// Format from question tool: "question text\n> answer text"
	if idx := strings.Index(output, "\n> "); idx >= 0 {
		return output[:idx], output[idx+3:]
	}
	return output, ""
}

const readOnlyHideDelay = 5 * time.Second

func shouldAutoHide(msg Message) bool {
	if msg.UserExpanded || !msg.ToolDone || msg.ToolError != "" {
		return false
	}
	if !isReadOnlyToolCall(msg.ToolName, msg.ToolInput) {
		return false
	}
	if msg.ToolEndTime.IsZero() {
		return false
	}
	return time.Since(msg.ToolEndTime) > readOnlyHideDelay
}

func shouldAutoCollapse(msg Message) bool {
	if msg.ToolName == "question" {
		return false
	}
	if isStreamingDiffTool(msg.ToolName) {
		return false
	}
	if msg.ToolName == "subagent" {
		// Don't auto-collapse slash-command subagents — their result is
		// the only output the user sees (no engineer workflow summary step).
		if strings.HasPrefix(msg.ToolCallID, "slash-") {
			return false
		}
		return true
	}
	if msg.ToolError != "" {
		return isReadOnlyToolCall(msg.ToolName, msg.ToolInput)
	}
	return !looksLikeFailure(msg.ToolOutput)
}

func looksLikeFailure(output string) bool {
	lines := strings.Split(output, "\n")
	start := max(len(lines)-10, 0)
	for _, line := range lines[start:] {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lower, "fail") ||
			strings.HasPrefix(lower, "error") ||
			strings.Contains(lower, "exit code") ||
			strings.Contains(lower, "exited with") ||
			strings.Contains(lower, "✗") {
			return true
		}
	}
	return false
}

func (m Messages) renderScrollbar() []string {
	height := m.vp.Height()
	total := m.vp.TotalLineCount()
	offset := m.vp.YOffset()

	if total <= height {
		return nil
	}

	scrollbarColor := colorFrom(m.theme, "scrollbar", lipgloss.Color("241"))
	thumbColor := colorFrom(m.theme, "primary", lipgloss.Color("62"))

	trackStyle := lipgloss.NewStyle().Foreground(scrollbarColor)
	thumbStyle := lipgloss.NewStyle().Foreground(thumbColor)

	thumbHeight := max(1, height*height/total)
	thumbStart := offset * height / total
	if thumbStart+thumbHeight > height {
		thumbStart = height - thumbHeight
	}

	scrollbar := make([]string, height)
	for i := range height {
		if i >= thumbStart && i < thumbStart+thumbHeight {
			scrollbar[i] = thumbStyle.Render("█")
		} else {
			scrollbar[i] = trackStyle.Render("│")
		}
	}

	return scrollbar
}
