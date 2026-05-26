package app

import (
	"errors"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/agent"
	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/prompt"
)

var (
	ErrHandoffSourceMissing = errors.New("required handoff source is missing")
	ErrHandoffSourceInvalid = errors.New("handoff source is invalid")
)

const maxSourceHandoffResultChars = 8000

func (r *Runtime) resolveDelegateSourceHandoffIDs(state events.SessionState, parentTurnID string, child agent.Definition, explicit []string) ([]string, error) {
	explicit = compactHandoffIDs(explicit)
	if len(explicit) > 0 {
		return r.validateExplicitSourceHandoffIDs(state, parentTurnID, child, explicit)
	}
	return r.resolveImplicitSourceHandoffIDs(state, parentTurnID, child)
}

func (r *Runtime) validateExplicitSourceHandoffIDs(state events.SessionState, parentTurnID string, child agent.Definition, ids []string) ([]string, error) {
	turn := state.Turns[parentTurnID]
	if turn == nil {
		return nil, ErrParentTurnNotFound
	}
	for _, id := range ids {
		handoff := turn.Handoffs[id]
		if handoff == nil || handoff.Status != events.AgentResultStatusCompleted {
			return nil, ErrHandoffSourceInvalid
		}
		if len(child.Handoff.Consumes) > 0 && !handoffMatchesAnyConsume(handoff, child.Handoff.Consumes) {
			return nil, ErrHandoffSourceInvalid
		}
	}
	return ids, nil
}

func (r *Runtime) resolveImplicitSourceHandoffIDs(state events.SessionState, parentTurnID string, child agent.Definition) ([]string, error) {
	turn := state.Turns[parentTurnID]
	if turn == nil {
		return nil, ErrParentTurnNotFound
	}
	out := []string{}
	for _, consume := range child.Handoff.Consumes {
		kind := strings.TrimSpace(consume.Kind)
		if kind == "" {
			continue
		}
		from := strings.TrimSpace(consume.From)
		if from == "" {
			from = "latest"
		}
		if from != "latest" {
			return nil, ErrHandoffSourceInvalid
		}
		maxSources := consume.MaxSources
		if maxSources <= 0 {
			maxSources = 1
		}
		found := 0
		for idx := len(turn.HandoffOrder) - 1; idx >= 0 && found < maxSources; idx-- {
			handoff := turn.Handoffs[turn.HandoffOrder[idx]]
			if handoff == nil || handoff.Status != events.AgentResultStatusCompleted {
				continue
			}
			if !handoffProvidesKind(handoff, kind) {
				continue
			}
			if !containsHandoffString(out, handoff.HandoffID) {
				out = append(out, handoff.HandoffID)
				found++
			}
		}
		if found == 0 && (consume.Required || strings.TrimSpace(consume.Missing) == "reject") {
			return nil, ErrHandoffSourceMissing
		}
	}
	return out, nil
}

func handoffMatchesAnyConsume(handoff *events.AgentHandoffState, consumes []agent.HandoffConsume) bool {
	if handoff == nil {
		return false
	}
	for _, consume := range consumes {
		if handoffProvidesKind(handoff, consume.Kind) {
			return true
		}
	}
	return false
}

func handoffProvidesKind(handoff *events.AgentHandoffState, kind string) bool {
	kind = strings.TrimSpace(kind)
	if handoff == nil || kind == "" {
		return false
	}
	for _, provided := range handoff.ProvidedKinds {
		if strings.TrimSpace(provided) == kind {
			return true
		}
	}
	return false
}

func handoffProvidedKinds(definition agent.Definition) []string {
	out := make([]string, 0, len(definition.Handoff.Provides))
	for _, provide := range definition.Handoff.Provides {
		kind := strings.TrimSpace(provide.Kind)
		if kind == "" || containsHandoffString(out, kind) {
			continue
		}
		out = append(out, kind)
	}
	return out
}

func compactHandoffIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || containsHandoffString(out, id) {
			continue
		}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func containsHandoffString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func handoffSourceFragments(state events.SessionState, ids []string, childAgentID string) []prompt.Fragment {
	ids = compactHandoffIDs(ids)
	if len(ids) == 0 {
		return nil
	}
	turnsByHandoff := make(map[string]*events.AgentHandoffState)
	for _, turnID := range state.TurnOrder {
		turn := state.Turns[turnID]
		if turn == nil {
			continue
		}
		for _, id := range ids {
			if handoff := turn.Handoffs[id]; handoff != nil {
				turnsByHandoff[id] = handoff
			}
		}
	}
	lines := []string{
		"Source handoff results supplied by the runtime.",
		"Use these delegated results as source context; do not re-run the same investigation unless runtime reports stale resources, a mutation fails, or the user explicitly asks to recheck.",
	}
	if strings.TrimSpace(childAgentID) == "planner" && sourceHandoffsProvideKind(turnsByHandoff, ids, "review_findings") {
		lines = append(lines,
			"",
			"Planner guidance for review-source handoffs:",
			"- Treat the structured review artifact as the primary evidence.",
			"- Do not repeat broad discovery already done by the reviewer.",
			"- Use tools only to resolve planning-specific uncertainty: dependencies, sequencing, exact verification commands, or risky implementation details.",
			"- Prefer targeted reads of cited files over broad locate/search.",
			"- If the review artifact is sufficient, produce the plan without additional tools.",
		)
	}
	for _, id := range ids {
		handoff := turnsByHandoff[id]
		if handoff == nil {
			continue
		}
		result := sourceHandoffResultText(state, handoff)
		lines = append(lines,
			"",
			"## Source handoff "+strings.TrimSpace(handoff.HandoffID),
			"Agent: "+strings.TrimSpace(handoff.ChildAgentID),
			"Task: "+strings.TrimSpace(handoff.Task),
			"Provided kinds: "+strings.Join(compactHandoffIDs(handoff.ProvidedKinds), ", "),
			"Result:",
			result,
		)
	}
	return []prompt.Fragment{{
		Kind:      prompt.KindRuntime,
		Source:    prompt.SourceRuntime,
		Stability: prompt.StabilityDynamic,
		Key:       "source-handoffs",
		Label:     "source-handoffs",
		Content:   strings.Join(lines, "\n"),
	}}
}

func sourceHandoffsProvideKind(handoffs map[string]*events.AgentHandoffState, ids []string, kind string) bool {
	for _, id := range ids {
		if handoffProvidesKind(handoffs[id], kind) {
			return true
		}
	}
	return false
}

func sourceHandoffResultText(state events.SessionState, handoff *events.AgentHandoffState) string {
	if review := reviewForHandoff(state, handoff); review != nil {
		return formatReviewSourceText(review)
	}
	result := strings.TrimSpace(handoff.AssistantText)
	if len(result) > maxSourceHandoffResultChars {
		result = result[:maxSourceHandoffResultChars] + "\n[truncated]"
	}
	return result
}

func reviewForHandoff(state events.SessionState, handoff *events.AgentHandoffState) *events.ReviewState {
	if handoff == nil {
		return nil
	}
	handoffID := strings.TrimSpace(handoff.HandoffID)
	if handoffID == "" {
		return nil
	}
	if review := state.Reviews[handoffID]; review != nil {
		return review
	}
	for _, reviewID := range state.ReviewOrder {
		review := state.Reviews[reviewID]
		if review != nil && strings.TrimSpace(review.SourceHandoffID) == handoffID {
			return review
		}
	}
	return nil
}

func formatReviewSourceText(review *events.ReviewState) string {
	if review == nil {
		return ""
	}
	lines := []string{
		"Structured review artifact:",
		"Title: " + strings.TrimSpace(review.Title),
		"Overall correctness: " + strings.TrimSpace(review.OverallCorrectness),
		"Overall summary: " + strings.TrimSpace(review.OverallSummary),
	}
	if len(review.Findings) == 0 {
		lines = append(lines, "Findings: none")
		return strings.Join(lines, "\n")
	}
	lines = append(lines, "Findings:")
	for _, finding := range review.Findings {
		location := strings.TrimSpace(finding.Path)
		if finding.Line > 0 {
			location = location + ":" + strconv.Itoa(finding.Line)
		}
		lines = append(lines,
			"- ["+strings.TrimSpace(finding.Severity)+"] "+strings.TrimSpace(finding.Title),
			"  Location: "+location,
			"  Explanation: "+strings.TrimSpace(finding.Explanation),
		)
	}
	return strings.Join(lines, "\n")
}
