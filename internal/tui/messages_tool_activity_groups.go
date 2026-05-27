package tui

import (
	"path/filepath"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sageil/kodacode/internal/events"
)

func renderWideToolGroupSummarySection(m Model, state events.SessionState, group wideToolTranscriptGroup, width int) string {
	if title, ok := flattenedUsedToolGroupTitle(state, group); ok {
		return renderWideToolSection(m, title, group.Status, nil, "", width)
	}
	switch group.Kind {
	case wideToolGroupMutation:
		if len(group.Refs) == 0 {
			return ""
		}
		ref := group.Refs[0]
		_, call := sessionToolCall(state, ref)
		if call == nil {
			return ""
		}
		rows := deriveTurnToolOutcomeRows(state, []sessionToolCallRef{ref})
		if len(rows) == 0 {
			return ""
		}
		return renderToolOutcomeSummarySection(m, state, rows[0], call, width)
	case wideToolGroupExplored:
		return renderWideToolGroupSection(m, state, group, toolOutcomeGroupLabel(group.Kind, group.Status), group.Status, nil, width)
	case wideToolGroupRan:
		return renderWideToolGroupSection(m, state, group, toolOutcomeGroupLabel(group.Kind, group.Status), group.Status, nil, width)
	case wideToolGroupBlocked:
		return renderWideToolGroupSection(m, state, group, toolOutcomeGroupLabel(group.Kind, group.Status), group.Status, nil, width)
	case wideToolGroupQuestion:
		return renderWideToolGroupSection(m, state, group, toolOutcomeGroupLabel(group.Kind, group.Status), group.Status, nil, width)
	case wideToolGroupTaskList:
		return renderWideTaskListGroupSection(m, state, group, width)
	default:
		return renderWideToolGroupSection(m, state, group, toolOutcomeGroupLabel(group.Kind, group.Status), group.Status, nil, width)
	}
}

func renderWideToolGroupSection(m Model, state events.SessionState, group wideToolTranscriptGroup, title, status string, meta []string, width int) string {
	lines := make([]string, 0, len(group.Refs)+1)
	lines = append(lines, renderTranscriptWideToolHeader(m, title, status, width))
	for _, ref := range group.Refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		lines = append(lines, renderWideToolGroupItemLine(
			m,
			state.WorkspaceRoot,
			ref,
			call,
			width,
			selectedToolMatchesSession(m, state.SessionID, ref),
		))
	}
	return strings.Join(lines, "\n")
}

func renderWideTaskListGroupSection(m Model, state events.SessionState, group wideToolTranscriptGroup, width int) string {
	lines := make([]string, 0, len(group.Refs)+1)
	lines = append(lines, renderTranscriptWideToolHeader(m, toolOutcomeGroupLabel(group.Kind, group.Status), group.Status, width))
	for _, ref := range group.Refs {
		_, call := sessionToolCall(state, ref)
		if call == nil {
			continue
		}
		tasks, ok := parseTaskToolViewListOutput(call.Output)
		if !ok {
			lines = append(lines, renderWideToolGroupItemLine(
				m,
				state.WorkspaceRoot,
				ref,
				call,
				width,
				selectedToolMatchesSession(m, state.SessionID, ref),
			))
			continue
		}
		if len(tasks) == 0 {
			lines = append(lines, renderTaskListGroupItemLine(m, "No tasks", "", width, selectedToolMatchesSession(m, state.SessionID, ref)))
			continue
		}
		for _, task := range tasks {
			lines = append(lines, renderTaskListGroupItemLine(
				m,
				taskToolListItemLabel(task),
				taskToolListItemStatus(task),
				width,
				selectedToolMatchesSession(m, state.SessionID, ref),
			))
		}
	}
	return strings.Join(lines, "\n")
}

func renderWideToolGroupItemLine(m Model, workspaceRoot string, ref sessionToolCallRef, call *events.ToolCallState, width int, selected bool) string {
	label := groupedToolItemLabel(workspaceRoot, call)
	if strings.TrimSpace(label) == "" {
		label = "Tool"
	}
	if detail := strings.TrimSpace(groupedToolItemResultDetail(call)); detail != "" {
		switch {
		case strings.TrimSpace(call.ToolName) == "read" &&
			strings.TrimSpace(call.Error) == "" && !call.Executing:
			label += " (" + detail + ")"
		default:
			label += " · " + detail
		}
	}
	status := normalizeOutcomeStatus(toolStatus(call))
	prefixText := "↳ "
	prefixColor := colorFor(m.theme, "subtext", "#9da8ca")
	labelColor := colorFor(m.theme, "text", "#ecf0ff")
	switch status {
	case "error":
		prefixText = "× "
		prefixColor = colorFor(m.theme, "error", "#ff9aa6")
		labelColor = colorFor(m.theme, "error", "#ff9aa6")
	case "running", "preparing", "building":
		prefixText = "• "
		prefixColor = colorFor(m.theme, "warning", "#ffd28f")
	}
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(prefixColor)).
		Render(prefixText)
	contentStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(labelColor))
	if selected {
		contentStyle = contentStyle.Bold(true)
		if status != "error" {
			contentStyle = contentStyle.Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff")))
		}
	}
	line := prefix + contentStyle.Render(truncateEnd(singleLineToolText(label), max(width-ansi.StringWidth(prefixText), 1)))
	return line
}

func flattenedUsedToolGroupTitle(state events.SessionState, group wideToolTranscriptGroup) (string, bool) {
	if group.Kind != wideToolGroupUsed || len(group.Refs) != 1 {
		return "", false
	}
	_, call := sessionToolCall(state, group.Refs[0])
	if call == nil {
		return "", false
	}
	switch strings.TrimSpace(call.ToolName) {
	case "web_search":
		label := strings.TrimSpace(groupedToolItemLabel(state.WorkspaceRoot, call))
		if label == "" {
			return "", false
		}
		return label, true
	case "skill":
		input, ok := parseSkillToolViewInput(call.Input)
		if !ok {
			return "", false
		}
		label := "Skill " + input.ID
		if input.Section != "" {
			label += " · " + input.Section
		}
		switch normalizeOutcomeStatus(toolStatus(call)) {
		case "running", "preparing", "building":
			return label + " loading", true
		case "error":
			return label + " failed", true
		default:
			return label + " loaded", true
		}
	default:
		return "", false
	}
}

func renderTaskListGroupItemLine(m Model, label, status string, width int, selected bool) string {
	status = strings.TrimSpace(status)
	statusLabel := ""
	if status != "" {
		statusLabel = strings.ToUpper(status)
	}
	prefix := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "subtext", "#9da8ca"))).
		Render("↳ ")
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorFor(m.theme, "text", "#ecf0ff")))
	if selected {
		labelStyle = labelStyle.
			Foreground(lipgloss.Color(colorFor(m.theme, "primary", "#7cc7ff"))).
			Bold(true)
	}
	leftWidth := max(width-ansi.StringWidth("↳ ")-len(statusLabel)-1, 1)
	left := prefix + labelStyle.Render(truncateEnd(singleLineToolText(label), leftWidth))
	if statusLabel == "" {
		return left
	}
	right := lipgloss.NewStyle().
		Foreground(taskStatusColor(m.theme, status)).
		Render(statusLabel)
	return joinBar(left, right, max(width, 1))
}

func wideToolDetailTitleForSession(state events.SessionState, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	switch strings.TrimSpace(call.ToolName) {
	case "read", "search", "locate", "web_fetch", "refs", "list", "tree", "git_status", "git_diff", "git_show":
		if label := strings.TrimSpace(groupedToolItemLabel(state.WorkspaceRoot, call)); label != "" {
			return label
		}
	}
	title := strings.TrimSpace(toolDisplayNameForSession(state, call))
	if title == "" {
		return "Tool"
	}
	return title
}

func wideToolDetailTitle(workspaceRoot string, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	switch strings.TrimSpace(call.ToolName) {
	case "read", "search", "locate", "web_fetch", "refs", "list", "tree", "git_status", "git_diff", "git_show":
		if label := strings.TrimSpace(groupedToolItemLabel(workspaceRoot, call)); label != "" {
			return label
		}
	}
	title := strings.TrimSpace(toolDisplayNameForWorkspace(workspaceRoot, call))
	if title == "" {
		return "Tool"
	}
	return title
}

func groupedToolItemLabel(workspaceRoot string, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	inProgress := toolCallUsesProgressiveLabel(call)
	if label, ok := groupedToolItemWorkspaceLabel(workspaceRoot, call, inProgress); ok {
		return label
	}
	if label, ok := groupedToolItemMutationLabel(workspaceRoot, call, inProgress); ok {
		return label
	}
	if label, ok := groupedToolItemCommandLabel(workspaceRoot, call); ok {
		return label
	}
	if label, ok := groupedToolItemWorkflowLabel(workspaceRoot, call); ok {
		return label
	}
	label := strings.TrimSpace(toolDisplayNameForWorkspace(workspaceRoot, call))
	if label == "" {
		return ""
	}
	return "Use " + label
}

func groupedToolItemWorkspaceLabel(workspaceRoot string, call *events.ToolCallState, inProgress bool) (string, bool) {
	switch strings.TrimSpace(call.ToolName) {
	case "bash":
		if outcomeCategoryForTool(call) == toolOutcomeMutation {
			return "", false
		}
		if command := strings.TrimSpace(commandToolDisplayName(workspaceRoot, call)); command != "" && command != "bash" {
			return "Shell: " + command, true
		}
		return "Shell", true
	case "read":
		input, ok := parseReadToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Reading file", true
			}
			return "Read file", true
		}
		verb := "Read "
		if inProgress {
			verb = "Reading "
		}
		if len(input.Paths) == 1 {
			return verb + displayToolBaseName(workspaceRoot, input.Paths[0]), true
		}
		return verb + readGroupedToolPathLabel(workspaceRoot, input.Paths), true
	case "search":
		input, ok := parseSearchToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Searching workspace", true
			}
			return "Search workspace", true
		}
		path := displayToolPath(workspaceRoot, input.Path)
		query := searchToolQueryLabel(input.Query, input.Regex)
		prefix := "Search"
		if inProgress {
			prefix = "Searching"
		}
		switch {
		case path != "." && path != "" && query != "":
			return prefix + " " + path + " for " + query, true
		case query != "":
			return prefix + " for " + query, true
		case path != "." && path != "":
			return prefix + " " + path, true
		default:
			return prefix + " workspace", true
		}
	case "locate":
		input, ok := parseLocateToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Locating path", true
			}
			return "Locate path", true
		}
		path := displayToolPath(workspaceRoot, input.Path)
		query := strings.TrimSpace(input.Query)
		prefix := "Locate"
		if inProgress {
			prefix = "Locating"
		}
		switch {
		case path != "." && path != "" && query != "":
			return prefix + " " + query + " under " + path, true
		case query != "":
			return prefix + " " + query, true
		case path != "." && path != "":
			return prefix + " " + path, true
		default:
			return prefix + " workspace", true
		}
	case "refs":
		input, ok := parseRefsToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Finding refs", true
			}
			return "Refs", true
		}
		path := displayToolPath(workspaceRoot, input.Path)
		if path == "." {
			path = ""
		}
		prefix := "Refs"
		if inProgress {
			prefix = "Finding refs"
		}
		switch {
		case path != "" && strings.TrimSpace(input.Mode) != "":
			return prefix + " " + path + " · " + input.Mode, true
		case path != "":
			return prefix + " " + path, true
		case strings.TrimSpace(input.Mode) != "":
			return prefix + " · " + input.Mode, true
		default:
			return prefix, true
		}
	case "web_fetch":
		input, ok := parseWebFetchToolViewInput(call.Input)
		if !ok || strings.TrimSpace(input.URL) == "" {
			if inProgress {
				return "Fetching URL", true
			}
			return "Fetch URL", true
		}
		prefix := "Fetch "
		if inProgress {
			prefix = "Fetching "
		}
		return prefix + strings.TrimSpace(input.URL), true
	case "search_skills":
		input, ok := parseSearchSkillsToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Searching skills", true
			}
			return "Search skills", true
		}
		prefix := "Search skills for "
		if inProgress {
			prefix = "Searching skills for "
		}
		return prefix + input.Query, true
	case "skill":
		input, ok := parseSkillToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Loading skill", true
			}
			return "Load skill", true
		}
		prefix := "Load skill "
		if inProgress {
			prefix = "Loading skill "
		}
		if input.Section != "" {
			return prefix + input.ID + " · " + input.Section, true
		}
		return prefix + input.ID, true
	case "list":
		input, ok := parseListToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Listing directory", true
			}
			return "List directory", true
		}
		prefix := "List "
		if inProgress {
			prefix = "Listing "
		}
		return prefix + displayToolPath(workspaceRoot, input.Path), true
	case "tree":
		input, ok := parseTreeToolViewInput(call.Input)
		if !ok {
			return "Inspect tree", true
		}
		return "Inspect tree " + displayToolPath(workspaceRoot, input.Path), true
	default:
		return "", false
	}
}

func groupedToolItemMutationLabel(workspaceRoot string, call *events.ToolCallState, inProgress bool) (string, bool) {
	switch strings.TrimSpace(call.ToolName) {
	case "write":
		input, ok := parseWriteToolViewInput(call.Input)
		if !ok {
			if inProgress {
				return "Writing file", true
			}
			if strings.TrimSpace(call.Error) != "" {
				return "Write file", true
			}
			return "Wrote file", true
		}
		name := displayToolBaseName(workspaceRoot, input.Path)
		if inProgress {
			return "Writing " + name, true
		}
		if strings.TrimSpace(call.Error) != "" {
			return "Write " + name, true
		}
		action := "Wrote"
		if call.WriteMutation != nil && !call.WriteMutation.Existed {
			action = "Created"
		}
		return action + " " + name, true
	case "apply_patch":
		paths := applyPatchToolMutationPaths(call)
		pathLabel := mutationGroupedToolPathLabel(workspaceRoot, paths)
		if inProgress {
			switch len(paths) {
			case 1:
				return "Editing " + displayToolBaseName(workspaceRoot, paths[0]), true
			case 0:
				return "Editing files", true
			default:
				return "Editing " + pathLabel, true
			}
		}
		if strings.TrimSpace(call.Error) != "" {
			if pathLabel != "" {
				return "Edit " + pathLabel, true
			}
			return "Edit files", true
		}
		switch len(paths) {
		case 0:
			return "Edited files", true
		case 1:
			return "Edited " + displayToolBaseName(workspaceRoot, paths[0]), true
		default:
			return "Edited " + pathLabel, true
		}
	case "bash":
		if outcomeCategoryForTool(call) != toolOutcomeMutation {
			return "", false
		}
		if call.WriteMutation == nil || strings.TrimSpace(call.WriteMutation.Path) == "" {
			if inProgress {
				return "Shell write", true
			}
			if strings.TrimSpace(call.Error) != "" {
				return "Shell write", true
			}
			return "Shell wrote file", true
		}
		name := displayToolBaseName(workspaceRoot, call.WriteMutation.Path)
		if inProgress {
			return "Writing " + name, true
		}
		if strings.TrimSpace(call.Error) != "" {
			return "Write " + name, true
		}
		action := "Changed"
		if !call.WriteMutation.Existed {
			action = "Created"
		}
		return action + " " + name, true
	default:
		return "", false
	}
}

func groupedToolItemCommandLabel(workspaceRoot string, call *events.ToolCallState) (string, bool) {
	switch strings.TrimSpace(call.ToolName) {
	case "test":
		command := strings.TrimSpace(testCommandDisplayName(workspaceRoot, call))
		if command == "" || command == "test" {
			return "Run tests", true
		}
		return "Run " + command, true
	case "git_status":
		return "Check git status", true
	case "git_diff":
		input, ok := parseGitDiffToolViewInput(call.Input)
		if !ok {
			return "Inspect git diff", true
		}
		if input.Staged {
			return "Inspect staged diff", true
		}
		return "Inspect working diff", true
	case "git_show":
		input, ok := parseGitShowToolViewInput(call.Input)
		if !ok {
			return "Show revision", true
		}
		return "Show " + input.Rev, true
	case "web_search":
		input, ok := parseWebSearchToolViewInput(call.Input)
		if !ok || strings.TrimSpace(input.Query) == "" {
			return "WebSearch", true
		}
		return "WebSearch: " + input.Query, true
	default:
		return "", false
	}
}

func groupedToolItemWorkflowLabel(workspaceRoot string, call *events.ToolCallState) (string, bool) {
	switch strings.TrimSpace(call.ToolName) {
	case "delegate":
		return delegateToolAgentLabel(call), true
	case "question":
		if prompt := strings.TrimSpace(questionToolPrompt(call)); prompt != "" {
			return prompt, true
		}
		return "Question", true
	case "task", "task_workflow", "task_review":
		return taskToolDisplayName(call), true
	default:
		return "", false
	}
}

func toolCallUsesProgressiveLabel(call *events.ToolCallState) bool {
	switch normalizeOutcomeStatus(toolStatus(call)) {
	case "running", "declared", "preparing", "building":
		return true
	default:
		return false
	}
}

func readGroupedToolPathLabel(workspaceRoot string, paths []string) string {
	if label := mutationGroupedToolPathLabel(workspaceRoot, paths); label != "" {
		return label
	}
	return "files"
}

func mutationGroupedToolPathLabel(workspaceRoot string, paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	var labels []string
	for _, path := range paths {
		label := displayToolBaseName(workspaceRoot, path)
		if strings.TrimSpace(label) == "" {
			continue
		}
		labels = append(labels, label)
	}
	if len(labels) == 0 {
		return ""
	}
	return strings.Join(labels, ", ")
}

func displayToolPath(workspaceRoot, path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !filepath.IsAbs(trimmed) {
		return trimmed
	}
	return displaySessionPath(workspaceRoot, trimmed)
}

func displayToolBaseName(workspaceRoot, path string) string {
	display := strings.TrimSpace(displayToolPath(workspaceRoot, path))
	if display == "" {
		return ""
	}
	base := filepath.Base(filepath.Clean(display))
	switch base {
	case "", ".":
		return display
	default:
		return base
	}
}
