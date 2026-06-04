package app

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

var (
	ErrPlannerSavePlanQuestionRequiresVisiblePlan = errors.New("planner save-plan question requires a visible plan")
	ErrPlannerSavePlanQuestionInvalid             = errors.New("planner save-plan question contract is invalid")
	ErrPlannerPlanApprovalDisabledByWorkflow      = errors.New("planner plan approval is disabled during active workflow")
	ErrPlannerPlanApprovalDisabled                = errors.New("planner plan approval is disabled")
)

const plannerSavePlanQuestionRequiresVisiblePlanText = "Show the completed plan to the user first, then call `question` with purpose `planner_save_plan`."
const plannerSavePlanQuestionInvalidText = "question failed: planner_save_plan requires exactly two options: `Save plan` and `Revise plan`."
const plannerPlanApprovalDisabledByWorkflowText = "question failed: workflow owns phase approval. Do not use planner_save_plan during an active workflow; call workflow_phase_output for required phase outputs or follow the workflow phase instructions."
const plannerPlanApprovalDisabledText = "question failed: planner_save_plan is disabled. Continue with normal assistant text, or enable workflow.planner_approval in config.yaml to use the runtime Save/Apply/Revise/Stop planner prompt."

func isPlannerSavePlanQuestion(toolName string, arguments json.RawMessage) bool {
	return strings.TrimSpace(toolName) == tool.QuestionToolName &&
		questionPurposeFromArguments(arguments) == events.QuestionPurposePlannerSavePlan
}

func questionPurposeFromArguments(arguments json.RawMessage) string {
	var raw struct {
		Purpose *string `json:"purpose"`
	}
	if err := json.Unmarshal(arguments, &raw); err != nil || raw.Purpose == nil {
		return ""
	}
	return strings.TrimSpace(*raw.Purpose)
}

func questionOptionsFromArguments(arguments json.RawMessage) []string {
	var raw struct {
		Options []string `json:"options"`
	}
	if err := json.Unmarshal(arguments, &raw); err != nil {
		return nil
	}
	options := make([]string, 0, len(raw.Options))
	for _, option := range raw.Options {
		if option = strings.TrimSpace(option); option != "" {
			options = append(options, option)
		}
	}
	return options
}

func enforcePlannerSavePlanQuestionShape(input ExecuteToolInput) error {
	if strings.TrimSpace(input.ToolName) != tool.QuestionToolName {
		return nil
	}
	purpose := questionPurposeFromArguments(input.Arguments)
	options := questionOptionsFromArguments(input.Arguments)
	mentionsSavePlan := false
	for _, option := range options {
		if looksLikeSavePlanOption(option) {
			mentionsSavePlan = true
			break
		}
	}
	if purpose != events.QuestionPurposePlannerSavePlan && !mentionsSavePlan {
		return nil
	}
	if purpose != events.QuestionPurposePlannerSavePlan {
		return ErrPlannerSavePlanQuestionInvalid
	}
	if len(options) != 2 || !isSavePlanApprovalAnswer(options[0]) || strings.TrimSpace(options[1]) != "Revise plan" {
		return ErrPlannerSavePlanQuestionInvalid
	}
	return nil
}

func enforcePlannerSavePlanQuestionWorkflowBoundary(state events.SessionState, input ExecuteToolInput) error {
	if !workflowOwnsPlanApproval(state) || strings.TrimSpace(input.ToolName) != tool.QuestionToolName {
		return nil
	}
	purpose := questionPurposeFromArguments(input.Arguments)
	if purpose == events.QuestionPurposePlannerSavePlan || purpose == events.QuestionPurposePlannerPlanDecision {
		return ErrPlannerPlanApprovalDisabledByWorkflow
	}
	for _, option := range questionOptionsFromArguments(input.Arguments) {
		if looksLikeSavePlanOption(option) {
			return ErrPlannerPlanApprovalDisabledByWorkflow
		}
	}
	return nil
}

func enforcePlannerSavePlanQuestionVisible(state events.SessionState, input ExecuteToolInput) error {
	if !isPlannerSavePlanQuestion(input.ToolName, input.Arguments) {
		return nil
	}
	turn := state.Turns[input.TurnID]
	if turn == nil || strings.TrimSpace(turn.AssistantText) == "" {
		return ErrPlannerSavePlanQuestionRequiresVisiblePlan
	}
	return nil
}

func isSavePlanApprovalAnswer(answer string) bool {
	answer = strings.TrimSpace(answer)
	return answer == "Save plan" || strings.HasPrefix(answer, "Save plan ")
}

func looksLikeSavePlanOption(option string) bool {
	option = strings.ToLower(strings.TrimSpace(option))
	return strings.Contains(option, "save") &&
		(strings.Contains(option, "plan") || strings.Contains(option, "execution"))
}

func workflowOwnsPlanApproval(state events.SessionState) bool {
	workflow := state.Workflow
	return workflow != nil &&
		strings.TrimSpace(workflow.WorkflowID) != "" &&
		workflow.Status != events.WorkflowStatusCompleted
}
