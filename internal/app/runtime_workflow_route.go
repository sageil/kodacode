package app

import (
	"context"
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

type workflowRouteRecommendation struct {
	WorkflowID   string
	AgentID      string
	Confidence   string
	Reasons      []string
	Alternatives []string
}

func (r *Runtime) appendWorkflowRouteRecommendation(ctx context.Context, input runExistingTurnInput, view turnStartSessionView) error {
	if strings.TrimSpace(input.WorkflowID) != "" || view.workflow != nil {
		return nil
	}
	recommendation, ok := r.workflowRouteRecommendation(view.workspaceRoot, input.UserText)
	if !ok {
		return nil
	}
	_, err := r.Sessions.append(ctx, events.Draft{
		SessionID: input.SessionID,
		TurnID:    workflowEventTurnID(input.TurnID),
		Type:      events.TypeWorkflowRouteRecommended,
		Payload: events.WorkflowRouteRecommendedPayload{
			WorkflowID:   recommendation.WorkflowID,
			AgentID:      recommendation.AgentID,
			Confidence:   recommendation.Confidence,
			Reasons:      append([]string(nil), recommendation.Reasons...),
			Alternatives: append([]string(nil), recommendation.Alternatives...),
		},
	})
	return err
}

func (r *Runtime) workflowRouteRecommendation(workspaceRoot, userText string) (workflowRouteRecommendation, bool) {
	text := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(userText)), " "))
	if text == "" {
		return workflowRouteRecommendation{}, false
	}
	candidate, ok := classifyWorkflowRoute(text)
	if !ok || !r.workflowRouteAvailable(workspaceRoot, candidate.WorkflowID) {
		return workflowRouteRecommendation{}, false
	}
	candidate.Alternatives = r.availableWorkflowRouteAlternatives(workspaceRoot, candidate.WorkflowID, candidate.Alternatives)
	return candidate, true
}

func classifyWorkflowRoute(text string) (workflowRouteRecommendation, bool) {
	switch {
	case containsWorkflowRouteAny(text, "review", "audit", "acceptance", "regression review", "check current changes"):
		return workflowRouteRecommendation{
			WorkflowID:   "review",
			AgentID:      reviewerAgentID,
			Confidence:   "high",
			Reasons:      []string{"request asks for review or acceptance checking"},
			Alternatives: []string{"delivery", "explore"},
		}, true
	case containsWorkflowRouteAny(text, "debug", "failing test", "test failure", "bug", "panic", "traceback", "reproduce", "broken"):
		return workflowRouteRecommendation{
			WorkflowID:   "debug",
			AgentID:      "engineer",
			Confidence:   "high",
			Reasons:      []string{"request describes a failure, bug, or reproduction task"},
			Alternatives: []string{"delivery", "review"},
		}, true
	case containsWorkflowRouteAny(text, "explain", "summarize", "inspect", "map", "where is", "how does", "architecture", "understand"):
		return workflowRouteRecommendation{
			WorkflowID:   "explore",
			AgentID:      "planner",
			Confidence:   "medium",
			Reasons:      []string{"request appears read-only or analysis-oriented"},
			Alternatives: []string{"review", "delivery"},
		}, true
	case containsWorkflowRouteAny(text, "implement", "add", "change", "update", "refactor", "build", "apply"):
		return workflowRouteRecommendation{
			WorkflowID:   "delivery",
			AgentID:      "engineer",
			Confidence:   "medium",
			Reasons:      []string{"request appears to require planned implementation"},
			Alternatives: []string{"debug", "explore"},
		}, true
	default:
		return workflowRouteRecommendation{}, false
	}
}

func (r *Runtime) workflowRouteAvailable(workspaceRoot, workflowID string) bool {
	catalog, err := r.workflowCatalog(workspaceRoot)
	if err != nil {
		return false
	}
	_, err = catalog.Get(workflowID)
	return err == nil
}

func (r *Runtime) availableWorkflowRouteAlternatives(workspaceRoot, workflowID string, candidates []string) []string {
	catalog, err := r.workflowCatalog(workspaceRoot)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || candidate == strings.TrimSpace(workflowID) {
			continue
		}
		if _, err := catalog.Get(candidate); err == nil {
			out = append(out, candidate)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func containsWorkflowRouteAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
