package service

import (
	"encoding/json"
	"strings"

	"github.com/sageil/kodacode/v1/internal/provider"
)

const (
	planApprovalPurpose           = "plan_approval"
	planApprovalQuestionText      = "How would you like to proceed?"
	planApprovalDecisionMarkerTag = "[PLAN_APPROVAL_DECISION]"
	planApprovalSaveOption        = "Save plan and proceed"
	planApprovalProceedOption     = "Proceed without saving plan files"
	planApprovalRejectOption      = "Reject plan"
	legacyPlanApprovalProceedOption = "Proceed without saving"
)

var planApprovalOptions = []string{
	planApprovalSaveOption,
	planApprovalProceedOption,
	planApprovalRejectOption,
}

type planApprovalRecord struct {
	Decision string `json:"decision"`
	Answer   string `json:"answer,omitempty"`
}

func latestPlannerMessageIndex(msgs []provider.Message) int {
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != "assistant" {
			continue
		}
		for _, p := range m.Parts {
			tc, ok := p.(provider.ToolCallPart)
			if !ok || tc.Name != "subagent" {
				continue
			}
			if plannerAgentIDFromArgs(tc.Arguments) == "planner" {
				return i
			}
		}
	}
	return -1
}

func plannerAgentIDFromArgs(args string) string {
	var parsed struct {
		AgentID string `json:"agent_id"`
		Agent   string `json:"agent"`
		Tool    string `json:"tool"`
		Name    string `json:"name"`
	}
	if json.Unmarshal([]byte(args), &parsed) != nil {
		return ""
	}
	for _, v := range []string{parsed.AgentID, parsed.Agent, parsed.Tool, parsed.Name} {
		if v != "" {
			return v
		}
	}
	return ""
}

func encodePlanApprovalDecision(decision planApprovalDecision, answer string) string {
	decisionName := "pending"
	switch decision {
	case planApprovalApproved:
		decisionName = "approved"
	case planApprovalRejected:
		decisionName = "rejected"
	}
	payload, _ := json.Marshal(planApprovalRecord{
		Decision: decisionName,
		Answer:   normalizePlanApprovalAnswer(answer),
	})
	return planApprovalDecisionMarkerTag + " " + string(payload)
}

func parsePlanApprovalDecisionText(text string) (planApprovalDecision, string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(text), planApprovalDecisionMarkerTag+" ")
	if !ok {
		return planApprovalPending, "", false
	}
	var payload planApprovalRecord
	if json.Unmarshal([]byte(rest), &payload) != nil {
		return planApprovalPending, "", false
	}
	answer := strings.TrimSpace(payload.Answer)
	switch strings.ToLower(payload.Decision) {
	case "approved":
		return planApprovalApproved, answer, true
	case "rejected":
		return planApprovalRejected, answer, true
	default:
		return planApprovalPending, answer, true
	}
}

func classifyPlanApprovalSelection(answer string) planApprovalDecision {
	switch normalizePlanApprovalAnswer(answer) {
	case planApprovalSaveOption, planApprovalProceedOption:
		return planApprovalApproved
	case "":
		return planApprovalRejected
	default:
		return planApprovalRejected
	}
}

func normalizePlanApprovalAnswer(answer string) string {
	switch strings.TrimSpace(answer) {
	case legacyPlanApprovalProceedOption:
		return planApprovalProceedOption
	default:
		return strings.TrimSpace(answer)
	}
}

func latestPlanApprovalState(msgs []provider.Message) (planApprovalDecision, string) {
	plannerIdx := latestPlannerMessageIndex(msgs)
	if plannerIdx < 0 {
		return planApprovalPending, ""
	}

	type planQuestion struct {
		id      string
		options []planOption
	}

	var latestQuestion *planQuestion
	decision := planApprovalPending
	answer := ""

	for i := plannerIdx + 1; i < len(msgs); i++ {
		m := msgs[i]
		switch m.Role {
		case "assistant":
			for _, p := range m.Parts {
				tc, ok := p.(provider.ToolCallPart)
				if !ok || tc.Name != "question" || !isPlanApprovalQuestion(tc.Arguments) {
					continue
				}
				latestQuestion = &planQuestion{
					id:      tc.ID,
					options: parsePlanOptions(tc.Arguments),
				}
				decision = planApprovalPending
				answer = ""
			}
		case "user":
			if parsedDecision, parsedAnswer, ok := parsePlanApprovalDecisionText(provider.TextFromParts(m.Parts)); ok {
				decision = parsedDecision
				answer = parsedAnswer
				latestQuestion = nil
				continue
			}
			if latestQuestion == nil {
				continue
			}
			for _, p := range m.Parts {
				tr, ok := p.(provider.ToolResultPart)
				if !ok || tr.ToolCallID != latestQuestion.id {
					continue
				}
				decision = classifyPlanApprovalAnswer(tr.Output, latestQuestion.options)
				answer = extractAnswer(tr.Output)
			}
		}
	}

	return decision, answer
}


// priorApprovalInEffect returns true if there is an approved plan decision
// in the message history that is not followed by a rejection. This handles
// the case where a new planner is called during execution of an approved
// plan — tools should remain available until the new plan is decided.
func priorApprovalInEffect(msgs []provider.Message) bool {
	approvalQuestionIDs := make(map[string][]planOption)
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, p := range m.Parts {
			tc, ok := p.(provider.ToolCallPart)
			if !ok || tc.Name != "question" || !isPlanApprovalQuestion(tc.Arguments) {
				continue
			}
			approvalQuestionIDs[tc.ID] = parsePlanOptions(tc.Arguments)
		}
	}

	lastDecision := planApprovalPending
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		if d, _, ok := parsePlanApprovalDecisionText(provider.TextFromParts(m.Parts)); ok {
			lastDecision = d
			continue
		}
		for _, p := range m.Parts {
			tr, ok := p.(provider.ToolResultPart)
			if !ok {
				continue
			}
			opts, isApprovalQ := approvalQuestionIDs[tr.ToolCallID]
			if !isApprovalQ {
				continue
			}
			lastDecision = classifyPlanApprovalAnswer(tr.Output, opts)
		}
	}
	return lastDecision == planApprovalApproved
}
