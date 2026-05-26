package events

import (
	"errors"
	"fmt"
	"strings"
)

const (
	TurnContinuationReasonContextLimit   = "context_limit"
	TurnContinuationReasonQuestionAnswer = "question_answer"
)

type TurnContinuationStartedPayload struct {
	PreviousTurnID string
	Reason         string
	Summary        TurnWorkStateSummaryPayload
}

func (TurnContinuationStartedPayload) eventType() Type { return TypeTurnContinuationStarted }

func (p TurnContinuationStartedPayload) validate() error {
	if strings.TrimSpace(p.PreviousTurnID) == "" {
		return errors.New("previous_turn_id is required")
	}
	switch strings.TrimSpace(p.Reason) {
	case TurnContinuationReasonContextLimit:
		return p.Summary.validate()
	case TurnContinuationReasonQuestionAnswer:
		return p.Summary.validate()
	default:
		return fmt.Errorf("reason %q is invalid", p.Reason)
	}
}

type TurnContinuationState struct {
	PreviousTurnID string
	Reason         string
	Summary        TurnWorkStateSummaryState
}

func cloneTurnContinuationState(state *TurnContinuationState) *TurnContinuationState {
	if state == nil {
		return nil
	}
	copyState := *state
	return &copyState
}
