package events

import (
	"errors"
	"strings"
)

type PlanState struct {
	PlanID          string
	SourceHandoffID string
	SourceTurnID    string
	Title           string
	Markdown        string
	CreatedByAgent  string
}

type PlanRecordedPayload struct {
	PlanID          string `json:"plan_id"`
	SourceHandoffID string `json:"source_handoff_id,omitempty"`
	SourceTurnID    string `json:"source_turn_id,omitempty"`
	Title           string `json:"title"`
	Markdown        string `json:"markdown"`
	CreatedByAgent  string `json:"created_by_agent"`
}

func (PlanRecordedPayload) eventType() Type { return TypePlanRecorded }

func (p PlanRecordedPayload) validate() error {
	switch {
	case strings.TrimSpace(p.PlanID) == "":
		return errors.New("plan_id is required")
	case strings.TrimSpace(p.SourceHandoffID) == "" && strings.TrimSpace(p.SourceTurnID) == "":
		return errors.New("plan source is required")
	case strings.TrimSpace(p.Title) == "":
		return errors.New("title is required")
	case strings.TrimSpace(p.Markdown) == "":
		return errors.New("markdown is required")
	case strings.TrimSpace(p.CreatedByAgent) == "":
		return errors.New("created_by_agent is required")
	default:
		return nil
	}
}

func planStateFromPayload(payload PlanRecordedPayload) *PlanState {
	return &PlanState{
		PlanID:          strings.TrimSpace(payload.PlanID),
		SourceHandoffID: strings.TrimSpace(payload.SourceHandoffID),
		SourceTurnID:    strings.TrimSpace(payload.SourceTurnID),
		Title:           strings.TrimSpace(payload.Title),
		Markdown:        strings.TrimRight(payload.Markdown, "\n"),
		CreatedByAgent:  strings.TrimSpace(payload.CreatedByAgent),
	}
}

func clonePlanState(state *PlanState) *PlanState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}

func clonePlanStates(plans map[string]*PlanState) map[string]*PlanState {
	out := make(map[string]*PlanState, len(plans))
	for id, plan := range plans {
		out[id] = clonePlanState(plan)
	}
	return out
}
