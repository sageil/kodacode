package tui

import (
	"hash/fnv"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type taskDetailDialog struct {
	id          string
	taskID      string
	frameWidth  int
	frameHeight int
	theme       *theme.Theme
	title       string
	subtitle    string
	body        Messages
	renderBody  func(width int) string
}

func newTaskDetailDialog(m Model, state events.SessionState, taskID string, task *events.TaskState) *taskDetailDialog {
	body := NewMessagesWithTone(m.theme, scrollableDetailDialogBodyTone)
	body.SetSoftWrap(false)
	dialog := &taskDetailDialog{
		id:          dialogIDTaskDetail,
		taskID:      strings.TrimSpace(taskID),
		frameWidth:  108,
		frameHeight: 32,
		theme:       m.theme,
		body:        body,
	}
	width, height := dialogRenderSize(m, state)
	dialog.SetFrame(width, height)
	dialog.Sync(m, state, taskID, task)
	return dialog
}

func (d *taskDetailDialog) ID() string { return d.id }

func (d *taskDetailDialog) ignoreWheel(msg tea.MouseWheelMsg) bool {
	return shouldDropVerticalWheel(d.body, msg)
}

func (d *taskDetailDialog) wheelState() (int, bool) {
	return d.body.YOffset(), d.body.AtBottom()
}

func (d *taskDetailDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	d.body.ApplyTheme(th)
}

func (d *taskDetailDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	dialogWidth := toolDetailDialogWidth(width)
	bodyWidth := toolDetailDialogBodyWidth(dialogWidth)
	bodyHeight := toolDetailDialogBodyHeight(height)
	prevBodyWidth := max(d.body.Width(), 1)
	d.body.SetSize(bodyWidth, bodyHeight)
	if d.renderBody != nil && bodyWidth != prevBodyWidth {
		d.body.Sync(d.renderBody(bodyWidth), false)
	}
}

func (d *taskDetailDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		switch typed.String() {
		case "q", "esc", "ctrl+c":
			return d, closeDialog(d.id, nil)
		case "up", "k":
			d.body.ScrollUp(1)
			return d, nil
		case "down", "j":
			d.body.ScrollDown(1)
			return d, nil
		case "pgup", "ctrl+u":
			d.body.PageUp()
			return d, nil
		case "pgdown", "ctrl+d":
			d.body.PageDown()
			return d, nil
		case "home", "g":
			d.body.GotoTop()
			return d, nil
		case "end", "G":
			d.body.GotoBottom()
			return d, nil
		}
	case tea.MouseWheelMsg:
		cmd := d.body.Update(typed)
		return d, cmd
	}
	return d, nil
}

func (d *taskDetailDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	summary := strings.TrimSpace(d.title)
	if meta := strings.TrimSpace(d.subtitle); meta != "" {
		if summary != "" {
			summary += " • " + meta
		} else {
			summary = meta
		}
	}
	subtitle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(d.theme, "subtext", "#9da8ca"))).
		Render(summary)
	width := toolDetailDialogWidth(d.frameWidth)
	content := renderScrollableDetailDialogContent(
		d.theme,
		max(width-dialogFrameInset*2, 1),
		subtitle,
		renderToolDetailDialogViewport(d.theme, d.body, toolDetailDialogContentWidth(width)),
		"q close • ↑/↓ scroll • pgup/pgdn page",
	)
	return drawDialogFrameOnSurfaceWithTone(surface, area, d.theme, width, content, nil, scrollableDetailDialogCardTone)
}

func (d *taskDetailDialog) Sync(m Model, state events.SessionState, taskID string, task *events.TaskState) {
	d.taskID = strings.TrimSpace(taskID)
	d.title = taskDetailDialogTitle(state, taskID, task)
	d.subtitle = taskDetailDialogSubtitle(taskID, task)
	d.renderBody = func(width int) string {
		return taskDetailDialogBody(m, state, taskID, task, width)
	}
	wasEmpty := strings.TrimSpace(d.body.raw) == ""
	d.body.Sync(d.renderBody(max(d.body.Width(), 1)), false)
	if wasEmpty {
		d.body.GotoTop()
	}
}

func (d *taskDetailDialog) OverlayCacheKey() uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, d.title)
	writeTranscriptSignatureString(hasher, d.subtitle)
	appendMessagesRenderCacheSignature(hasher, d.body)
	return hasher.Sum64()
}

func taskDetailDialogTitle(state events.SessionState, taskID string, task *events.TaskState) string {
	if label := strings.TrimSpace(taskDetailLabel(state, taskID, task)); label != "" {
		return label
	}
	return "Task"
}

func taskDetailDialogSubtitle(taskID string, task *events.TaskState) string {
	if task == nil {
		return ""
	}
	parts := []string{strings.TrimSpace(task.Status)}
	if kind := strings.TrimSpace(task.Kind); kind != "" {
		parts = append(parts, kind)
	}
	if taskID = strings.TrimSpace(taskID); taskID != "" {
		parts = append(parts, taskID)
	}
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, strings.TrimSpace(part))
		}
	}
	return strings.Join(filtered, " • ")
}

func taskDetailDialogBody(m Model, state events.SessionState, taskID string, task *events.TaskState, width int) string {
	if task == nil {
		return ""
	}
	body := taskDetailDialogMarkdownBody(state, taskID, task)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return strings.Join(renderMarkdownBlockOnSurface(m, body, max(width, 1), ""), "\n")
}

func taskDetailDialogMarkdownBody(state events.SessionState, taskID string, task *events.TaskState) string {
	if task == nil {
		return ""
	}
	lines := make([]string, 0, 20)
	if taskID = strings.TrimSpace(taskID); taskID != "" {
		lines = append(lines, "Task ID: `"+taskID+"`")
	}
	if status := strings.TrimSpace(task.Status); status != "" {
		lines = append(lines, "Status: `"+status+"`")
	}
	if kind := strings.TrimSpace(task.Kind); kind != "" {
		lines = append(lines, "Kind: `"+kind+"`")
	}
	if parent := strings.TrimSpace(task.ParentTaskID); parent != "" {
		lines = append(lines, "Parent: "+taskDetailReferenceLabel(state, parent))
	}
	if children := taskDetailChildCount(state, taskID); children > 0 {
		lines = append(lines, "Children: `"+strconv.Itoa(children)+"`")
	}
	if review := strings.TrimSpace(task.ReviewStatus); review != "" {
		lines = append(lines, "Review: `"+review+"`")
	}

	appendSection := func(heading, body string) {
		body = strings.TrimSpace(body)
		if body == "" {
			return
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, heading, body)
	}

	appendSection("## Notes", task.Notes)
	appendSection("## Progress", task.Progress)
	appendSection("## Block Reason", task.BlockReason)
	appendSection("## Review Summary", task.ReviewSummary)

	if len(lines) == 0 {
		return "_No task details recorded yet._"
	}
	if len(lines) <= 4 {
		lines = append(lines, "", "_No notes, progress, or review details recorded yet._")
	}
	return strings.Join(lines, "\n")
}

func taskDetailLabel(state events.SessionState, taskID string, task *events.TaskState) string {
	label := ""
	if task != nil {
		label = strings.TrimSpace(task.Title)
	}
	if label == "" {
		label = strings.TrimSpace(taskID)
	}
	if label == "" {
		return ""
	}
	return label
}

func taskDetailReferenceLabel(state events.SessionState, taskID string) string {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return ""
	}
	task := state.Tasks[taskID]
	if task != nil && strings.TrimSpace(task.Title) != "" {
		return strings.TrimSpace(task.Title) + " (`" + taskID + "`)"
	}
	return "`" + taskID + "`"
}

func taskDetailChildCount(state events.SessionState, taskID string) int {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return 0
	}
	count := 0
	for _, candidate := range state.TaskOrder {
		task := state.Tasks[candidate]
		if task == nil {
			continue
		}
		if strings.TrimSpace(task.ParentTaskID) == taskID {
			count++
		}
	}
	return count
}
