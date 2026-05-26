package events

import (
	"errors"
	"strings"
)

type QuestionRequestedPayload struct {
	QuestionID string
	ToolCallID string
	ToolName   string
	PlanID     string
	Question   string
	Options    []string
	Multiple   bool
	Purpose    string
}

const (
	QuestionPurposeTurnLoopResolution  = "turn_loop_resolution"
	QuestionPurposePlannerSavePlan     = "planner_save_plan"
	QuestionPurposePlannerPlanDecision = "planner_plan_decision"
)

func (QuestionRequestedPayload) eventType() Type { return TypeQuestionRequested }

func (p QuestionRequestedPayload) validate() error {
	if strings.TrimSpace(p.QuestionID) == "" {
		return errors.New("question_id is required")
	}
	toolCallID := strings.TrimSpace(p.ToolCallID)
	toolName := strings.TrimSpace(p.ToolName)
	if (toolCallID == "") != (toolName == "") {
		return errors.New("tool_call_id and tool_name must either both be set or both be empty")
	}
	if strings.TrimSpace(p.Question) == "" {
		return errors.New("question is required")
	}
	if len(p.Options) == 0 {
		return errors.New("options is required")
	}
	for _, option := range p.Options {
		if strings.TrimSpace(option) == "" {
			return errors.New("options must not contain empty values")
		}
	}
	return nil
}

type QuestionAnsweredPayload struct {
	QuestionID string
	ToolCallID string
	PlanID     string
	Answer     string
}

func (QuestionAnsweredPayload) eventType() Type { return TypeQuestionAnswered }

func (p QuestionAnsweredPayload) validate() error {
	if strings.TrimSpace(p.QuestionID) == "" {
		return errors.New("question_id is required")
	}
	return nil
}
