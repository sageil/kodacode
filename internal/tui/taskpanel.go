package tui

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/v1/internal/tool"
	"github.com/sageil/kodacode/v1/internal/tui/theme"
)

var titlePrefixRe = regexp.MustCompile(`^(?:\d+\.\s*|[Tt]ask\s*\d+[:.]\s*)`)

// countCriteria counts bullet-point lines in the acceptance criteria section of task notes.
func countCriteria(notes string) int {
	criteria := extractAcceptanceCriteriaItems(notes)
	return len(criteria)
}

func extractAcceptanceCriteriaItems(notes string) []string {
	re := regexp.MustCompile(`(?is)\bacceptance criteria\b\s*:?\s*`)
	loc := re.FindStringIndex(notes)
	if loc == nil {
		return nil
	}
	raw := strings.TrimSpace(notes[loc[1]:])
	if raw == "" {
		return nil
	}
	return parseCriteriaItems(raw)
}

func parseCriteriaItems(raw string) []string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	if strings.Contains(raw, "\n") {
		lines := strings.Split(raw, "\n")
		items := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimSpace(line)
			line = strings.TrimLeft(line, "-* ")
			line = strings.TrimSpace(regexp.MustCompile(`^\d+[.)]\s*`).ReplaceAllString(line, ""))
			if line == "" {
				continue
			}
			items = append(items, line)
		}
		return items
	}
	re := regexp.MustCompile(`(?:^|\s+)\d+[.)]\s+`)
	locs := re.FindAllStringIndex(raw, -1)
	if len(locs) == 0 {
		return []string{raw}
	}
	items := make([]string, 0, len(locs))
	for i, loc := range locs {
		start := loc[1]
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		item := strings.TrimSpace(raw[start:end])
		if item != "" {
			items = append(items, item)
		}
	}
	return items
}

func truncateTask(s string, maxW int) string {
	runes := []rune(s)
	if len(runes) <= maxW {
		return s
	}
	return string(runes[:maxW-1]) + "…"
}

func taskPanelBadge(task *tool.Task) string {
	if task == nil {
		return ""
	}
	if task.Status == "blocked" {
		switch task.BlockReason {
		case tool.TaskBlockReasonReviewCap:
			return "blocked: review cap"
		case tool.TaskBlockReasonExecutionStall:
			return "blocked: execution stalled"
		default:
			return "blocked"
		}
	}
	switch task.ReviewStatus {
	case tool.TaskReviewFail:
		return "review fail"
	case tool.TaskReviewConcern:
		return "review concern"
	case tool.TaskReviewAccepted:
		return "review accepted"
	default:
		return ""
	}
}

func taskPanelDetailLine(task *tool.Task) string {
	if task == nil {
		return ""
	}
	meta := tool.TaskWorkflowStateSummary(task)
	detail := strings.TrimSpace(task.LastReviewSummary)
	switch {
	case detail != "" && tool.ShouldShowReviewSummary(task):
		if meta != "" {
			return meta + " — " + detail
		}
		return detail
	case meta != "":
		return meta
	default:
		return ""
	}
}

// TaskPanel renders a collapsible task progress bar between the header and messages.
type TaskPanel struct {
	tasks     []*tool.Task
	width     int
	expanded  bool
	cancelled bool // true after ESC — suppresses spinners for in-progress tasks
	theme     *theme.Theme
}

func NewTaskPanel() TaskPanel { return TaskPanel{} }

func (p *TaskPanel) ApplyTheme(t *theme.Theme) { p.theme = t }

func (p *TaskPanel) SetSize(w int) { p.width = w }

func (p *TaskPanel) SetTasks(tasks []*tool.Task) {
	p.tasks = tasks
	// Reset cancelled when new tasks arrive (a new turn started).
	if len(tasks) > 0 {
		p.cancelled = false
	}
}

func (p *TaskPanel) Cancel() { p.cancelled = true }

func (p *TaskPanel) Toggle() { p.expanded = !p.expanded }

func (p TaskPanel) HasTasks() bool { return len(p.tasks) > 0 }

// Height returns the number of lines this panel occupies.
// Returns 0 when there are no tasks (panel is hidden).
func (p TaskPanel) Height() int {
	if len(p.tasks) == 0 {
		return 0
	}
	if !p.expanded {
		return 3 // accent border + collapsed line + bottom separator
	}
	height := 3 + len(p.tasks) // accent border + header + tasks + bottom separator
	for _, task := range p.tasks {
		if taskPanelDetailLine(task) != "" {
			height++
		}
	}
	return height
}

func (p TaskPanel) View() string {
	if len(p.tasks) == 0 {
		return ""
	}

	w := p.width
	if w < 1 {
		w = 80
	}

	dimColor := colorFrom(p.theme, "subtext", lipgloss.Color("241"))
	dimStyle := lipgloss.NewStyle().Foreground(dimColor)
	accentColor := colorFrom(p.theme, "secondary", lipgloss.Color("4"))
	accentStyle := lipgloss.NewStyle().Foreground(accentColor).Bold(true)
	greenColor := colorFrom(p.theme, "success", lipgloss.Color("76"))
	greenStyle := lipgloss.NewStyle().Foreground(greenColor)

	// Count by status and find the active task.
	// Priority: in_progress > first pending.
	var completed, inProgress int
	var currentTask, firstPending string
	for _, t := range p.tasks {
		switch t.Status {
		case "completed", "done":
			completed++
		case "in_progress", "in-progress", "running":
			inProgress++
			if currentTask == "" {
				currentTask = t.Title
			}
		default:
			if firstPending == "" {
				firstPending = t.Title
			}
		}
	}
	if currentTask == "" {
		currentTask = firstPending
	}
	currentTask = titlePrefixRe.ReplaceAllString(currentTask, "")
	total := len(p.tasks)

	barWidth := 12
	filled := 0
	if total > 0 {
		filled = completed * barWidth / total
	}
	bar := greenStyle.Render(strings.Repeat("█", filled)) + dimStyle.Render(strings.Repeat("░", barWidth-filled))

	topBorder := lipgloss.NewStyle().Foreground(accentColor).Render(strings.Repeat("▔", w))
	bottomSep := dimStyle.Render(strings.Repeat("─", w))

	var sb strings.Builder
	sb.WriteString(topBorder)

	allDone := completed >= total
	const spinnerFrames = "⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	pulseIcon := func(style lipgloss.Style) string {
		if p.cancelled {
			return dimStyle.Render("○")
		}
		frame := (pulseTick % 10) * 3
		return style.Render(spinnerFrames[frame : frame+3])
	}

	if !p.expanded {
		content := accentStyle.Render("Tasks") + " " + dimStyle.Render(fmt.Sprintf("%d/%d", completed, total)) + "  " + bar
		if currentTask != "" && !allDone {
			var icon string
			if inProgress > 0 {
				icon = pulseIcon(accentStyle)
			} else {
				icon = dimStyle.Render("○") // static for pending
			}
			// Truncate task title in collapsed view to keep it on one line.
			maxTaskW := max(w/2-20, 20)
			content += "  " + icon + " " + truncateTask(currentTask, maxTaskW)
		}
		content += "  " + dimStyle.Render("▸")

		contentW := lipgloss.Width(content)
		pad := max((w-contentW)/2, 0)
		sb.WriteByte('\n')
		sb.WriteString(strings.Repeat(" ", pad) + content)
		sb.WriteByte('\n')
		sb.WriteString(bottomSep)
		return sb.String()
	}

	header := accentStyle.Render("Tasks") + " " + dimStyle.Render(fmt.Sprintf("%d/%d completed", completed, total)) + "  " + bar + "  " + dimStyle.Render("▾")
	headerW := lipgloss.Width(header)
	headerPad := max((w-headerW)/2, 0)
	sb.WriteByte('\n')
	sb.WriteString(strings.Repeat(" ", headerPad) + header)

	for i, t := range p.tasks {
		var icon string
		var titleStyle lipgloss.Style
		num := dimStyle.Render(fmt.Sprintf("%d.", i+1))
		done := false
		switch t.Status {
		case "completed", "done":
			icon = accentStyle.Render("⦿")
			titleStyle = lipgloss.NewStyle().Foreground(dimColor).Strikethrough(true)
			done = true
		case "in_progress", "in-progress", "running":
			icon = pulseIcon(accentStyle)
			titleStyle = lipgloss.NewStyle()
		default:
			icon = dimStyle.Render("○")
			titleStyle = dimStyle
		}
		title := titlePrefixRe.ReplaceAllString(t.Title, "")
		line := num + " " + icon + " " + titleStyle.Render(title)
		if n := countCriteria(t.Notes); n > 0 {
			badge := fmt.Sprintf("%d criteria", n)
			if done {
				badge += " ✓"
			}
			line += " " + dimStyle.Render(badge)
		}
		if badge := taskPanelBadge(t); badge != "" {
			line += " " + dimStyle.Render(badge)
		}
		lineW := lipgloss.Width(line)
		linePad := max((w-lineW)/2, 0)
		sb.WriteByte('\n')
		sb.WriteString(strings.Repeat(" ", linePad) + line)
		if detail := taskPanelDetailLine(t); detail != "" {
			rendered := dimStyle.Render(truncateTask(detail, max(w-12, 24)))
			detailW := lipgloss.Width(rendered)
			detailPad := max((w-detailW)/2, 0)
			sb.WriteByte('\n')
			sb.WriteString(strings.Repeat(" ", detailPad) + rendered)
		}
	}

	sb.WriteByte('\n')
	sb.WriteString(bottomSep)
	return sb.String()
}
