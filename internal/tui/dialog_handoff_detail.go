package tui

import (
	"fmt"
	"hash/fnv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type handoffDetailDialog struct {
	id          string
	sessionID   string
	target      inspectorHandoffTarget
	frameWidth  int
	frameHeight int
	theme       *theme.Theme
	icons       terminalIconProfile
	title       string
	subtitle    string
	body        Messages
	renderBody  func(width int) string
}

func newHandoffDetailDialog(m Model, sessionID string, state events.SessionState, target inspectorHandoffTarget, handoff *events.AgentHandoffState) *handoffDetailDialog {
	body := NewMessagesWithTone(m.theme, scrollableDetailDialogBodyTone)
	body.SetSoftWrap(false)
	dialog := &handoffDetailDialog{
		id:          dialogIDHandoffDetail,
		sessionID:   normalizeToolTargetSessionID(m.sessionID, sessionID),
		target:      target,
		frameWidth:  108,
		frameHeight: 32,
		theme:       m.theme,
		icons:       m.terminalIcons,
		body:        body,
	}
	width, height := dialogRenderSize(m, state)
	dialog.SetFrame(width, height)
	dialog.Sync(m, dialog.sessionID, state, target, handoff)
	return dialog
}

func (d *handoffDetailDialog) ID() string { return d.id }

func (d *handoffDetailDialog) ignoreWheel(msg tea.MouseWheelMsg) bool {
	return shouldDropVerticalWheel(d.body, msg)
}

func (d *handoffDetailDialog) wheelState() (int, bool) {
	return d.body.YOffset(), d.body.AtBottom()
}

func (d *handoffDetailDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	d.body.ApplyTheme(th)
}

func (d *handoffDetailDialog) SetFrame(width, height int) {
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

func (d *handoffDetailDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
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

func (d *handoffDetailDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
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
		renderToolDetailDialogViewport(d.theme, d.icons, d.body, toolDetailDialogContentWidth(width)),
		"q close • ↑/↓ scroll • pgup/pgdn page",
	)
	return drawDialogFrameOnSurfaceWithTone(surface, area, d.theme, width, content, nil, scrollableDetailDialogCardTone)
}

func (d *handoffDetailDialog) Sync(m Model, sessionID string, state events.SessionState, target inspectorHandoffTarget, handoff *events.AgentHandoffState) {
	d.sessionID = normalizeToolTargetSessionID(m.sessionID, sessionID)
	d.target = target
	d.target.SessionID = d.sessionID
	d.title = handoffDetailDialogTitle(d.target, handoff)
	d.subtitle = handoffDetailDialogSubtitle(d.target, handoff)
	d.renderBody = func(width int) string {
		return handoffDetailDialogBody(m, state, d.target, handoff, width)
	}
	wasEmpty := strings.TrimSpace(d.body.raw) == ""
	d.body.Sync(d.renderBody(max(d.body.Width(), 1)), false)
	if wasEmpty {
		d.body.GotoTop()
	}
}

func (d *handoffDetailDialog) OverlayCacheKey() uint64 {
	hasher := fnv.New64a()
	writeTranscriptSignatureString(hasher, d.title)
	writeTranscriptSignatureString(hasher, d.subtitle)
	appendMessagesRenderCacheSignature(hasher, d.body)
	return hasher.Sum64()
}

func handoffDetailDialogTitle(target inspectorHandoffTarget, handoff *events.AgentHandoffState) string {
	if label := delegatedInspectorAgentLabel(handoff, nil); label != "" {
		return label
	}
	if handoffID := strings.TrimSpace(target.HandoffID); handoffID != "" {
		return handoffID
	}
	return "Delegated Work"
}

func handoffDetailDialogSubtitle(target inspectorHandoffTarget, handoff *events.AgentHandoffState) string {
	parts := make([]string, 0, 3)
	if handoff != nil {
		if status := strings.TrimSpace(handoffDisplayStatusLabel(handoff)); status != "" {
			parts = append(parts, status)
		}
	}
	if handoffID := strings.TrimSpace(target.HandoffID); handoffID != "" {
		parts = append(parts, handoffID)
	}
	if sessionID := strings.TrimSpace(target.SessionID); sessionID != "" {
		parts = append(parts, sessionID)
	}
	return strings.Join(parts, " • ")
}

func handoffDetailDialogBody(m Model, state events.SessionState, target inspectorHandoffTarget, handoff *events.AgentHandoffState, width int) string {
	body := handoffDetailDialogMarkdownBody(state, target, handoff)
	if strings.TrimSpace(body) == "" {
		return ""
	}
	return strings.Join(renderMarkdownBlockOnSurface(m, body, max(width, 1), ""), "\n")
}

func handoffDetailDialogMarkdownBody(state events.SessionState, target inspectorHandoffTarget, handoff *events.AgentHandoffState) string {
	if handoff == nil {
		return ""
	}

	lines := make([]string, 0, 32)
	appendMeta := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		lines = append(lines, label+": `"+value+"`")
	}
	appendCodeList := func(label string, values []string) {
		filtered := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" {
				continue
			}
			filtered = append(filtered, "`"+value+"`")
		}
		if len(filtered) == 0 {
			return
		}
		lines = append(lines, label+": "+strings.Join(filtered, ", "))
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

	appendMeta("Handoff ID", target.HandoffID)
	if ordinal := sessionToolTurnOrdinal(state, target.TurnID); ordinal > 0 {
		lines = append(lines, fmt.Sprintf("Parent Turn: `%d of %d`", ordinal, len(state.TurnOrder)))
	}
	appendMeta("Parent Session ID", handoff.ParentSessionID)
	appendMeta("Parent Turn ID", handoff.ParentTurnID)
	appendMeta("Child Session ID", handoff.ChildSessionID)
	appendMeta("Child Turn ID", handoff.ChildTurnID)
	appendMeta("Status", handoffDisplayStatusLabel(handoff))
	appendMeta("Parent Agent", handoff.ParentAgentID)
	appendMeta("Child Agent", handoff.ChildAgentID)
	appendMeta("Model", handoff.Model)
	appendCodeList("Allowed Tools", handoff.AllowedTools)

	appendSection("## Task", handoff.Task)
	appendSection("## Context", handoff.ContextSummary)
	appendSection("## Live Preview", handoffPreviewInspectorText(handoff))
	if handoff.Reused {
		appendSection("## Reused Result", handoff.ReusedContent)
	}
	appendSection("## Child Output", handoff.AssistantText)
	appendSection("## Error", handoff.Error)

	if handoff.Status == events.AgentResultStatusPendingPermission {
		permissionLines := []string{}
		if requestID := strings.TrimSpace(handoff.PermissionRequestID); requestID != "" {
			permissionLines = append(permissionLines, "Request ID: `"+requestID+"`")
		}
		if toolName := strings.TrimSpace(handoff.PermissionToolName); toolName != "" {
			permissionLines = append(permissionLines, "Tool: `"+toolName+"`")
		}
		if label := strings.TrimSpace(permissionKindLabel(handoff.PermissionKind, handoff.PermissionAccess)); label != "" {
			permissionLines = append(permissionLines, "Kind: `"+label+"`")
		}
		switch handoff.PermissionKind {
		case events.PermissionRequestKindExecution:
			appendDetailLine(&permissionLines, "Working Directory", handoff.PermissionDir)
		case events.PermissionRequestKindNetwork:
			appendDetailLine(&permissionLines, "Target", handoff.PermissionPath)
		default:
			appendDetailLine(&permissionLines, "Path", handoff.PermissionPath)
			if access := strings.TrimSpace(permissionAccessLabel(handoff.PermissionAccess)); access != "" {
				permissionLines = append(permissionLines, "Access: `"+access+"`")
			}
		}
		if reason := strings.TrimSpace(handoff.PermissionReason); reason != "" {
			permissionLines = append(permissionLines, "Reason: "+reason)
		}
		if command := strings.TrimSpace(handoff.PermissionCommand); command != "" {
			permissionLines = append(permissionLines, "", "```sh", command, "```")
		}
		appendSection("## Pending Permission", strings.Join(permissionLines, "\n"))
	}

	if handoff.Status == events.AgentResultStatusPendingQuestion {
		questionLines := []string{}
		appendDetailLine(&questionLines, "Request ID", handoff.QuestionRequestID)
		appendDetailLine(&questionLines, "Tool", handoff.QuestionToolName)
		if question := strings.TrimSpace(handoff.QuestionText); question != "" {
			questionLines = append(questionLines, "Question: "+question)
		}
		if len(handoff.QuestionOptions) > 0 {
			options := make([]string, 0, len(handoff.QuestionOptions))
			for _, option := range handoff.QuestionOptions {
				option = strings.TrimSpace(option)
				if option == "" {
					continue
				}
				options = append(options, "`"+option+"`")
			}
			if len(options) > 0 {
				questionLines = append(questionLines, "Options: "+strings.Join(options, ", "))
			}
		}
		questionLines = append(questionLines, "Answer from the active transcript prompt.")
		appendSection("## Pending Input", strings.Join(questionLines, "\n"))
	}

	if len(lines) == 0 {
		return "_No delegated task details recorded yet._"
	}
	return strings.Join(lines, "\n")
}

func appendDetailLine(lines *[]string, label, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	*lines = append(*lines, label+": `"+value+"`")
}
