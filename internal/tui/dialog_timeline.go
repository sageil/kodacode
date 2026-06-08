package tui

import (
	"fmt"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tui/theme"
)

type timelineDialogResult struct {
	BranchTurnID     string
	TraceTurnID      string
	OpenSessionID    string
	SummarySessionID string
	LabelSessionID   string
	Label            string
}

type timelineItemKind int

const (
	timelineItemTurn timelineItemKind = iota
	timelineItemChildSession
	timelineItemParentSession
)

type timelineFilter int

const (
	timelineFilterAll timelineFilter = iota
	timelineFilterCompleted
	timelineFilterFailed
	timelineFilterWithTools
	timelineFilterBranches
)

type timelineBuildOptions struct {
	Search string
	Filter timelineFilter
	Folded map[string]bool
}

type timelineItem struct {
	Kind       timelineItemKind
	TurnID     string
	SessionID  string
	FoldKey    string
	Title      string
	Label      string
	Meta       string
	Preview    string
	Depth      int
	HasChild   bool
	Folded     bool
	BranchPath []bool
	Text       string
}

type timelineDialog struct {
	id          string
	frameWidth  int
	frameHeight int
	theme       *theme.Theme
	state       events.SessionState
	sessions    []app.SessionSummary
	items       []timelineItem
	search      string
	searching   bool
	filter      timelineFilter
	folded      map[string]bool
	labelInput  textinput.Model
	labelID     string

	paletteListState
}

func newTimelineDialog(state events.SessionState, sessions []app.SessionSummary, th *theme.Theme) *timelineDialog {
	dialog := &timelineDialog{
		id:               dialogIDTimeline,
		frameWidth:       96,
		frameHeight:      32,
		theme:            th,
		state:            state,
		sessions:         append([]app.SessionSummary(nil), sessions...),
		folded:           make(map[string]bool),
		labelInput:       newDialogTextInput(th, 160),
		paletteListState: newPaletteListState(commandPaletteMaxVisible),
	}
	dialog.refilter()
	return dialog
}

func (d *timelineDialog) ID() string { return d.id }

func (d *timelineDialog) ApplyTheme(th *theme.Theme) {
	d.theme = th
	applyDialogInputTheme(&d.labelInput, th)
}

func (d *timelineDialog) SetFrame(width, height int) {
	d.frameWidth = width
	d.frameHeight = height
	d.labelInput.SetWidth(max(desiredDialogWidth(d.frameWidth, 52, 112)-8, 18))
}

func (d *timelineDialog) Update(msg tea.Msg) (dialogModel, tea.Cmd) {
	if strings.TrimSpace(d.labelID) != "" {
		return d.updateLabel(msg)
	}
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		if d.searching {
			return d.updateSearch(typed)
		}
		switch typed.String() {
		case "q", "esc", "ctrl+c":
			return d, closeDialog(d.id, nil)
		case "/":
			d.searching = true
			return d, nil
		case "f":
			d.filter = (d.filter + 1) % 5
			d.refilter()
			return d, nil
		case "1":
			d.setFilter(timelineFilterAll)
			return d, nil
		case "2":
			d.setFilter(timelineFilterCompleted)
			return d, nil
		case "3":
			d.setFilter(timelineFilterFailed)
			return d, nil
		case "4":
			d.setFilter(timelineFilterWithTools)
			return d, nil
		case "5":
			d.setFilter(timelineFilterBranches)
			return d, nil
		case "up", "k":
			if d.cursor > 0 {
				d.moveCursor(-1, len(d.items))
			}
			return d, nil
		case "down", "j":
			if d.cursor < len(d.items)-1 {
				d.moveCursor(1, len(d.items))
			}
			return d, nil
		case "pgup", "ctrl+u":
			d.moveCursor(-d.visibleLimit(), len(d.items))
			return d, nil
		case "pgdown", "ctrl+d":
			d.moveCursor(d.visibleLimit(), len(d.items))
			return d, nil
		case "home", "g":
			d.cursor = 0
			d.ensureVisible(len(d.items))
			return d, nil
		case "end", "G":
			d.cursor = max(len(d.items)-1, 0)
			d.ensureVisible(len(d.items))
			return d, nil
		case "left":
			if item, ok := d.selectedItem(); ok && item.HasChild {
				if foldKey := timelineItemFoldKey(item); foldKey != "" {
					d.folded[foldKey] = true
					d.refilter()
				}
			}
			return d, nil
		case "right":
			if item, ok := d.selectedItem(); ok && item.HasChild {
				if foldKey := timelineItemFoldKey(item); foldKey != "" {
					delete(d.folded, foldKey)
					d.refilter()
				}
			}
			return d, nil
		case " ":
			if item, ok := d.selectedItem(); ok && item.HasChild {
				if foldKey := timelineItemFoldKey(item); foldKey != "" {
					if d.folded[foldKey] {
						delete(d.folded, foldKey)
					} else {
						d.folded[foldKey] = true
					}
					d.refilter()
				}
			}
			return d, nil
		case "e":
			if item, ok := d.selectedItem(); ok && timelineItemIsSession(item) {
				d.labelID = item.SessionID
				d.labelInput.SetValue(item.Title)
				d.labelInput.CursorEnd()
				return d, d.labelInput.Focus()
			}
			return d, nil
		case "s":
			if item, ok := d.selectedItem(); ok && item.Kind == timelineItemChildSession && strings.TrimSpace(item.SessionID) != "" {
				return d, closeDialog(d.id, timelineDialogResult{SummarySessionID: item.SessionID})
			}
			return d, nil
		case "t":
			if item, ok := d.selectedItem(); ok && item.Kind == timelineItemTurn {
				return d, closeDialog(d.id, timelineDialogResult{TraceTurnID: item.TurnID})
			}
		case "b", "enter":
			if item, ok := d.selectedItem(); ok {
				switch item.Kind {
				case timelineItemTurn:
					if timelineTurnBranchable(d.state.Turns[item.TurnID]) {
						return d, closeDialog(d.id, timelineDialogResult{BranchTurnID: item.TurnID})
					}
				case timelineItemChildSession, timelineItemParentSession:
					if strings.TrimSpace(item.SessionID) != "" {
						return d, closeDialog(d.id, timelineDialogResult{OpenSessionID: item.SessionID})
					}
				}
			}
		}
	}
	return d, nil
}

func (d *timelineDialog) Draw(surface dialogSurface, area dialogRenderArea) *tea.Cursor {
	d.SetFrame(area.width, area.height)
	width := desiredDialogWidth(d.frameWidth, 52, 112)
	content := renderPaletteDialogContentSized(d.theme, max(width-dialogFrameInset*2, 1), dialogPaletteFrame{
		Prompt: d.promptView(),
		Body:   d.bodyView(width),
		Hint:   d.hintView(),
	}, d.dialogContentMinBodyHeight())
	return drawDialogFrameOnSurface(surface, area, d.theme, width, content, nil)
}

func (d *timelineDialog) promptView() string {
	if strings.TrimSpace(d.labelID) != "" {
		return "Label Branch"
	}
	prompt := "Timeline"
	if d.filter != timelineFilterAll {
		prompt += " [" + timelineFilterLabel(d.filter) + "]"
	}
	if d.search != "" || d.searching {
		prompt += " /" + d.search
	}
	return prompt
}

func (d *timelineDialog) hintView() string {
	if strings.TrimSpace(d.labelID) != "" {
		return "enter save | esc cancel"
	}
	if d.searching {
		return "typing search | backspace edit | enter keep | esc leave search"
	}
	return "enter open/branch | b branch | s summarize | t trace | e label | / search | f filter | space fold"
}

func (d *timelineDialog) bodyView(width int) string {
	if strings.TrimSpace(d.labelID) != "" {
		return strings.Join([]string{
			dialogHintStyle(d.theme).Render("Branch session label"),
			d.labelInput.View(),
		}, "\n")
	}
	rows := []string{dialogSectionStyle(d.theme).Render("SESSION TREE")}
	if len(d.items) == 0 {
		rows = append(rows, dialogHintStyle(d.theme).Render("  no matching timeline entries"))
		return strings.Join(rows, "\n")
	}
	selected := dialogSelectedItemStyle(d.theme)
	normal := dialogItemStyle(d.theme)
	muted := dialogHintStyle(d.theme)
	start, end := d.visibleRange(len(d.items))
	rowWidth := max(width-8, 12)
	for idx, item := range d.items[start:end] {
		prefix := "  "
		style := normal
		if start+idx == d.cursor {
			prefix = "> "
			style = selected
		}
		line := joinBar(timelineItemDisplayLabel(item), item.Meta, rowWidth)
		rows = append(rows, style.Render(prefix+line))
		if start+idx == d.cursor && item.Kind == timelineItemTurn {
			turn := d.state.Turns[item.TurnID]
			if turn != nil && strings.TrimSpace(turn.UserText) != "" {
				userText := strings.TrimSpace(turn.UserText)
				rows = append(rows, muted.Render("  "+truncateEnd(strings.ReplaceAll(userText, "\n", " "), rowWidth)))
			}
		} else if start+idx == d.cursor && strings.TrimSpace(item.Preview) != "" {
			rows = append(rows, muted.Render("  "+truncateEnd(item.Preview, rowWidth)))
		}
	}
	return strings.Join(rows, "\n")
}

func (d *timelineDialog) dialogContentMinBodyHeight() int {
	contentHeight := min(max(d.frameHeight-2, 1), commandPaletteDefaultModalHeight-2)
	return max(contentHeight-3, 1)
}

func (d *timelineDialog) selectedItem() (timelineItem, bool) {
	if len(d.items) == 0 || d.cursor < 0 || d.cursor >= len(d.items) {
		return timelineItem{}, false
	}
	return d.items[d.cursor], true
}

func (d *timelineDialog) updateSearch(msg tea.KeyPressMsg) (dialogModel, tea.Cmd) {
	switch msg.String() {
	case "enter":
		d.searching = false
		return d, nil
	case "esc":
		d.searching = false
		return d, nil
	case "backspace", "ctrl+h":
		if d.search != "" {
			d.search = d.search[:len(d.search)-1]
			d.refilter()
		}
		return d, nil
	case "ctrl+u":
		d.search = ""
		d.refilter()
		return d, nil
	}
	if text := printableKeyText(msg.String()); text != "" {
		d.search += text
		d.refilter()
	}
	return d, nil
}

func (d *timelineDialog) updateLabel(msg tea.Msg) (dialogModel, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.KeyPressMsg:
		switch typed.String() {
		case "esc", "ctrl+c":
			d.labelID = ""
			d.labelInput.SetValue("")
			d.labelInput.Blur()
			return d, nil
		case "enter":
			sessionID := strings.TrimSpace(d.labelID)
			label := strings.TrimSpace(d.labelInput.Value())
			d.labelID = ""
			d.labelInput.SetValue("")
			d.labelInput.Blur()
			if sessionID != "" && label != "" {
				return d, closeDialog(d.id, timelineDialogResult{LabelSessionID: sessionID, Label: label})
			}
			return d, nil
		}
	}
	var cmd tea.Cmd
	d.labelInput, cmd = d.labelInput.Update(msg)
	return d, cmd
}

func (d *timelineDialog) setFilter(filter timelineFilter) {
	d.filter = filter
	d.refilter()
}

func (d *timelineDialog) refilter() {
	selectedID := ""
	if item, ok := d.selectedItem(); ok {
		selectedID = timelineItemStableID(item)
	}
	d.items = buildTimelineItems(d.state, d.sessions, timelineBuildOptions{
		Search: d.search,
		Filter: d.filter,
		Folded: d.folded,
	})
	d.cursor = min(d.cursor, max(len(d.items)-1, 0))
	if selectedID != "" {
		for idx, item := range d.items {
			if timelineItemStableID(item) == selectedID {
				d.cursor = idx
				break
			}
		}
	}
	d.ensureVisible(len(d.items))
}

func buildTimelineItems(state events.SessionState, sessions []app.SessionSummary, options timelineBuildOptions) []timelineItem {
	turnIDs := orderedSessionTurnIDs(state)
	children := timelineChildSessionsByParent(sessions)
	items := make([]timelineItem, 0, len(turnIDs)+len(children[state.SessionID]))
	if state.Branch != nil && strings.TrimSpace(state.Branch.ParentSessionID) != "" {
		parentTitle := timelineSessionTitle(sessions, state.Branch.ParentSessionID)
		if parentTitle == "" {
			parentTitle = "Parent session"
		}
		item := timelineItem{
			Kind:      timelineItemParentSession,
			SessionID: state.Branch.ParentSessionID,
			FoldKey:   timelineSessionFoldKey(state.Branch.ParentSessionID),
			Title:     parentTitle,
			Label:     "parent: " + parentTitle,
			Meta:      "branched from " + shortSessionRef(state.Branch.ParentTurnID),
			Preview:   "parent session " + shortSessionRef(state.Branch.ParentSessionID) + " at turn " + shortSessionRef(state.Branch.ParentTurnID),
			Text:      parentTitle + " " + state.Branch.ParentSessionID + " " + state.Branch.ParentTurnID,
		}
		if timelineItemVisible(item, state, children, options) {
			items = append(items, item)
		}
	}
	for idx, turnID := range turnIDs {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		title := strings.TrimSpace(turn.UserText)
		if title == "" {
			title = "turn"
		}
		title = strings.ReplaceAll(title, "\n", " ")
		meta := []string{costDialogTurnStatus(turn)}
		if len(turn.ToolCallOrder) > 0 {
			meta = append(meta, fmt.Sprintf("%d tools", len(turn.ToolCallOrder)))
		}
		if turn.ProviderUsage != nil && turn.ProviderUsage.EstimatedInputCost+turn.ProviderUsage.EstimatedOutputCost > 0 {
			meta = append(meta, formatEstimatedCost(turn.ProviderUsage.EstimatedInputCost+turn.ProviderUsage.EstimatedOutputCost))
		}
		foldKey := timelineTurnFoldKey(turnID)
		item := timelineItem{
			Kind:     timelineItemTurn,
			TurnID:   turnID,
			FoldKey:  foldKey,
			Title:    title,
			Label:    fmt.Sprintf("%d. %s", idx+1, title),
			Meta:     strings.Join(meta, " | "),
			HasChild: len(timelineChildrenForTurn(state.SessionID, turnID, children)) > 0,
			Folded:   options.Folded[foldKey],
			Text:     title + " " + turnID + " " + strings.Join(meta, " "),
		}
		if timelineItemVisible(item, state, children, options) {
			items = append(items, item)
		}
		if item.Folded && strings.TrimSpace(options.Search) == "" {
			continue
		}
		turnChildren := timelineChildrenForTurn(state.SessionID, turnID, children)
		for childIdx, child := range turnChildren {
			items = appendTimelineBranchSessionItems(items, child, children, []bool{childIdx < len(turnChildren)-1}, options)
		}
	}
	return items
}

func appendTimelineBranchSessionItems(items []timelineItem, summary app.SessionSummary, children map[string][]app.SessionSummary, branchPath []bool, options timelineBuildOptions) []timelineItem {
	title := strings.TrimSpace(summary.Title)
	if title == "" {
		title = "Branched session"
	}
	hasChildren := len(children[summary.ID]) > 0
	parentTurnID := ""
	if summary.Branch != nil {
		parentTurnID = strings.TrimSpace(summary.Branch.ParentTurnID)
	}
	foldKey := timelineSessionFoldKey(summary.ID)
	item := timelineItem{
		Kind:       timelineItemChildSession,
		SessionID:  summary.ID,
		FoldKey:    foldKey,
		Title:      title,
		Label:      title,
		Meta:       timelineBranchMeta(summary, len(children[summary.ID])),
		Preview:    timelineBranchPreview(summary, len(children[summary.ID]), parentTurnID),
		Depth:      len(branchPath),
		HasChild:   hasChildren,
		Folded:     options.Folded[foldKey] || options.Folded[summary.ID],
		BranchPath: append([]bool(nil), branchPath...),
		Text:       title + " " + summary.ID + " " + parentTurnID,
	}
	item.Text += " " + item.Meta + " " + item.Preview
	if timelineItemVisible(item, events.SessionState{}, children, options) {
		items = append(items, item)
	}
	if hasChildren && (!item.Folded || strings.TrimSpace(options.Search) != "") {
		childSessions := children[summary.ID]
		for childIdx, child := range childSessions {
			childPath := append(append([]bool(nil), branchPath...), childIdx < len(childSessions)-1)
			items = appendTimelineBranchSessionItems(items, child, children, childPath, options)
		}
	}
	return items
}

func timelineChildSessionsByParent(sessions []app.SessionSummary) map[string][]app.SessionSummary {
	children := make(map[string][]app.SessionSummary)
	for _, summary := range sessions {
		if summary.Branch == nil {
			continue
		}
		parentSessionID := strings.TrimSpace(summary.Branch.ParentSessionID)
		if parentSessionID == "" {
			continue
		}
		children[parentSessionID] = append(children[parentSessionID], summary)
	}
	return children
}

func timelineChildrenForTurn(sessionID, turnID string, children map[string][]app.SessionSummary) []app.SessionSummary {
	var matches []app.SessionSummary
	for _, child := range children[sessionID] {
		if child.Branch != nil && strings.TrimSpace(child.Branch.ParentTurnID) == turnID {
			matches = append(matches, child)
		}
	}
	return matches
}

func timelineSessionTitle(sessions []app.SessionSummary, sessionID string) string {
	for _, summary := range sessions {
		if summary.ID == sessionID {
			return strings.TrimSpace(summary.Title)
		}
	}
	return ""
}

func timelineTurnBranchable(turn *events.TurnState) bool {
	return turn != nil && turn.Status != events.TurnStatusRunning && turn.CompletedAtSeq > 0
}

func timelineItemVisible(item timelineItem, state events.SessionState, children map[string][]app.SessionSummary, options timelineBuildOptions) bool {
	switch options.Filter {
	case timelineFilterCompleted:
		return item.Kind == timelineItemTurn && state.Turns[item.TurnID] != nil && state.Turns[item.TurnID].Status == events.TurnStatusCompleted && timelineItemMatchesSearch(item, options.Search)
	case timelineFilterFailed:
		if item.Kind != timelineItemTurn || state.Turns[item.TurnID] == nil {
			return false
		}
		status := state.Turns[item.TurnID].Status
		return (status == events.TurnStatusFailed || status == events.TurnStatusCanceled) && timelineItemMatchesSearch(item, options.Search)
	case timelineFilterWithTools:
		return item.Kind == timelineItemTurn && state.Turns[item.TurnID] != nil && len(state.Turns[item.TurnID].ToolCallOrder) > 0 && timelineItemMatchesSearch(item, options.Search)
	case timelineFilterBranches:
		if timelineItemIsSession(item) {
			return timelineItemMatchesSearch(item, options.Search)
		}
		if item.Kind == timelineItemTurn {
			return item.HasChild && timelineItemMatchesSearch(item, options.Search)
		}
		return false
	default:
		return timelineItemMatchesSearch(item, options.Search)
	}
}

func timelineItemMatchesSearch(item timelineItem, search string) bool {
	search = strings.TrimSpace(strings.ToLower(search))
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(item.Text+" "+item.Label+" "+item.Meta), search)
}

func timelineItemIsSession(item timelineItem) bool {
	return item.Kind == timelineItemParentSession || item.Kind == timelineItemChildSession
}

func timelineItemFoldKey(item timelineItem) string {
	if strings.TrimSpace(item.FoldKey) != "" {
		return item.FoldKey
	}
	switch item.Kind {
	case timelineItemTurn:
		return timelineTurnFoldKey(item.TurnID)
	case timelineItemChildSession, timelineItemParentSession:
		return timelineSessionFoldKey(item.SessionID)
	default:
		return ""
	}
}

func timelineTurnFoldKey(turnID string) string {
	if strings.TrimSpace(turnID) == "" {
		return ""
	}
	return "turn:" + turnID
}

func timelineSessionFoldKey(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return ""
	}
	return "session:" + sessionID
}

func timelineItemDisplayLabel(item timelineItem) string {
	label := item.Label
	if item.Kind == timelineItemTurn && item.HasChild {
		if item.Folded {
			label = "▸ " + label
		} else {
			label = "▾ " + label
		}
	} else if item.Kind == timelineItemChildSession {
		prefix := timelineTreePrefix(item.BranchPath)
		if item.HasChild {
			if item.Folded {
				prefix += "▸ "
			} else {
				prefix += "▾ "
			}
		}
		label = prefix + label
	}
	return truncateEnd(label, 78)
}

func timelineTreePrefix(branchPath []bool) string {
	if len(branchPath) == 0 {
		return ""
	}
	var builder strings.Builder
	for _, hasNext := range branchPath[:len(branchPath)-1] {
		if hasNext {
			builder.WriteString("│  ")
		} else {
			builder.WriteString("   ")
		}
	}
	if branchPath[len(branchPath)-1] {
		builder.WriteString("├─ ")
	} else {
		builder.WriteString("└─ ")
	}
	return builder.String()
}

func timelineBranchMeta(summary app.SessionSummary, childCount int) string {
	parts := []string{timelineSessionStatusLabel(summary.Status)}
	if summary.BranchSummary != nil && strings.TrimSpace(summary.BranchSummary.Summary) != "" {
		parts = append(parts, "summary")
	}
	if childCount > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", childCount, timelinePlural(childCount, "child", "children")))
	}
	if updated := relativeTimeUnix(summary.UpdatedAt.Unix()); updated != "" {
		parts = append(parts, updated)
	}
	return strings.Join(parts, " | ")
}

func timelineBranchPreview(summary app.SessionSummary, childCount int, parentTurnID string) string {
	if summary.BranchSummary != nil && strings.TrimSpace(summary.BranchSummary.Summary) != "" {
		return flattenTimelinePreview(summary.BranchSummary.Summary)
	}
	parts := []string{
		"branch from turn " + shortSessionRef(parentTurnID),
		"session " + shortSessionRef(summary.ID),
		timelineSessionStatusLabel(summary.Status),
	}
	if childCount > 0 {
		parts = append(parts, fmt.Sprintf("%d child %s", childCount, timelinePlural(childCount, "branch", "branches")))
	}
	if updated := timelineUpdatedPreview(summary.UpdatedAt.Unix()); updated != "" {
		parts = append(parts, updated)
	}
	return strings.Join(parts, " | ")
}

func shortSessionRef(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 8 {
		return id
	}
	return id[:8]
}

func flattenTimelinePreview(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func timelineUpdatedPreview(unixSeconds int64) string {
	updated := relativeTimeUnix(unixSeconds)
	switch {
	case updated == "":
		return ""
	case updated == "now":
		return "updated now"
	case strings.Contains(updated, "-"):
		return "updated " + updated
	default:
		return "updated " + updated + " ago"
	}
}

func timelineSessionStatusLabel(status events.TurnStatus) string {
	if trimmed := strings.TrimSpace(string(status)); trimmed != "" {
		return trimmed
	}
	return "waiting"
}

func timelinePlural(count int, singular, plural string) string {
	if count == 1 {
		return singular
	}
	return plural
}

func timelineItemStableID(item timelineItem) string {
	if timelineItemIsSession(item) {
		return "session:" + item.SessionID
	}
	return "turn:" + item.TurnID
}

func timelineFilterLabel(filter timelineFilter) string {
	switch filter {
	case timelineFilterCompleted:
		return "completed"
	case timelineFilterFailed:
		return "failed"
	case timelineFilterWithTools:
		return "with tools"
	case timelineFilterBranches:
		return "branches"
	default:
		return "all"
	}
}

func printableKeyText(value string) string {
	if value == "" {
		return ""
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return value
}
