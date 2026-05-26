package app

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

const sessionHistoryPageInTurnLimit = 2

var sessionHistoryPageInTurnRefPattern = regexp.MustCompile(`\bturn-[A-Za-z0-9][A-Za-z0-9_-]*\b`)

type sessionHistoryPageInIntent struct {
	Requested     bool
	ExplicitExact bool
	WantOutput    bool
	WantError     bool
	WantCommand   bool
	WantWording   bool
	WantAudit     bool
}

type sessionHistoryPageInQuery struct {
	Text   string
	Lower  string
	Intent sessionHistoryPageInIntent
}

type sessionHistoryPageInSelection struct {
	TurnID                   string
	Score                    int
	FullTurn                 bool
	IncludeUserMessage       bool
	IncludeAssistantMessages bool
	IncludeRuntimeNotes      bool
	IncludeTurnStatus        bool
	IncludeToolCalls         bool
	IncludeToolResults       bool
	IncludeExecutions        bool
	ToolNames                []string
	Paths                    []string
	CallIDs                  []string
}

const (
	sessionHistoryPageInScoreHint        = 90
	sessionHistoryPageInScoreExplicitRef = 100
)

func selectSessionHistoryPageInTurnIDs(currentInputs []provider.Input, history *sessionHistoryState) []string {
	selections := selectSessionHistoryPageInSelections(currentInputs, history)
	return pageInSelectionTurnIDs(selections)
}

func selectSessionHistoryPageInSelections(currentInputs []provider.Input, history *sessionHistoryState) []sessionHistoryPageInSelection {
	if history == nil {
		return nil
	}
	available := make(map[string]struct{}, len(history.CompletedOrder))
	positions := make(map[string]int, len(history.CompletedOrder))
	for _, turnID := range history.CompletedOrder {
		available[turnID] = struct{}{}
		positions[turnID] = len(positions)
	}

	text := sessionHistoryPageInText(currentInputs)
	query := sessionHistoryPageInQuery{
		Text:  text,
		Lower: strings.ToLower(strings.TrimSpace(text)),
	}
	query.Intent = detectSessionHistoryPageInIntent(text)

	continuation := history.Conversation.Continuation
	if continuation == nil {
		continuation = history.ExistingContinuation
	}

	selections := make(map[string]sessionHistoryPageInSelection, sessionHistoryPageInTurnLimit)
	addSessionHistoryPageInSelections(
		selections,
		available,
		extractSessionHistoryTurnRefs(query.Text),
		func(turnID string) sessionHistoryPageInSelection {
			return newSessionHistoryPageInExplicitSelection(turnID, sessionHistoryPageInScoreExplicitRef, query)
		},
	)
	if continuation != nil {
		collectSessionHistoryArtifactPageInSelections(selections, query, continuation.Artifact, available)
	}
	return rankedSessionHistoryPageInSelections(history.CompletedOrder, positions, selections, sessionHistoryPageInTurnLimit)
}

func sessionHistoryPageInText(inputs []provider.Input) string {
	if len(inputs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(inputs)*2)
	for _, input := range inputs {
		for _, value := range []string{input.Content, input.Arguments, input.Output, input.Error} {
			value = strings.TrimSpace(value)
			if value != "" {
				parts = append(parts, value)
			}
		}
		if input.AnthropicThinking != nil {
			if thinking := strings.TrimSpace(input.AnthropicThinking.Thinking); thinking != "" {
				parts = append(parts, thinking)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func detectSessionHistoryPageInIntent(text string) sessionHistoryPageInIntent {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return sessionHistoryPageInIntent{}
	}
	priorReference := pageInContainsAny(lower,
		"earlier", "previous", "prior", "before", "from above", "from earlier", "from before", "older turn", "original",
	)
	intent := sessionHistoryPageInIntent{
		ExplicitExact: pageInContainsAny(lower,
			"exact", "verbatim", "quote", "quoted", "raw", "full", "complete", "actual", "original wording",
		),
		WantOutput: pageInContainsAny(lower,
			"output", "result", "returned", "return", "stdout", "content", "contents", "read returned",
		),
		WantError: pageInContainsAny(lower,
			"error", "stderr", "failed", "failure", "traceback", "stack trace", "log", "logs",
		),
		WantCommand: pageInContainsAny(lower,
			"command", "arguments", "argument", "args", "invocation",
		),
		WantWording: pageInContainsAny(lower,
			"verbatim", "exact words", "what did i ask", "what did i say", "wording", "quote",
		),
		WantAudit: pageInContainsAny(lower,
			"audit", "re-audit", "recheck", "double-check", "verify against", "compare against",
		),
	}
	intent.Requested = intent.ExplicitExact || intent.WantWording || intent.WantAudit || ((intent.WantOutput || intent.WantError || intent.WantCommand) && priorReference)
	return intent
}

func collectSessionHistoryArtifactPageInSelections(
	selections map[string]sessionHistoryPageInSelection,
	query sessionHistoryPageInQuery,
	artifact events.HistoryContinuationArtifact,
	available map[string]struct{},
) {
	for _, hint := range artifact.PageInHints {
		if !sessionHistoryPageInHintMatchesQuery(query, hint) {
			continue
		}
		addSessionHistoryPageInSelections(
			selections,
			available,
			hint.SourceTurnIDs,
			func(turnID string) sessionHistoryPageInSelection {
				return newSessionHistoryPageInHintSelection(turnID, sessionHistoryPageInScoreHint, query, hint)
			},
		)
	}
}

func newSessionHistoryPageInSelection(turnID string, score int, query sessionHistoryPageInQuery) sessionHistoryPageInSelection {
	selection := sessionHistoryPageInSelection{
		TurnID: turnID,
		Score:  score,
	}
	applySessionHistoryPageInQuerySelection(&selection, query)
	finalizeSessionHistoryPageInSelection(&selection, query)
	return selection
}

func newSessionHistoryPageInExplicitSelection(turnID string, score int, query sessionHistoryPageInQuery) sessionHistoryPageInSelection {
	selection := newSessionHistoryPageInSelection(turnID, score, query)
	if !query.Intent.Requested {
		selection.FullTurn = true
	}
	return selection
}

func newSessionHistoryPageInHintSelection(turnID string, score int, query sessionHistoryPageInQuery, hint events.HistoryPageInHintPayload) sessionHistoryPageInSelection {
	selection := sessionHistoryPageInSelection{
		TurnID:    turnID,
		Score:     score,
		ToolNames: normalizeHistoryPageInToolNames(hint.ToolNames),
		Paths:     normalizeHistoryPageInPaths(hint.Paths),
		CallIDs:   normalizeHistoryPageInCallIDs(hint.CallIDs),
	}
	applySessionHistoryPageInHintSelection(&selection, hint)
	applySessionHistoryPageInQuerySelection(&selection, query)
	finalizeSessionHistoryPageInSelection(&selection, query)
	return selection
}

func applySessionHistoryPageInHintSelection(selection *sessionHistoryPageInSelection, hint events.HistoryPageInHintPayload) {
	if selection == nil {
		return
	}
	for _, kind := range hint.MatchKinds {
		switch strings.TrimSpace(kind) {
		case events.HistoryPageInMatchKindAudit:
			continue
		case events.HistoryPageInMatchKindUserWording:
			selection.IncludeUserMessage = true
		case events.HistoryPageInMatchKindToolOutput, events.HistoryPageInMatchKindToolError:
			selection.IncludeToolCalls = true
			selection.IncludeToolResults = true
		case events.HistoryPageInMatchKindToolCommand:
			selection.IncludeToolCalls = true
			selection.IncludeExecutions = true
		case events.HistoryPageInMatchKindPathContext:
			selection.IncludeUserMessage = true
			selection.IncludeAssistantMessages = true
		}
	}
}

func applySessionHistoryPageInQuerySelection(selection *sessionHistoryPageInSelection, query sessionHistoryPageInQuery) {
	if selection == nil {
		return
	}
	intent := query.Intent
	if intent.WantWording {
		selection.IncludeUserMessage = true
	}
	if intent.WantOutput || intent.WantError {
		selection.IncludeToolCalls = true
		selection.IncludeToolResults = true
	}
	if intent.WantCommand {
		selection.IncludeToolCalls = true
		selection.IncludeExecutions = true
	}
	if intent.WantAudit {
		selection.IncludeUserMessage = true
		selection.IncludeAssistantMessages = true
		selection.IncludeRuntimeNotes = true
		selection.IncludeTurnStatus = true
		selection.IncludeToolCalls = true
		selection.IncludeToolResults = true
		selection.IncludeExecutions = true
	}
}

func finalizeSessionHistoryPageInSelection(selection *sessionHistoryPageInSelection, query sessionHistoryPageInQuery) {
	if selection == nil {
		return
	}
	selection.ToolNames = normalizeHistoryPageInToolNames(selection.ToolNames)
	selection.Paths = normalizeHistoryPageInPaths(selection.Paths)
	selection.CallIDs = normalizeHistoryPageInCallIDs(selection.CallIDs)
	if selection.FullTurn {
		return
	}
	if !selection.hasFragments() {
		switch {
		case query.Intent.ExplicitExact:
			selection.FullTurn = true
		case !query.Intent.Requested:
			selection.IncludeUserMessage = true
			selection.IncludeAssistantMessages = true
		}
	}
}

func (selection sessionHistoryPageInSelection) hasFragments() bool {
	return selection.IncludeUserMessage ||
		selection.IncludeAssistantMessages ||
		selection.IncludeRuntimeNotes ||
		selection.IncludeTurnStatus ||
		selection.IncludeToolCalls ||
		selection.IncludeToolResults ||
		selection.IncludeExecutions
}

func (selection sessionHistoryPageInSelection) merged(other sessionHistoryPageInSelection) sessionHistoryPageInSelection {
	merged := selection
	if strings.TrimSpace(merged.TurnID) == "" {
		merged.TurnID = other.TurnID
	}
	if other.Score > merged.Score {
		merged.Score = other.Score
	}
	merged.FullTurn = merged.FullTurn || other.FullTurn
	merged.IncludeUserMessage = merged.IncludeUserMessage || other.IncludeUserMessage
	merged.IncludeAssistantMessages = merged.IncludeAssistantMessages || other.IncludeAssistantMessages
	merged.IncludeRuntimeNotes = merged.IncludeRuntimeNotes || other.IncludeRuntimeNotes
	merged.IncludeTurnStatus = merged.IncludeTurnStatus || other.IncludeTurnStatus
	merged.IncludeToolCalls = merged.IncludeToolCalls || other.IncludeToolCalls
	merged.IncludeToolResults = merged.IncludeToolResults || other.IncludeToolResults
	merged.IncludeExecutions = merged.IncludeExecutions || other.IncludeExecutions
	merged.ToolNames = appendUniqueValues(merged.ToolNames, other.ToolNames)
	merged.Paths = appendUniqueValues(merged.Paths, other.Paths)
	merged.CallIDs = appendUniqueValues(merged.CallIDs, other.CallIDs)
	return merged
}

func sessionHistoryPageInHintMatchesQuery(query sessionHistoryPageInQuery, hint events.HistoryPageInHintPayload) bool {
	if sessionHistoryPageInHintKindsMatchIntent(hint.MatchKinds, query.Intent) {
		return true
	}
	if query.Intent.Requested && sessionHistoryPageInToolMentioned(query.Lower, hint.ToolNames) {
		return true
	}
	if sessionHistoryPageInPathsMentioned(query.Lower, hint.Paths) {
		return true
	}
	return sessionHistoryPageInHintMatchesIntent(hint.When, query.Intent)
}

func sessionHistoryPageInHintKindsMatchIntent(kinds []string, intent sessionHistoryPageInIntent) bool {
	for _, kind := range kinds {
		if sessionHistoryPageInKindMatchesIntent(kind, intent) {
			return true
		}
	}
	return false
}

func sessionHistoryPageInKindMatchesIntent(kind string, intent sessionHistoryPageInIntent) bool {
	switch strings.TrimSpace(kind) {
	case events.HistoryPageInMatchKindAudit:
		return intent.WantAudit || intent.ExplicitExact
	case events.HistoryPageInMatchKindUserWording:
		return intent.WantWording
	case events.HistoryPageInMatchKindToolOutput:
		return intent.WantOutput || intent.WantAudit || intent.ExplicitExact
	case events.HistoryPageInMatchKindToolError:
		return intent.WantError || intent.WantAudit || intent.ExplicitExact
	case events.HistoryPageInMatchKindToolCommand:
		return intent.WantCommand || intent.WantAudit || intent.ExplicitExact
	case events.HistoryPageInMatchKindPathContext:
		return intent.Requested
	default:
		return false
	}
}

func sessionHistoryPageInHintMatchesIntent(when string, intent sessionHistoryPageInIntent) bool {
	if !intent.Requested {
		return false
	}
	lower := strings.ToLower(strings.TrimSpace(when))
	if lower == "" {
		return false
	}
	switch {
	case intent.WantWording:
		return pageInContainsAny(lower, "verbatim", "wording", "quote", "user wording")
	case intent.WantCommand:
		return pageInContainsAny(lower, "command", "arguments", "arg", "invocation")
	case intent.WantError:
		return pageInContainsAny(lower, "error", "stderr", "failure", "trace", "log")
	case intent.WantOutput:
		return pageInContainsAny(lower, "output", "result", "stdout", "text", "content")
	case intent.WantAudit:
		return pageInContainsAny(lower, "audit", "recheck", "verify", "exact")
	case intent.ExplicitExact:
		return true
	default:
		return false
	}
}

func sessionHistoryArtifactPathMentioned(queryLower string, artifactPath string) bool {
	artifactPath = strings.ToLower(normalizeCompactionArtifactValue(artifactPath))
	if artifactPath == "" || queryLower == "" {
		return false
	}
	if strings.Contains(queryLower, artifactPath) {
		return true
	}
	base := strings.ToLower(strings.TrimSpace(path.Base(artifactPath)))
	return len(base) >= 4 && strings.Contains(queryLower, base)
}

func sessionHistoryPageInPathsMentioned(queryLower string, paths []string) bool {
	for _, candidate := range paths {
		if sessionHistoryArtifactPathMentioned(queryLower, candidate) {
			return true
		}
	}
	return false
}

func sessionHistoryPageInToolMentioned(queryLower string, toolNames []string) bool {
	for _, candidate := range toolNames {
		candidate = strings.ToLower(strings.TrimSpace(candidate))
		if candidate != "" && strings.Contains(queryLower, candidate) {
			return true
		}
	}
	return false
}

func extractSessionHistoryTurnRefs(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return sanitizeCompactionTurnOrder(sessionHistoryPageInTurnRefPattern.FindAllString(strings.ToLower(text), -1))
}

func filterAvailableHistoryTurnIDs(turnIDs []string, available map[string]struct{}) []string {
	if len(turnIDs) == 0 || len(available) == 0 {
		return nil
	}
	out := make([]string, 0, len(turnIDs))
	for _, turnID := range sanitizeCompactionTurnOrder(turnIDs) {
		if _, ok := available[turnID]; ok {
			out = append(out, turnID)
		}
	}
	return out
}

func addSessionHistoryPageInSelections(
	selections map[string]sessionHistoryPageInSelection,
	available map[string]struct{},
	turnIDs []string,
	build func(turnID string) sessionHistoryPageInSelection,
) {
	for _, turnID := range filterAvailableHistoryTurnIDs(turnIDs, available) {
		selection := build(turnID)
		if current, ok := selections[turnID]; ok {
			selection = current.merged(selection)
		}
		selections[turnID] = selection
	}
}

func rankedSessionHistoryPageInSelections(
	order []string,
	positions map[string]int,
	selections map[string]sessionHistoryPageInSelection,
	limit int,
) []sessionHistoryPageInSelection {
	if len(selections) == 0 {
		return nil
	}
	ranked := make([]sessionHistoryPageInSelection, 0, len(selections))
	for _, selection := range selections {
		ranked = append(ranked, selection)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		left, right := ranked[i], ranked[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		return positions[left.TurnID] > positions[right.TurnID]
	})
	if limit > 0 && len(ranked) > limit {
		ranked = ranked[:limit]
	}
	rankedIDs := make([]string, 0, len(ranked))
	byID := make(map[string]sessionHistoryPageInSelection, len(ranked))
	for _, selection := range ranked {
		rankedIDs = append(rankedIDs, selection.TurnID)
		byID[selection.TurnID] = selection
	}
	orderedIDs := orderedSessionHistoryTurnIDs(order, rankedIDs, 0)
	orderedSelections := make([]sessionHistoryPageInSelection, 0, len(orderedIDs))
	for _, turnID := range orderedIDs {
		orderedSelections = append(orderedSelections, byID[turnID])
	}
	return orderedSelections
}

func pageInSelectionTurnIDs(selections []sessionHistoryPageInSelection) []string {
	if len(selections) == 0 {
		return nil
	}
	turnIDs := make([]string, 0, len(selections))
	for _, selection := range selections {
		if turnID := strings.TrimSpace(selection.TurnID); turnID != "" {
			turnIDs = append(turnIDs, turnID)
		}
	}
	return sanitizeCompactionTurnOrder(turnIDs)
}

func orderedSessionHistoryTurnIDs(order []string, turnIDs []string, limit int) []string {
	if len(order) == 0 || len(turnIDs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(turnIDs))
	for _, turnID := range sanitizeCompactionTurnOrder(turnIDs) {
		seen[turnID] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]string, 0, min(len(seen), max(limit, 1)))
	for _, turnID := range order {
		if _, ok := seen[turnID]; !ok {
			continue
		}
		out = append(out, turnID)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func pageInContainsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}
