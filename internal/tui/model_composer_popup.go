package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sageil/kodacode/internal/app"
)

const composerPopupMaxVisible = 6
const composerPromptHistoryLimit = 24

type composerPopupMode string

const (
	composerPopupNone    composerPopupMode = ""
	composerPopupSlash   composerPopupMode = "slash"
	composerPopupHistory composerPopupMode = "history"
	composerPopupSkills  composerPopupMode = "skills"
	composerPopupPaths   composerPopupMode = "paths"
)

type composerPopupItem struct {
	ID    string
	Title string
	Meta  string
	Value string
	Arg   string
}

func loadPromptHistoryCmd(ctx context.Context, backend Backend, limit int) tea.Cmd {
	return func() tea.Msg {
		entries, err := backend.ListPromptHistory(ctx, limit)
		return promptHistoryLoadedMsg{
			entries: entries,
			err:     err,
		}
	}
}

func loadComposerSkillsCmd(ctx context.Context, backend Backend, workspaceRoot string) tea.Cmd {
	return func() tea.Msg {
		skills, err := backend.ListSkills(ctx, workspaceRoot)
		return composerSkillsLoadedMsg{
			skills: skills,
			err:    err,
		}
	}
}

func loadComposerWorkspacePathsCmd(ctx context.Context, backend Backend, workspaceRoot string) tea.Cmd {
	return func() tea.Msg {
		paths, err := backend.ListWorkspacePaths(ctx, workspaceRoot)
		return composerWorkspacePathsLoadedMsg{
			paths: paths,
			err:   err,
		}
	}
}

func (m *Model) openComposerHistory() tea.Cmd {
	m.clearComposerError()
	m.composerState.popupMode = composerPopupHistory
	m.composerState.popupCursor = 0
	return m.ensurePromptHistoryLoaded()
}

func (m *Model) ensureComposerWorkspacePathsLoaded() tea.Cmd {
	if m.composerState.workspacePathsLoaded || m.composerState.workspacePathsBusy {
		return nil
	}
	m.composerState.workspacePathsBusy = true
	return loadComposerWorkspacePathsCmd(m.ctx, m.backend, m.workspace)
}

func (m *Model) ensureComposerSkillsLoaded() tea.Cmd {
	if m.composerState.skillsLoaded || m.composerState.skillsBusy {
		return nil
	}
	m.composerState.skillsBusy = true
	return loadComposerSkillsCmd(m.ctx, m.backend, m.workspace)
}

func (m *Model) ensurePromptHistoryLoaded() tea.Cmd {
	if m.composerState.promptHistoryLoaded || m.composerState.promptHistoryBusy {
		return nil
	}
	m.composerState.promptHistoryBusy = true
	return loadPromptHistoryCmd(m.ctx, m.backend, composerPromptHistoryLimit)
}

func (m *Model) recordPromptHistoryPrompt(prompt, turnID string) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return
	}
	entry := app.PromptHistoryEntry{
		SessionID: strings.TrimSpace(m.sessionID),
		TurnID:    strings.TrimSpace(turnID),
		Prompt:    prompt,
		UpdatedAt: time.Now().UTC(),
	}
	m.composerState.promptHistoryPending = mergePromptHistoryEntries(
		composerPromptHistoryLimit,
		[]app.PromptHistoryEntry{entry},
		m.composerState.promptHistoryPending,
	)
}

func (m Model) composerPromptHistoryEntries() []app.PromptHistoryEntry {
	return dedupePromptHistoryEntries(mergePromptHistoryEntries(
		composerPromptHistoryLimit,
		m.composerState.promptHistoryPending,
		m.composerState.promptHistory,
	))
}

func mergePromptHistoryEntries(limit int, groups ...[]app.PromptHistoryEntry) []app.PromptHistoryEntry {
	if limit <= 0 {
		limit = composerPromptHistoryLimit
	}
	merged := make([]app.PromptHistoryEntry, 0, limit)
	for _, group := range groups {
		for _, entry := range group {
			entry.Prompt = strings.TrimSpace(entry.Prompt)
			if entry.Prompt == "" {
				continue
			}
			merged = append(merged, entry)
			if len(merged) >= limit {
				return merged
			}
		}
	}
	return merged
}

func dedupePromptHistoryEntries(entries []app.PromptHistoryEntry) []app.PromptHistoryEntry {
	if len(entries) == 0 {
		return nil
	}
	deduped := make([]app.PromptHistoryEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		key := normalizePromptHistoryPromptKey(entry.Prompt)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, entry)
	}
	return deduped
}

func filterPendingPromptHistoryEntries(pending, persisted []app.PromptHistoryEntry) []app.PromptHistoryEntry {
	if len(pending) == 0 {
		return nil
	}
	filtered := make([]app.PromptHistoryEntry, 0, len(pending))
	for _, entry := range pending {
		if strings.TrimSpace(entry.Prompt) == "" {
			continue
		}
		if promptHistoryEntryPersisted(entry, persisted) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func promptHistoryEntryPersisted(entry app.PromptHistoryEntry, persisted []app.PromptHistoryEntry) bool {
	for _, candidate := range persisted {
		if promptHistoryEntriesMatch(entry, candidate) {
			return true
		}
	}
	return false
}

func promptHistoryEntriesMatch(left, right app.PromptHistoryEntry) bool {
	if strings.TrimSpace(left.Prompt) != strings.TrimSpace(right.Prompt) {
		return false
	}
	leftTurnID := strings.TrimSpace(left.TurnID)
	rightTurnID := strings.TrimSpace(right.TurnID)
	if leftTurnID != "" && rightTurnID != "" && leftTurnID == rightTurnID {
		leftSessionID := strings.TrimSpace(left.SessionID)
		rightSessionID := strings.TrimSpace(right.SessionID)
		return leftSessionID == "" || rightSessionID == "" || leftSessionID == rightSessionID
	}
	if leftTurnID != "" || rightTurnID != "" {
		return false
	}
	leftSessionID := strings.TrimSpace(left.SessionID)
	rightSessionID := strings.TrimSpace(right.SessionID)
	return leftSessionID != "" && leftSessionID == rightSessionID
}

func normalizePromptHistoryPromptKey(prompt string) string {
	fields := strings.Fields(strings.TrimSpace(prompt))
	if len(fields) == 0 {
		return ""
	}
	return strings.Join(fields, " ")
}

func (m *Model) dismissComposerPopup() {
	m.composerState.popupMode = composerPopupNone
	m.composerState.popupCursor = 0
}

func (m *Model) refreshComposerPopup() tea.Cmd {
	if !m.composerInputEnabled() {
		m.dismissComposerPopup()
		return nil
	}
	if m.composerState.popupMode == composerPopupHistory {
		m.clampComposerPopupCursor()
		return m.ensurePromptHistoryLoaded()
	}
	if _, _, _, ok := composerPathQueryAtCursor(m.composer.Value(), m.composerCursorOffset()); ok {
		m.composerState.popupMode = composerPopupPaths
		m.clampComposerPopupCursor()
		return m.ensureComposerWorkspacePathsLoaded()
	}
	if _, _, _, ok := composerSkillQueryAtCursor(m.composer.Value(), m.composerCursorOffset()); ok {
		m.composerState.popupMode = composerPopupSkills
		m.clampComposerPopupCursor()
		if m.composerState.skillsLoaded && m.shouldDismissEmptySkillPopup() {
			m.dismissComposerPopup()
			return nil
		}
		return m.ensureComposerSkillsLoaded()
	}
	if _, ok := composerSlashQuery(m.composer.Value()); ok {
		m.composerState.popupMode = composerPopupSlash
		m.clampComposerPopupCursor()
		return nil
	}
	m.dismissComposerPopup()
	return nil
}

func (m *Model) clampComposerPopupCursor() {
	items := m.composerPopupItems()
	if len(items) == 0 {
		m.composerState.popupCursor = 0
		return
	}
	if m.composerState.popupCursor < 0 {
		m.composerState.popupCursor = 0
	}
	if m.composerState.popupCursor >= len(items) {
		m.composerState.popupCursor = len(items) - 1
	}
}

func (m *Model) moveComposerPopup(delta int) {
	items := m.composerPopupItems()
	if len(items) == 0 {
		m.composerState.popupCursor = 0
		return
	}
	m.composerState.popupCursor += delta
	if m.composerState.popupCursor < 0 {
		m.composerState.popupCursor = 0
	}
	if m.composerState.popupCursor >= len(items) {
		m.composerState.popupCursor = len(items) - 1
	}
}

func (m *Model) moveComposerHistoryPopup(delta int) {
	items := m.composerPopupItems()
	if len(items) == 0 {
		m.composerState.popupCursor = 0
		return
	}
	if len(items) == 1 {
		m.composerState.popupCursor = 0
		return
	}
	m.composerState.popupCursor = (m.composerState.popupCursor + delta) % len(items)
	if m.composerState.popupCursor < 0 {
		m.composerState.popupCursor += len(items)
	}
}

func (m *Model) resetComposerHistoryRecall() {
	m.composerState.historyRecallActive = false
	m.composerState.historyRecallDraft = ""
	m.composerState.historyRecallIndex = 0
}

func (m *Model) beginComposerHistoryRecall() {
	if m.composerState.historyRecallActive {
		return
	}
	m.composerState.historyRecallActive = true
	m.composerState.historyRecallDraft = m.composer.Value()
	m.composerState.historyRecallIndex = -1
}

func (m Model) canRecallComposerHistory() bool {
	return m.composerState.historyRecallActive || !strings.Contains(m.composer.Value(), "\n")
}

func (m *Model) stepComposerHistoryRecall(direction int) bool {
	next := m.composerState.historyRecallIndex + direction
	if next < -1 {
		next = -1
	}
	if m.composerState.promptHistoryLoaded {
		maxIndex := len(m.composerPromptHistoryEntries()) - 1
		if next > maxIndex {
			next = maxIndex
		}
	}
	if next == m.composerState.historyRecallIndex {
		return false
	}
	m.composerState.historyRecallIndex = next
	return true
}

func (m *Model) applyComposerHistoryRecall() {
	if !m.composerState.historyRecallActive {
		return
	}
	entries := m.composerPromptHistoryEntries()
	if m.composerState.historyRecallIndex < 0 || len(entries) == 0 {
		m.composer.SetValue(m.composerState.historyRecallDraft)
		m.resetComposerHistoryRecall()
		return
	}
	if m.composerState.historyRecallIndex >= len(entries) {
		m.composerState.historyRecallIndex = len(entries) - 1
	}
	m.composer.SetValue(entries[m.composerState.historyRecallIndex].Prompt)
}

func (m Model) handleComposerHistoryNavigation(delta int) (tea.Model, tea.Cmd, bool) {
	if m.composerState.popupMode == composerPopupSlash || m.composerState.popupMode == composerPopupSkills || m.composerState.popupMode == composerPopupPaths {
		return m, nil, false
	}
	if _, ok := composerSlashQuery(m.composer.Value()); ok && m.composerState.popupMode == composerPopupNone {
		return m, nil, false
	}
	if m.composerState.popupMode == composerPopupHistory {
		m.moveComposerHistoryPopup(delta)
		return m, m.ensurePromptHistoryLoaded(), true
	}
	if !m.canRecallComposerHistory() {
		return m, nil, false
	}
	direction := -delta
	if direction < 0 && !m.composerState.historyRecallActive {
		return m, nil, true
	}
	m.beginComposerHistoryRecall()
	if !m.stepComposerHistoryRecall(direction) {
		if m.composerState.historyRecallIndex < 0 {
			m.resetComposerHistoryRecall()
		}
		return m, nil, true
	}
	m.clearComposerError()
	if !m.composerState.promptHistoryLoaded {
		if m.composerState.historyRecallIndex < 0 {
			m.applyComposerHistoryRecall()
			return m, nil, true
		}
		if m.composerState.historyRecallIndex < len(m.composerPromptHistoryEntries()) {
			m.applyComposerHistoryRecall()
		}
		return m, m.ensurePromptHistoryLoaded(), true
	}
	m.applyComposerHistoryRecall()
	return m, nil, true
}

func (m Model) selectedComposerPopupItem() (composerPopupItem, bool) {
	items := m.composerPopupItems()
	if len(items) == 0 || m.composerState.popupCursor < 0 || m.composerState.popupCursor >= len(items) {
		return composerPopupItem{}, false
	}
	return items[m.composerState.popupCursor], true
}

func (m Model) composerPopupItems() []composerPopupItem {
	switch m.composerState.popupMode {
	case composerPopupSlash:
		return composerSlashItems(m, m.composer.Value(), availableComposerCommands(m))
	case composerPopupHistory:
		return composerHistoryItems(strings.TrimSpace(m.composer.Value()), m.composerPromptHistoryEntries())
	case composerPopupSkills:
		return composerSkillItems(m)
	case composerPopupPaths:
		return composerPathItems(m)
	default:
		return nil
	}
}

func composerPathItems(m Model) []composerPopupItem {
	query, _, _, ok := composerPathQueryAtCursor(m.composer.Value(), m.composerCursorOffset())
	if !ok || !m.composerState.workspacePathsLoaded {
		return nil
	}
	query = strings.TrimSpace(strings.ToLower(query))
	type scoredItem struct {
		item  composerPopupItem
		score int
	}
	matches := make([]scoredItem, 0, len(m.composerState.workspacePaths))
	for _, path := range m.composerState.workspacePaths {
		id := cleanWorkspaceFocusPath(path.Path)
		if id == "" {
			continue
		}
		matchText := strings.ToLower(id)
		score := 0
		if query != "" {
			ok, nextScore := fuzzyScore(query, matchText)
			if !ok {
				continue
			}
			score = nextScore
		}
		meta := "file"
		title := id
		if path.IsDir {
			meta = "directory"
			title += "/"
		}
		matches = append(matches, scoredItem{
			item: composerPopupItem{
				ID:    id,
				Title: title,
				Meta:  meta,
			},
			score: score,
		})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].item.ID < matches[j].item.ID
	})
	items := make([]composerPopupItem, 0, len(matches))
	for _, match := range matches {
		items = append(items, match.item)
	}
	return items
}

func (m Model) shouldDismissEmptySkillPopup() bool {
	query, _, _, ok := composerSkillQueryAtCursor(m.composer.Value(), m.composerCursorOffset())
	if !ok || strings.TrimSpace(query) == "" {
		return false
	}
	return len(composerSkillItems(m)) == 0
}

func composerSkillItems(m Model) []composerPopupItem {
	query, _, _, ok := composerSkillQueryAtCursor(m.composer.Value(), m.composerCursorOffset())
	if !ok || !m.composerState.skillsLoaded {
		return nil
	}
	query = strings.TrimSpace(strings.ToLower(query))
	items := buildSkillItems(m.composerState.skills)
	out := make([]composerPopupItem, 0, len(items))
	for _, skill := range items {
		id := strings.TrimSpace(skill.ID)
		if id == "" {
			continue
		}
		matchText := id + " " + skill.Description + " " + skill.Source
		if query != "" {
			if ok, _ := fuzzyScore(query, strings.ToLower(matchText)); !ok {
				continue
			}
		}
		meta := skillSourceLabel(skill.Source)
		if desc := strings.TrimSpace(skill.Description); desc != "" {
			meta = strings.TrimSpace(meta + " · " + desc)
		}
		out = append(out, composerPopupItem{
			ID:    id,
			Title: "$" + id,
			Meta:  meta,
			Value: "$" + id,
		})
	}
	return out
}

func composerSkillQueryAtCursor(value string, cursor int) (query string, start int, end int, ok bool) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	left := cursor
	for left > 0 && isSkillNameRune(runes[left-1]) {
		left--
	}
	if left == 0 || runes[left-1] != '$' {
		return "", 0, 0, false
	}

	right := cursor
	for right < len(runes) && isSkillNameRune(runes[right]) {
		right++
	}
	return string(runes[left:cursor]), left - 1, right, true
}

func composerPathQueryAtCursor(value string, cursor int) (query string, start int, end int, ok bool) {
	runes := []rune(value)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}

	left := cursor
	for left > 0 && isPathMentionRune(runes[left-1]) {
		left--
	}
	if left == 0 || runes[left-1] != '@' {
		return "", 0, 0, false
	}

	right := cursor
	for right < len(runes) && isPathMentionRune(runes[right]) {
		right++
	}
	return string(runes[left:cursor]), left - 1, right, true
}

func isPathMentionRune(r rune) bool {
	if isWhitespaceRune(r) {
		return false
	}
	switch r {
	case '@', '[', ']', '(', ')', '{', '}', '"', '\'', '`':
		return false
	default:
		return true
	}
}

func isSkillNameRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= 'A' && r <= 'Z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '_' || r == '-' || r == '.':
		return true
	default:
		return false
	}
}

func composerSlashItems(m Model, text string, commands []composerCommand) []composerPopupItem {
	query, ok := composerSlashQuery(text)
	if !ok {
		return nil
	}
	type scoredItem struct {
		item  composerPopupItem
		score int
	}
	matches := make([]scoredItem, 0, len(commands))
	for _, command := range commands {
		for _, item := range composerSlashPopupItems(m, command) {
			name := strings.TrimPrefix(command.Name, "/")
			if query == "" {
				matches = append(matches, scoredItem{
					item: item,
				})
				continue
			}
			matchText := name + " " + item.Title + " " + item.Meta + " " + command.Description
			if ok, score := fuzzyScore(query, matchText); ok {
				matches = append(matches, scoredItem{
					item:  item,
					score: score,
				})
			}
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})
	items := make([]composerPopupItem, 0, len(matches))
	for _, match := range matches {
		items = append(items, match.item)
	}
	return items
}

func composerSlashPopupItems(m Model, command composerCommand) []composerPopupItem {
	item := composerPopupItem{
		ID:    command.ID,
		Title: command.Name,
		Meta:  command.Description,
		Value: command.Name,
	}
	switch command.ID {
	case "thinking":
		if m.thinkingEnabled {
			item.Meta = "currently requesting provider thought output · toggle off"
		} else {
			item.Meta = "currently not requesting provider thought output · toggle on"
		}
		return []composerPopupItem{item}
	default:
		return []composerPopupItem{item}
	}
}

func composerHistoryItems(query string, entries []app.PromptHistoryEntry) []composerPopupItem {
	type scoredItem struct {
		item  composerPopupItem
		score int
	}
	matches := make([]scoredItem, 0, len(entries))
	for _, entry := range entries {
		item := composerPopupItem{
			ID:    fmt.Sprintf("%s:%s", entry.SessionID, entry.TurnID),
			Title: entry.Prompt,
			Value: entry.Prompt,
		}
		if query == "" {
			matches = append(matches, scoredItem{item: item})
			continue
		}
		if ok, score := fuzzyScore(query, entry.Prompt); ok {
			matches = append(matches, scoredItem{item: item, score: score})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].score > matches[j].score
	})
	items := make([]composerPopupItem, 0, len(matches))
	for _, match := range matches {
		items = append(items, match.item)
	}
	return items
}
