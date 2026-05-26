package app

import (
	"fmt"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func cloneHistoryCompactionArtifact(artifact events.HistoryContinuationArtifact) events.HistoryContinuationArtifact {
	return events.HistoryContinuationArtifact{
		SessionObjective:  strings.TrimSpace(artifact.SessionObjective),
		Constraints:       append([]string(nil), artifact.Constraints...),
		SettledDecisions:  cloneHistoryDecisionPayloads(artifact.SettledDecisions),
		CompletedEpisodes: cloneHistoryEpisodePayloads(artifact.CompletedEpisodes),
		OpenThreads:       cloneHistoryOpenThreadPayloads(artifact.OpenThreads),
		WorkspaceFacts:    cloneHistoryWorkspaceFactPayloads(artifact.WorkspaceFacts),
		PageInHints:       cloneHistoryPageInHintPayloads(artifact.PageInHints),
	}
}

func cloneHistoryDecisionPayloads(values []events.HistoryDecisionPayload) []events.HistoryDecisionPayload {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]events.HistoryDecisionPayload(nil), values...)
	return cloned
}

func cloneHistoryEpisodePayloads(values []events.HistoryEpisodePayload) []events.HistoryEpisodePayload {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]events.HistoryEpisodePayload, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, events.HistoryEpisodePayload{
			EpisodeID:     value.EpisodeID,
			Summary:       value.Summary,
			TouchedPaths:  append([]string(nil), value.TouchedPaths...),
			Verification:  cloneHistoryVerificationPayloads(value.Verification),
			SourceTurnIDs: append([]string(nil), value.SourceTurnIDs...),
		})
	}
	return cloned
}

func cloneHistoryVerificationPayloads(values []events.HistoryVerificationPayload) []events.HistoryVerificationPayload {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]events.HistoryVerificationPayload(nil), values...)
	return cloned
}

func cloneHistoryOpenThreadPayloads(values []events.HistoryOpenThreadPayload) []events.HistoryOpenThreadPayload {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]events.HistoryOpenThreadPayload(nil), values...)
	return cloned
}

func cloneHistoryWorkspaceFactPayloads(values []events.HistoryWorkspaceFactPayload) []events.HistoryWorkspaceFactPayload {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]events.HistoryWorkspaceFactPayload(nil), values...)
	return cloned
}

func cloneHistoryPageInHintPayloads(values []events.HistoryPageInHintPayload) []events.HistoryPageInHintPayload {
	if len(values) == 0 {
		return nil
	}
	cloned := make([]events.HistoryPageInHintPayload, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, events.HistoryPageInHintPayload{
			When:          value.When,
			MatchKinds:    append([]string(nil), value.MatchKinds...),
			ToolNames:     append([]string(nil), value.ToolNames...),
			Paths:         append([]string(nil), value.Paths...),
			CallIDs:       append([]string(nil), value.CallIDs...),
			SourceTurnIDs: append([]string(nil), value.SourceTurnIDs...),
		})
	}
	return cloned
}

func normalizeHistoryCompactionArtifact(artifact events.HistoryContinuationArtifact) events.HistoryContinuationArtifact {
	artifact.SessionObjective = strings.TrimSpace(artifact.SessionObjective)
	artifact.Constraints = appendUniqueValues(nil, artifact.Constraints)
	artifact.SettledDecisions = normalizeHistoryDecisionPayloads(artifact.SettledDecisions)
	artifact.CompletedEpisodes = normalizeHistoryEpisodePayloads(artifact.CompletedEpisodes)
	artifact.OpenThreads = normalizeHistoryOpenThreadPayloads(artifact.OpenThreads)
	artifact.WorkspaceFacts = normalizeHistoryWorkspaceFactPayloads(artifact.WorkspaceFacts)
	artifact.PageInHints = normalizeHistoryPageInHintPayloads(artifact.PageInHints)
	return artifact
}

func normalizeHistoryDecisionPayloads(values []events.HistoryDecisionPayload) []events.HistoryDecisionPayload {
	if len(values) == 0 {
		return nil
	}
	out := make([]events.HistoryDecisionPayload, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Decision = compactOutcomeSingleLine(value.Decision, compactionTurnAssistantMaxBytes)
		value.Rationale = compactOutcomeSingleLine(value.Rationale, compactionTurnAssistantMaxBytes)
		value.Status = strings.TrimSpace(value.Status)
		value.SourceTurnID = strings.TrimSpace(value.SourceTurnID)
		if value.Decision == "" || value.SourceTurnID == "" || value.Status == "" {
			continue
		}
		key := value.Status + "\x00" + value.SourceTurnID + "\x00" + value.Decision
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryEpisodePayloads(values []events.HistoryEpisodePayload) []events.HistoryEpisodePayload {
	if len(values) == 0 {
		return nil
	}
	out := make([]events.HistoryEpisodePayload, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.EpisodeID = strings.TrimSpace(value.EpisodeID)
		value.Summary = compactOutcomeSingleLine(value.Summary, compactionTurnAssistantMaxBytes)
		value.TouchedPaths = appendUniqueValues(nil, value.TouchedPaths)
		value.Verification = normalizeHistoryVerificationPayloads(value.Verification)
		value.SourceTurnIDs = sanitizeCompactionTurnOrder(value.SourceTurnIDs)
		if value.EpisodeID == "" || value.Summary == "" || len(value.SourceTurnIDs) == 0 {
			continue
		}
		if _, ok := seen[value.EpisodeID]; ok {
			continue
		}
		seen[value.EpisodeID] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryVerificationPayloads(values []events.HistoryVerificationPayload) []events.HistoryVerificationPayload {
	if len(values) == 0 {
		return nil
	}
	out := make([]events.HistoryVerificationPayload, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Kind = strings.TrimSpace(value.Kind)
		value.Value = compactOutcomeSingleLine(value.Value, compactionTurnAssistantMaxBytes)
		if value.Kind == "" || value.Value == "" {
			continue
		}
		key := value.Kind + "\x00" + value.Value + "\x00" + fmt.Sprintf("%t", value.Succeeded)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryOpenThreadPayloads(values []events.HistoryOpenThreadPayload) []events.HistoryOpenThreadPayload {
	if len(values) == 0 {
		return nil
	}
	out := make([]events.HistoryOpenThreadPayload, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Item = compactOutcomeSingleLine(value.Item, compactionTurnAssistantMaxBytes)
		value.Status = strings.TrimSpace(value.Status)
		value.Owner = strings.TrimSpace(value.Owner)
		value.SourceTurnID = strings.TrimSpace(value.SourceTurnID)
		if value.Item == "" || value.Status == "" || value.SourceTurnID == "" {
			continue
		}
		key := value.Status + "\x00" + value.SourceTurnID + "\x00" + value.Item
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryWorkspaceFactPayloads(values []events.HistoryWorkspaceFactPayload) []events.HistoryWorkspaceFactPayload {
	if len(values) == 0 {
		return nil
	}
	out := make([]events.HistoryWorkspaceFactPayload, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.Path = normalizeCompactionArtifactValue(value.Path)
		value.Fact = compactOutcomeSingleLine(value.Fact, compactionTurnAssistantMaxBytes)
		value.SourceTurnID = strings.TrimSpace(value.SourceTurnID)
		if value.Path == "" || value.Fact == "" || value.SourceTurnID == "" {
			continue
		}
		key := value.Path + "\x00" + value.SourceTurnID + "\x00" + value.Fact
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryPageInHintPayloads(values []events.HistoryPageInHintPayload) []events.HistoryPageInHintPayload {
	if len(values) == 0 {
		return nil
	}
	out := make([]events.HistoryPageInHintPayload, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value.When = compactOutcomeSingleLine(value.When, compactionTurnAssistantMaxBytes)
		value.MatchKinds = normalizeHistoryPageInMatchKinds(value.MatchKinds)
		value.ToolNames = normalizeHistoryPageInToolNames(value.ToolNames)
		value.Paths = normalizeHistoryPageInPaths(value.Paths)
		value.CallIDs = normalizeHistoryPageInCallIDs(value.CallIDs)
		value.SourceTurnIDs = sanitizeCompactionTurnOrder(value.SourceTurnIDs)
		if value.When == "" || len(value.SourceTurnIDs) == 0 {
			continue
		}
		key := strings.Join(value.MatchKinds, ",") +
			"\x00" + strings.Join(value.ToolNames, ",") +
			"\x00" + strings.Join(value.Paths, ",") +
			"\x00" + strings.Join(value.CallIDs, ",") +
			"\x00" + value.When +
			"\x00" + strings.Join(value.SourceTurnIDs, ",")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryPageInMatchKinds(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		switch value {
		case events.HistoryPageInMatchKindAudit,
			events.HistoryPageInMatchKindUserWording,
			events.HistoryPageInMatchKindToolOutput,
			events.HistoryPageInMatchKindToolError,
			events.HistoryPageInMatchKindToolCommand,
			events.HistoryPageInMatchKindPathContext:
		default:
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryPageInToolNames(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryPageInPaths(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = normalizeCompactionArtifactValue(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func normalizeHistoryPageInCallIDs(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func buildSessionCompactionArtifact(existing *events.SessionHistoryContinuationUpdatedPayload, newTurnIDs []string, turns map[string]*replayedSessionTurn) events.HistoryContinuationArtifact {
	artifact := events.HistoryContinuationArtifact{}
	if existing != nil {
		artifact = cloneHistoryCompactionArtifact(existing.Artifact)
	}
	for _, turnID := range newTurnIDs {
		turn := turns[turnID]
		if turn == nil {
			continue
		}
		if objective := compactionGoalCandidate(turn.UserText); objective != "" {
			artifact.SessionObjective = objective
		}
		artifact.Constraints = appendUniqueValues(artifact.Constraints, extractCompactionConstraintCandidates(turn.AssistantText))
		if decision := sessionCompactionDecisionCandidate(turn.AssistantText); decision != "" {
			artifact.SettledDecisions = appendHistoryDecisionPayload(artifact.SettledDecisions, events.HistoryDecisionPayload{
				Decision:     decision,
				Status:       events.HistoryDecisionStatusActive,
				SourceTurnID: turnID,
			})
		}
		episode := buildHistoryContinuationEpisode(turnID, turn)
		if episode.EpisodeID != "" {
			artifact.CompletedEpisodes = appendHistoryEpisodePayload(artifact.CompletedEpisodes, episode)
			for _, fact := range buildHistoryWorkspaceFacts(episode) {
				artifact.WorkspaceFacts = appendHistoryWorkspaceFactPayload(artifact.WorkspaceFacts, fact)
			}
		}
		for _, thread := range buildHistoryOpenThreads(turnID, turn) {
			artifact.OpenThreads = appendHistoryOpenThreadPayload(artifact.OpenThreads, thread)
		}
	}
	return normalizeHistoryCompactionArtifact(artifact)
}

func extractCompactionConstraintCandidates(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(text), "\n")
	out := make([]string, 0, min(len(lines), compactionSummaryConstraintLimit))
	for _, line := range lines {
		kind, value, ok := parseCompactionSummaryLine(line)
		if !ok || kind != "constraint" {
			continue
		}
		normalized := normalizeCompactionSummaryValue(value, compactionTurnAssistantMaxBytes)
		if normalized == "" {
			continue
		}
		out = appendCompactionSummaryValues(out, normalized, compactionSummaryConstraintLimit)
	}
	return out
}

func sessionCompactionDecisionCandidate(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "Decision:"):
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "Decision:"))
		case strings.HasPrefix(trimmed, "Decisions:"):
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "Decisions:"))
		}
		if kind, value, ok := parseCompactionSummaryLine(trimmed); ok && kind == "decision" {
			return normalizeCompactionSummaryValue(value, compactionTurnAssistantMaxBytes)
		}
	}
	return ""
}

func buildHistoryContinuationEpisode(turnID string, turn *replayedSessionTurn) events.HistoryEpisodePayload {
	if turn == nil {
		return events.HistoryEpisodePayload{}
	}
	summaryParts := make([]string, 0, compactionTurnAssistantLineLimit)
	states, _ := extractCompactionAssistantFacts(turn.AssistantText)
	summaryParts = appendBoundedOutcomeValues(summaryParts, states, compactionSummaryDoneLimit, compactionTurnAssistantMaxBytes)
	for _, note := range compactRuntimeNotes(turn.RuntimeNotes) {
		if isCompactionBlockedText(note) {
			continue
		}
		summaryParts = appendBoundedOutcomeValues(summaryParts, []string{note}, compactionSummaryDoneLimit, compactionTurnAssistantMaxBytes)
	}
	if len(summaryParts) == 0 && turn.SuccessfulToolCalls > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d tool calls succeeded", turn.SuccessfulToolCalls))
	}
	if len(summaryParts) == 0 {
		if status := renderCompactionTurnStatus(turn); status != "" && status != "completed" {
			summaryParts = append(summaryParts, status)
		}
	}
	if len(summaryParts) == 0 && len(turn.WorkspacePaths) > 0 {
		summaryParts = append(summaryParts, "Touched "+strings.Join(appendBoundedOutcomeValues(nil, turn.WorkspacePaths, compactionSummaryFileLimit, compactionSummaryWorkspacePathBytes), ", "))
	}
	if len(summaryParts) == 0 {
		return events.HistoryEpisodePayload{}
	}
	episode := events.HistoryEpisodePayload{
		EpisodeID:     historyContinuationEpisodeID([]string{turnID}),
		Summary:       strings.Join(summaryParts, "; "),
		TouchedPaths:  appendUniqueValues(nil, turn.WorkspacePaths),
		SourceTurnIDs: []string{turnID},
	}
	episode.Verification = buildHistoryEpisodeVerification(turn)
	return episode
}

func buildHistoryEpisodeVerification(turn *replayedSessionTurn) []events.HistoryVerificationPayload {
	if turn == nil {
		return nil
	}
	var verification []events.HistoryVerificationPayload
	if turn.SuccessfulToolCalls > 0 {
		verification = append(verification, events.HistoryVerificationPayload{
			Kind:      events.HistoryVerificationKindToolResult,
			Value:     fmt.Sprintf("%d tool calls succeeded", turn.SuccessfulToolCalls),
			Succeeded: true,
		})
	}
	if turn.FailedToolCalls > 0 {
		verification = append(verification, events.HistoryVerificationPayload{
			Kind:      events.HistoryVerificationKindToolResult,
			Value:     appendArtifactFailureSummary(turn.FailedToolCalls, turn.FailedToolNames),
			Succeeded: false,
		})
	}
	if status := renderCompactionTurnStatus(turn); status != "" && status != "completed" {
		verification = append(verification, events.HistoryVerificationPayload{
			Kind:      events.HistoryVerificationKindTurnStatus,
			Value:     status,
			Succeeded: !strings.HasPrefix(strings.ToLower(status), "failed"),
		})
	}
	for _, note := range compactRuntimeNotes(turn.postTerminalRuntimeNotes()) {
		verification = append(verification, events.HistoryVerificationPayload{
			Kind:      events.HistoryVerificationKindRuntimeNote,
			Value:     note,
			Succeeded: !isCompactionBlockedText(note),
		})
	}
	return normalizeHistoryVerificationPayloads(verification)
}

func compactRuntimeNotes(notes []replayedSessionRuntimeNote) []string {
	if len(notes) == 0 {
		return nil
	}
	values := make([]string, 0, min(len(notes), compactionTurnRuntimeNoteLimit))
	start := max(len(notes)-compactionTurnRuntimeNoteLimit, 0)
	for _, note := range notes[start:] {
		values = append(values, note.Content)
	}
	return appendBoundedOutcomeValues(nil, values, compactionTurnRuntimeNoteLimit, compactionTurnRuntimeNoteMaxBytes)
}

func buildHistoryOpenThreads(turnID string, turn *replayedSessionTurn) []events.HistoryOpenThreadPayload {
	if turn == nil {
		return nil
	}
	var threads []events.HistoryOpenThreadPayload
	_, next := extractCompactionAssistantFacts(turn.AssistantText)
	if next != "" {
		threads = append(threads, buildHistoryOpenThread(turnID, next, false))
	}
	for _, note := range compactRuntimeNotes(turn.RuntimeNotes) {
		if !isCompactionBlockedText(note) {
			continue
		}
		threads = append(threads, buildHistoryOpenThread(turnID, note, true))
	}
	if turn.FailedToolCalls > 0 {
		threads = append(threads, buildHistoryOpenThread(turnID, appendArtifactFailureSummary(turn.FailedToolCalls, turn.FailedToolNames), true))
	}
	if status := renderCompactionTurnStatus(turn); strings.HasPrefix(strings.ToLower(status), "failed") {
		threads = append(threads, buildHistoryOpenThread(turnID, status, true))
	}
	return normalizeHistoryOpenThreadPayloads(threads)
}

func buildHistoryOpenThread(turnID string, item string, blocking bool) events.HistoryOpenThreadPayload {
	status := events.HistoryOpenThreadStatusPending
	if blocking {
		status = events.HistoryOpenThreadStatusBlocked
	}
	owner := events.HistoryOpenThreadOwnerAgent
	if isPendingDecisionText(item) {
		owner = events.HistoryOpenThreadOwnerUser
	}
	return events.HistoryOpenThreadPayload{
		Item:         item,
		Status:       status,
		Blocking:     blocking,
		Owner:        owner,
		SourceTurnID: turnID,
	}
}

func buildHistoryWorkspaceFacts(episode events.HistoryEpisodePayload) []events.HistoryWorkspaceFactPayload {
	if len(episode.TouchedPaths) == 0 {
		return nil
	}
	sourceTurnID := historyContinuationWorkspaceFactSourceTurnID(episode.SourceTurnIDs)
	if sourceTurnID == "" {
		return nil
	}
	fact := compactOutcomeSingleLine(episode.Summary, compactionTurnAssistantMaxBytes)
	if fact == "" {
		fact = "Touched during consolidated work"
	}
	out := make([]events.HistoryWorkspaceFactPayload, 0, len(episode.TouchedPaths))
	for _, path := range episode.TouchedPaths {
		out = append(out, events.HistoryWorkspaceFactPayload{
			Path:         path,
			Fact:         fact,
			SourceTurnID: sourceTurnID,
		})
	}
	return out
}

func historyContinuationWorkspaceFactSourceTurnID(sourceTurnIDs []string) string {
	sourceTurnIDs = sanitizeCompactionTurnOrder(sourceTurnIDs)
	if len(sourceTurnIDs) == 0 {
		return ""
	}
	return sourceTurnIDs[len(sourceTurnIDs)-1]
}

func appendHistoryDecisionPayload(existing []events.HistoryDecisionPayload, value events.HistoryDecisionPayload) []events.HistoryDecisionPayload {
	return normalizeHistoryDecisionPayloads(append(existing, value))
}

func appendHistoryEpisodePayload(existing []events.HistoryEpisodePayload, value events.HistoryEpisodePayload) []events.HistoryEpisodePayload {
	filtered := existing[:0:0]
	for _, current := range existing {
		if strings.TrimSpace(current.EpisodeID) == strings.TrimSpace(value.EpisodeID) {
			continue
		}
		filtered = append(filtered, current)
	}
	return normalizeHistoryEpisodePayloads(append(filtered, value))
}

func appendHistoryOpenThreadPayload(existing []events.HistoryOpenThreadPayload, value events.HistoryOpenThreadPayload) []events.HistoryOpenThreadPayload {
	return normalizeHistoryOpenThreadPayloads(append(existing, value))
}

func appendHistoryWorkspaceFactPayload(existing []events.HistoryWorkspaceFactPayload, value events.HistoryWorkspaceFactPayload) []events.HistoryWorkspaceFactPayload {
	return normalizeHistoryWorkspaceFactPayloads(append(existing, value))
}

func historyContinuationEpisodeID(sourceTurnIDs []string) string {
	sourceTurnIDs = sanitizeCompactionTurnOrder(sourceTurnIDs)
	if len(sourceTurnIDs) == 0 {
		return ""
	}
	if len(sourceTurnIDs) == 1 {
		return "episode:" + sourceTurnIDs[0]
	}
	return "episode:" + strings.Join(sourceTurnIDs, "+")
}

func renderSessionCompactionArtifactSummary(artifact events.HistoryContinuationArtifact, maxBytes int) string {
	header := historyContinuationSummaryHeader
	if maxBytes <= len(header) {
		return truncateUTF8Bytes(header, maxBytes)
	}
	artifact = normalizeHistoryCompactionArtifact(artifact)
	blocks := []string{header}
	remaining := maxBytes - len(header)
	for _, block := range []string{
		renderHistoryCompactionSection("Goal", summaryValuesOrNone(singleValueSlice(artifact.SessionObjective))),
		renderHistoryCompactionSection("Constraints & Preferences", summaryValuesOrNone(artifact.Constraints)),
		renderHistoryProgressSection(artifact),
		renderHistoryCompactionSection("Key Decisions", renderHistoryDecisionLines(artifact.SettledDecisions)),
		renderHistoryCompactionSection("Next Steps", renderHistoryNextStepLines(artifact.OpenThreads)),
		renderHistoryCompactionSection("Critical Context", renderHistoryCriticalContextLines(artifact)),
		renderHistoryCompactionSection("Relevant Files", renderHistoryRelevantFileLines(artifact)),
	} {
		if !appendCompactionBlock(&blocks, block, &remaining) {
			break
		}
	}
	return strings.Join(blocks, "\n")
}

func renderHistoryCompactionSection(title string, values []string) string {
	lines := make([]string, 0, len(values)+1)
	lines = append(lines, "## "+title)
	for _, value := range summaryValuesOrNone(values) {
		lines = append(lines, "- "+value)
	}
	return strings.Join(lines, "\n")
}

func renderHistoryProgressSection(artifact events.HistoryContinuationArtifact) string {
	lines := []string{"## Progress"}
	lines = append(lines, "### Done")
	for _, value := range summaryValuesOrNone(renderHistoryEpisodeLines(artifact.CompletedEpisodes)) {
		lines = append(lines, "- "+value)
	}
	lines = append(lines, "### In Progress")
	for _, value := range summaryValuesOrNone(renderHistoryInProgressLines(artifact.OpenThreads)) {
		lines = append(lines, "- "+value)
	}
	lines = append(lines, "### Blocked")
	for _, value := range summaryValuesOrNone(renderHistoryBlockedLines(artifact.OpenThreads)) {
		lines = append(lines, "- "+value)
	}
	return strings.Join(lines, "\n")
}

func singleValueSlice(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func renderHistoryDecisionLines(values []events.HistoryDecisionPayload) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		line := value.Decision
		if value.Rationale != "" {
			line += " - " + value.Rationale
		}
		if value.Status == events.HistoryDecisionStatusSuperseded {
			line = "[superseded] " + line
		}
		out = append(out, compactOutcomeSingleLine(line, 320))
	}
	return out
}

func renderHistoryEpisodeLines(values []events.HistoryEpisodePayload) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		line := value.Summary
		out = append(out, compactOutcomeSingleLine(line, 360))
	}
	return out
}

func renderHistoryVerificationSummary(values []events.HistoryVerificationPayload) string {
	if len(values) == 0 {
		return ""
	}
	summaries := make([]string, 0, len(values))
	for _, value := range values {
		prefix := value.Kind
		if value.Succeeded {
			prefix += ":ok"
		} else {
			prefix += ":fail"
		}
		summaries = append(summaries, prefix+"="+value.Value)
	}
	return compactOutcomeSingleLine(strings.Join(summaries, "; "), 240)
}

func renderHistoryInProgressLines(values []events.HistoryOpenThreadPayload) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if historyOpenThreadBlocked(value) {
			continue
		}
		line := value.Item
		if value.Owner == events.HistoryOpenThreadOwnerUser {
			line += " (waiting on user)"
		}
		out = append(out, compactOutcomeSingleLine(line, 320))
	}
	return out
}

func renderHistoryBlockedLines(values []events.HistoryOpenThreadPayload) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !historyOpenThreadBlocked(value) {
			continue
		}
		line := value.Item
		if value.Owner == events.HistoryOpenThreadOwnerUser {
			line += " (waiting on user)"
		}
		out = append(out, compactOutcomeSingleLine(line, 320))
	}
	return out
}

func renderHistoryNextStepLines(values []events.HistoryOpenThreadPayload) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value.Status == events.HistoryOpenThreadStatusBlocked || value.Blocking {
			continue
		}
		line := value.Item
		if value.Owner == events.HistoryOpenThreadOwnerUser {
			line += " (needs user input)"
		}
		out = append(out, compactOutcomeSingleLine(line, 320))
	}
	return out
}

func historyOpenThreadBlocked(value events.HistoryOpenThreadPayload) bool {
	return value.Blocking || value.Status == events.HistoryOpenThreadStatusBlocked
}

func renderHistoryCriticalContextLines(artifact events.HistoryContinuationArtifact) []string {
	out := make([]string, 0, len(artifact.WorkspaceFacts)+len(artifact.CompletedEpisodes))
	for _, fact := range artifact.WorkspaceFacts {
		line := fact.Fact
		if fact.Path != "" {
			line = fact.Path + ": " + line
		}
		out = append(out, compactOutcomeSingleLine(line, 320))
	}
	for _, episode := range artifact.CompletedEpisodes {
		if verification := renderHistoryVerificationSummary(episode.Verification); verification != "" {
			out = append(out, compactOutcomeSingleLine(verification, 240))
		}
	}
	return out
}

func renderHistoryRelevantFileLines(artifact events.HistoryContinuationArtifact) []string {
	type fileLine struct {
		path string
		text string
	}
	seen := make(map[string]struct{})
	lines := make([]fileLine, 0, compactionSummaryWorkspacePathLimit)
	appendLine := func(path, text string) {
		path = strings.TrimSpace(path)
		text = strings.TrimSpace(text)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		if text == "" {
			text = "touched during compacted work"
		}
		lines = append(lines, fileLine{
			path: path,
			text: compactOutcomeSingleLine(text, compactionSummaryWorkspacePathBytes),
		})
	}
	for _, fact := range artifact.WorkspaceFacts {
		appendLine(fact.Path, fact.Fact)
	}
	for _, episode := range artifact.CompletedEpisodes {
		for _, path := range episode.TouchedPaths {
			appendLine(path, "touched during compacted work")
		}
	}
	out := make([]string, 0, min(len(lines), compactionSummaryWorkspacePathLimit))
	for _, line := range lines {
		if len(out) >= compactionSummaryWorkspacePathLimit {
			break
		}
		out = append(out, line.path+": "+line.text)
	}
	return out
}

func appendRuntimeNoteToCompactionArtifact(compaction *events.SessionHistoryContinuationUpdatedPayload, turnID string, note string, budgetBytes int) *events.SessionHistoryContinuationUpdatedPayload {
	if compaction == nil || strings.TrimSpace(note) == "" {
		return compaction
	}
	copyPayload := cloneCompactionPayload(compaction)
	copyPayload.Artifact = cloneHistoryCompactionArtifact(copyPayload.Artifact)
	appendRuntimeNoteToArtifact(&copyPayload.Artifact, turnID, note)
	copyPayload.Artifact = normalizeHistoryCompactionArtifact(copyPayload.Artifact)
	copyPayload.RenderedSummary = renderSessionCompactionArtifactSummary(copyPayload.Artifact, budgetBytes)
	return copyPayload
}

func appendRuntimeNoteToArtifact(artifact *events.HistoryContinuationArtifact, turnID string, note string) {
	if artifact == nil {
		return
	}
	turnID = strings.TrimSpace(turnID)
	note = strings.TrimSpace(note)
	if turnID == "" || note == "" {
		return
	}
	if isCompactionBlockedText(note) {
		artifact.OpenThreads = appendHistoryOpenThreadPayload(artifact.OpenThreads, buildHistoryOpenThread(turnID, note, true))
		return
	}
	for index := range artifact.CompletedEpisodes {
		episode := &artifact.CompletedEpisodes[index]
		if !containsCompactionValue(episode.SourceTurnIDs, turnID) {
			continue
		}
		episode.Verification = normalizeHistoryVerificationPayloads(append(episode.Verification, events.HistoryVerificationPayload{
			Kind:      events.HistoryVerificationKindRuntimeNote,
			Value:     note,
			Succeeded: true,
		}))
		if episode.Summary == "" {
			episode.Summary = note
		}
		return
	}
}

func sameHistoryCompactionArtifact(a, b events.HistoryContinuationArtifact) bool {
	left := normalizeHistoryCompactionArtifact(a)
	right := normalizeHistoryCompactionArtifact(b)
	return left.SessionObjective == right.SessionObjective &&
		sameNormalizedStringSet(left.Constraints, right.Constraints) &&
		sameHistoryDecisionPayloads(left.SettledDecisions, right.SettledDecisions) &&
		sameHistoryEpisodePayloads(left.CompletedEpisodes, right.CompletedEpisodes) &&
		sameHistoryOpenThreadPayloads(left.OpenThreads, right.OpenThreads) &&
		sameHistoryWorkspaceFactPayloads(left.WorkspaceFacts, right.WorkspaceFacts) &&
		sameHistoryPageInHintPayloads(left.PageInHints, right.PageInHints)
}

func sameHistoryDecisionPayloads(a, b []events.HistoryDecisionPayload) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameHistoryEpisodePayloads(a, b []events.HistoryEpisodePayload) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].EpisodeID != b[i].EpisodeID ||
			a[i].Summary != b[i].Summary ||
			!sameNormalizedStringSet(a[i].TouchedPaths, b[i].TouchedPaths) ||
			!sameHistoryVerificationPayloads(a[i].Verification, b[i].Verification) ||
			!sameNormalizedStringSet(a[i].SourceTurnIDs, b[i].SourceTurnIDs) {
			return false
		}
	}
	return true
}

func sameHistoryVerificationPayloads(a, b []events.HistoryVerificationPayload) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameHistoryOpenThreadPayloads(a, b []events.HistoryOpenThreadPayload) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameHistoryWorkspaceFactPayloads(a, b []events.HistoryWorkspaceFactPayload) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameHistoryPageInHintPayloads(a, b []events.HistoryPageInHintPayload) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].When != b[i].When ||
			!sameNormalizedStringSet(a[i].MatchKinds, b[i].MatchKinds) ||
			!sameNormalizedStringSet(a[i].ToolNames, b[i].ToolNames) ||
			!sameNormalizedStringSet(a[i].Paths, b[i].Paths) ||
			!sameNormalizedStringSet(a[i].CallIDs, b[i].CallIDs) ||
			!sameNormalizedStringSet(a[i].SourceTurnIDs, b[i].SourceTurnIDs) {
			return false
		}
	}
	return true
}

func historyContinuationArtifactEmpty(artifact events.HistoryContinuationArtifact) bool {
	artifact = normalizeHistoryCompactionArtifact(artifact)
	return strings.TrimSpace(artifact.SessionObjective) == "" &&
		len(artifact.Constraints) == 0 &&
		len(artifact.SettledDecisions) == 0 &&
		len(artifact.CompletedEpisodes) == 0 &&
		len(artifact.OpenThreads) == 0 &&
		len(artifact.WorkspaceFacts) == 0 &&
		len(artifact.PageInHints) == 0
}

func historyContinuationArtifactPaths(artifact events.HistoryContinuationArtifact) []string {
	artifact = normalizeHistoryCompactionArtifact(artifact)
	paths := make([]string, 0, len(artifact.WorkspaceFacts))
	for _, fact := range artifact.WorkspaceFacts {
		paths = appendUniqueValues(paths, []string{fact.Path})
	}
	for _, episode := range artifact.CompletedEpisodes {
		paths = appendUniqueValues(paths, episode.TouchedPaths)
	}
	return paths
}
