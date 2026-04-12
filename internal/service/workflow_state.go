package service

import (
	"encoding/json"

	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/tool"
)

func ensureWorkflowState(req *pipeline.TurnRequest) *pipeline.WorkflowState {
	if req.Workflow == nil {
		req.Workflow = hydrateWorkflowState(req.Messages, req.Step)
	}
	updateWorkflowPhase(req)
	return req.Workflow
}

func hydrateWorkflowState(msgs []provider.Message, step int) *pipeline.WorkflowState {
	ws := &pipeline.WorkflowState{
		HasCalledTest:    hasSatisfiedPrebuildCheck(msgs),
		HasCalledPlanner: hasCalledAgent(msgs, "planner"),
	}

	latestDecision, latestAnswer := latestPlanApprovalState(msgs)
	ws.Plan.LatestStatus = workflowApprovalStatus(latestDecision)
	ws.Plan.LatestAnswer = normalizePlanApprovalAnswer(latestAnswer)
	ws.Plan.PriorApprovedInEffect = priorApprovalInEffect(msgs)
	ws.Plan.PendingQuestionID, _ = latestPlanApprovalQuestionState(msgs)
	recomputePlanEffective(ws)

	req := &pipeline.TurnRequest{Step: step, Workflow: ws}
	updateWorkflowPhase(req)
	return ws
}

func latestPlanApprovalQuestionState(msgs []provider.Message) (string, string) {
	plannerIdx := latestPlannerMessageIndex(msgs)
	if plannerIdx < 0 {
		return "", ""
	}

	latestQuestionID := ""
	answer := ""
	for i := plannerIdx + 1; i < len(msgs); i++ {
		m := msgs[i]
		switch m.Role {
		case "assistant":
			for _, p := range m.Parts {
				tc, ok := p.(provider.ToolCallPart)
				if ok && tc.Name == "question" && isPlanApprovalQuestion(tc.Arguments) {
					latestQuestionID = tc.ID
					answer = ""
				}
			}
		case "user":
			if _, parsedAnswer, ok := parsePlanApprovalDecisionText(provider.TextFromParts(m.Parts)); ok {
				return "", normalizePlanApprovalAnswer(parsedAnswer)
			}
			if latestQuestionID == "" {
				continue
			}
			for _, p := range m.Parts {
				tr, ok := p.(provider.ToolResultPart)
				if ok && tr.ToolCallID == latestQuestionID {
					answer = normalizePlanApprovalAnswer(extractAnswer(tr.Output))
					if answer != "" {
						return "", answer
					}
				}
			}
		}
	}

	return latestQuestionID, answer
}

func updateWorkflowPhase(req *pipeline.TurnRequest) {
	if req == nil || req.Workflow == nil {
		return
	}

	switch {
	case req.Workflow.Plan.EffectiveStatus == pipeline.WorkflowApprovalApproved:
		req.Workflow.Phase = pipeline.WorkflowPhaseApproved
	case req.Workflow.HasCalledPlanner && req.Workflow.Plan.LatestStatus == pipeline.WorkflowApprovalRejected:
		req.Workflow.Phase = pipeline.WorkflowPhasePostplanRejected
	case req.Workflow.HasCalledPlanner:
		req.Workflow.Phase = pipeline.WorkflowPhasePostplanPending
	case req.Workflow.HasCalledTest:
		req.Workflow.Phase = pipeline.WorkflowPhasePreplan
	default:
		req.Workflow.Phase = pipeline.WorkflowPhasePrebuild
	}
}

func noteWorkflowToolCalls(req *pipeline.TurnRequest, calls []provider.ToolCall) {
	ws := ensureWorkflowState(req)
	for _, tc := range calls {
		switch tc.Name {
		case "question":
			if isPlanApprovalQuestion(tc.Arguments) {
				ws.Plan.PendingQuestionID = tc.ID
				ws.Plan.LatestStatus = pipeline.WorkflowApprovalPending
				ws.Plan.LatestAnswer = ""
				recomputePlanEffective(ws)
			}
		case "subagent":
			if plannerAgentIDFromArgs(tc.Arguments) != "planner" {
				continue
			}
			if ws.Plan.EffectiveStatus == pipeline.WorkflowApprovalApproved {
				ws.Plan.PriorApprovedInEffect = true
			}
			ws.HasCalledPlanner = true
			ws.Plan.LatestStatus = pipeline.WorkflowApprovalPending
			ws.Plan.LatestAnswer = ""
			ws.Plan.PendingQuestionID = ""
			recomputePlanEffective(ws)
		}
	}
	updateWorkflowPhase(req)
}

func noteWorkflowExecutions(req *pipeline.TurnRequest, executions []toolExecution) {
	ws := ensureWorkflowState(req)
	for _, ex := range executions {
		if executionSatisfiedPrebuild(ex) {
			ws.HasCalledTest = true
		}
		if ex.call.Name != "question" {
			continue
		}

		switch {
		case isPlanApprovalQuestion(ex.call.Arguments):
			notePlanApprovalDecision(req, classifyPlanApprovalAnswer(ex.output, parsePlanOptions(ex.call.Arguments)), extractAnswer(ex.output))
		}
	}
	updateWorkflowPhase(req)
}

func hasSatisfiedPrebuildCheck(msgs []provider.Message) bool {
	executed := make(map[string]bool)
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		for _, p := range m.Parts {
			tr, ok := p.(provider.ToolResultPart)
			if !ok || tr.Error != nil || !toolResultHasExitCode(tr.Metadata) {
				continue
			}
			if tr.ToolCallID != "" {
				executed[tr.ToolCallID] = true
			}
		}
	}

	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, p := range m.Parts {
			tc, ok := p.(provider.ToolCallPart)
			if !ok {
				continue
			}
			switch tc.Name {
			case "test":
				if executed[tc.ID] {
					return true
				}
			}
		}
	}

	return false
}

func executionSatisfiedPrebuild(ex toolExecution) bool {
	if ex.errStr != nil || ex.result == nil || !toolResultHasExitCode(ex.result.Metadata) {
		return false
	}
	switch ex.call.Name {
	case "test":
		return true
	default:
		return false
	}
}

func executionCountsAsVerification(ex toolExecution) bool {
	if ex.errStr != nil || ex.result == nil || !toolResultHasExitCode(ex.result.Metadata) {
		return false
	}
	switch ex.call.Name {
	case "test":
		return true
	case "bash":
		purpose := toolResultPurpose(ex.result.Metadata)
		if purpose == "" {
			purpose = bashPurposeFromArgs(ex.call.Arguments)
		}
		return purpose == tool.BashPurposeVerification || purpose == tool.BashPurposeBuild
	default:
		return false
	}
}

func bashPurposeFromArgs(raw string) string {
	var args struct {
		Purpose string `json:"purpose"`
	}
	if json.Unmarshal([]byte(raw), &args) != nil {
		return ""
	}
	return args.Purpose
}

func toolResultPurpose(metadata map[string]any) string {
	if metadata == nil {
		return ""
	}
	purpose, _ := metadata["purpose"].(string)
	return purpose
}

func toolResultHasExitCode(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	switch metadata["exit_code"].(type) {
	case int, int64, float64:
		return true
	default:
		return false
	}
}

func notePlanApprovalDecision(req *pipeline.TurnRequest, decision planApprovalDecision, answer string) {
	ws := ensureWorkflowState(req)
	ws.Plan.LatestStatus = workflowApprovalStatus(decision)
	ws.Plan.LatestAnswer = normalizePlanApprovalAnswer(answer)
	ws.Plan.PendingQuestionID = ""

	switch ws.Plan.LatestStatus {
	case pipeline.WorkflowApprovalApproved:
		ws.Plan.PriorApprovedInEffect = true
	case pipeline.WorkflowApprovalRejected:
		ws.Plan.PriorApprovedInEffect = false
	}

	recomputePlanEffective(ws)
	updateWorkflowPhase(req)
}

func recomputePlanEffective(ws *pipeline.WorkflowState) {
	switch ws.Plan.LatestStatus {
	case pipeline.WorkflowApprovalApproved, pipeline.WorkflowApprovalRejected:
		ws.Plan.EffectiveStatus = ws.Plan.LatestStatus
	case pipeline.WorkflowApprovalPending:
		if ws.Plan.PriorApprovedInEffect {
			ws.Plan.EffectiveStatus = pipeline.WorkflowApprovalApproved
		} else {
			ws.Plan.EffectiveStatus = pipeline.WorkflowApprovalPending
		}
	default:
		if ws.Plan.PriorApprovedInEffect {
			ws.Plan.EffectiveStatus = pipeline.WorkflowApprovalApproved
		} else {
			ws.Plan.EffectiveStatus = pipeline.WorkflowApprovalPending
		}
	}
}

func workflowApprovalStatus(decision planApprovalDecision) pipeline.WorkflowApprovalStatus {
	switch decision {
	case planApprovalApproved:
		return pipeline.WorkflowApprovalApproved
	case planApprovalRejected:
		return pipeline.WorkflowApprovalRejected
	default:
		return pipeline.WorkflowApprovalPending
	}
}
