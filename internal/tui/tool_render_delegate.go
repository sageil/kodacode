package tui

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

type delegateToolViewInput struct {
	ChildAgentID   string `json:"agent_id"`
	Task           string `json:"task"`
	ContextSummary string `json:"context_summary"`
}

type delegateLogicalRequest struct {
	ChildAgentID   string
	Task           string
	ContextSummary string
}

func isDelegateToolCall(call *events.ToolCallState) bool {
	return call != nil && strings.TrimSpace(call.ToolName) == tool.DelegateToolName
}

func parseDelegateToolViewInput(raw string) (delegateToolViewInput, bool) {
	var input delegateToolViewInput
	if json.Unmarshal([]byte(raw), &input) != nil {
		return delegateToolViewInput{}, false
	}
	input.ChildAgentID = strings.TrimSpace(input.ChildAgentID)
	input.Task = strings.TrimSpace(input.Task)
	input.ContextSummary = strings.TrimSpace(input.ContextSummary)
	if input.ChildAgentID == "" && input.Task == "" && input.ContextSummary == "" {
		return delegateToolViewInput{}, false
	}
	return input, true
}

func parseDelegateLogicalRequest(raw string) (delegateLogicalRequest, bool) {
	var input struct {
		ChildAgentID   string `json:"agent_id"`
		Task           string `json:"task"`
		ContextSummary string `json:"context_summary"`
	}
	if json.Unmarshal([]byte(raw), &input) != nil {
		return delegateLogicalRequest{}, false
	}
	request := delegateLogicalRequest{
		ChildAgentID:   strings.TrimSpace(input.ChildAgentID),
		Task:           strings.Join(strings.Fields(strings.TrimSpace(input.Task)), " "),
		ContextSummary: strings.Join(strings.Fields(strings.TrimSpace(input.ContextSummary)), " "),
	}
	if request.ChildAgentID == "" && request.Task == "" && request.ContextSummary == "" {
		return delegateLogicalRequest{}, false
	}
	return request, true
}

func parseDelegateToolViewOutput(raw string) (tool.DelegateRecord, bool) {
	var record tool.DelegateRecord
	if json.Unmarshal([]byte(raw), &record) != nil {
		return tool.DelegateRecord{}, false
	}
	normalizeDelegateToolViewRecord(&record)
	if record.HandoffID == "" &&
		record.ChildSessionID == "" &&
		record.ChildTurnID == "" &&
		record.ChildAgentID == "" &&
		strings.TrimSpace(string(record.Status)) == "" &&
		record.AssistantText == "" &&
		record.Error == "" &&
		record.PendingPermission == nil &&
		record.PendingQuestion == nil {
		return tool.DelegateRecord{}, false
	}
	return record, true
}

func normalizeDelegateToolViewRecord(record *tool.DelegateRecord) {
	if record == nil {
		return
	}
	record.HandoffID = strings.TrimSpace(record.HandoffID)
	record.ChildSessionID = strings.TrimSpace(record.ChildSessionID)
	record.ChildTurnID = strings.TrimSpace(record.ChildTurnID)
	record.ChildAgentID = strings.TrimSpace(record.ChildAgentID)
	record.Status = tool.DelegateStatus(strings.TrimSpace(string(record.Status)))
	record.AssistantText = strings.TrimSpace(record.AssistantText)
	record.Error = strings.TrimSpace(record.Error)
	if record.PendingPermission != nil {
		record.PendingPermission.RequestID = strings.TrimSpace(record.PendingPermission.RequestID)
		record.PendingPermission.Kind = strings.TrimSpace(record.PendingPermission.Kind)
		record.PendingPermission.ToolName = strings.TrimSpace(record.PendingPermission.ToolName)
		record.PendingPermission.Access = strings.TrimSpace(record.PendingPermission.Access)
		record.PendingPermission.Path = strings.TrimSpace(record.PendingPermission.Path)
		record.PendingPermission.WorkingDirectory = strings.TrimSpace(record.PendingPermission.WorkingDirectory)
		record.PendingPermission.Command = strings.TrimSpace(record.PendingPermission.Command)
		record.PendingPermission.Reason = strings.TrimSpace(record.PendingPermission.Reason)
	}
	if record.PendingQuestion != nil {
		record.PendingQuestion.RequestID = strings.TrimSpace(record.PendingQuestion.RequestID)
		record.PendingQuestion.ToolName = strings.TrimSpace(record.PendingQuestion.ToolName)
		record.PendingQuestion.Question = strings.TrimSpace(record.PendingQuestion.Question)
		record.PendingQuestion.Options = compactDelegateOptions(record.PendingQuestion.Options)
	}
}

func compactDelegateOptions(options []string) []string {
	if len(options) == 0 {
		return nil
	}
	out := make([]string, 0, len(options))
	for _, option := range options {
		option = strings.TrimSpace(option)
		if option == "" {
			continue
		}
		out = append(out, option)
	}
	return out
}

func delegateToolDisplayName(call *events.ToolCallState) string {
	if agentID := delegateToolAgentID(call); agentID != "" {
		return "delegate " + agentID
	}
	return "delegate"
}

func delegateToolAgentID(call *events.ToolCallState) string {
	if input, ok := parseDelegateToolViewInput(call.Input); ok && input.ChildAgentID != "" {
		return input.ChildAgentID
	}
	if record, ok := parseDelegateToolViewOutput(call.Output); ok && record.ChildAgentID != "" {
		return record.ChildAgentID
	}
	return ""
}

func delegateToolAgentLabel(call *events.ToolCallState) string {
	if agentID := delegateToolAgentID(call); agentID != "" {
		return titleizeDelegatedAgentLabel(agentID)
	}
	return "Delegated"
}

func delegateToolListSummary(call *events.ToolCallState) string {
	input, ok := parseDelegateToolViewInput(call.Input)
	if !ok {
		return ""
	}
	parts := make([]string, 0, 3)
	if input.ChildAgentID != "" {
		parts = append(parts, "agent: "+input.ChildAgentID)
	}
	if input.Task != "" {
		parts = append(parts, "task: "+input.Task)
	}
	return strings.Join(parts, " · ")
}

func delegateInspectorParams(call *events.ToolCallState) []inspectorParam {
	input, _ := parseDelegateToolViewInput(call.Input)
	record, _ := parseDelegateToolViewOutput(call.Output)

	params := make([]inspectorParam, 0, 8)
	agentID := input.ChildAgentID
	if agentID == "" {
		agentID = record.ChildAgentID
	}
	if agentID != "" {
		params = append(params, inspectorParam{Label: "Agent", Value: agentID})
	}
	if status := strings.TrimSpace(string(record.Status)); status != "" {
		params = append(params, inspectorParam{Label: "Status", Value: status})
	}
	if record.HandoffID != "" {
		params = append(params, inspectorParam{Label: "Handoff", Value: record.HandoffID})
	}
	if record.ChildSessionID != "" {
		params = append(params, inspectorParam{Label: "Child Session", Value: record.ChildSessionID})
	}
	if record.ChildTurnID != "" {
		params = append(params, inspectorParam{Label: "Child Turn", Value: record.ChildTurnID})
	}
	if record.PendingPermission != nil {
		params = append(params, inspectorParam{Label: "Blocked", Value: "pending permission"})
	}
	if record.PendingQuestion != nil {
		params = append(params, inspectorParam{Label: "Blocked", Value: "pending question"})
	}
	if childError := summarizeBlock(record.Error); childError != "" {
		params = append(params, inspectorParam{Label: "Child Error", Value: childError, Error: true})
	}
	if errorText := summarizeBlock(call.Error); errorText != "" {
		params = append(params, inspectorParam{Label: "Error", Value: errorText, Error: true})
	}
	return params
}

func renderDelegateToolDetailMarkdownForSession(m Model, sessionID string, ref sessionToolCallRef, call *events.ToolCallState) string {
	if call == nil {
		return ""
	}
	input, hasInput := parseDelegateToolViewInput(call.Input)
	output := strings.TrimSpace(toolResultOutputForSession(m, sessionID, &ref, call))
	record, hasRecord := parseDelegateToolViewOutput(output)
	errorText := strings.TrimSpace(toolResultErrorForSession(m, sessionID, &ref, call))
	notice := ""
	if !toolResultBodyLoadedForSession(m, sessionID, &ref) {
		notice = strings.TrimSpace(toolResultPreviewNotice(call))
	}
	if !hasInput && !hasRecord && errorText == "" {
		return renderGenericToolDetailMarkdownForSession(m, sessionID, ref, call)
	}

	lines := make([]string, 0, 24)
	if errorText != "" {
		lines = append(lines, "## Error", toolDetailErrorBlock(errorText))
	}

	details := delegateMarkdownDetailLines(input, record)
	if len(details) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Delegation")
		for _, line := range details {
			lines = append(lines, "- "+line)
		}
	}

	if hasInput && input.Task != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Task", input.Task)
	}
	if hasInput && input.ContextSummary != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Context", input.ContextSummary)
	}
	if hasRecord && record.AssistantText != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Result", record.AssistantText)
	}
	if hasRecord && record.PendingPermission != nil {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Pending Permission", delegatePendingPermissionBody(record.PendingPermission))
	}
	if hasRecord && record.PendingQuestion != nil {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Pending Question", delegatePendingQuestionBody(record.PendingQuestion))
	}
	if hasRecord && record.Error != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Child Error", toolDetailErrorBlock(record.Error))
	}
	if !hasRecord && errorText == "" && call.Executing {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "## Output", "(running...)")
	}
	if notice != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, "_"+notice+"_")
	}
	return strings.Join(lines, "\n")
}

func delegateMarkdownDetailLines(input delegateToolViewInput, record tool.DelegateRecord) []string {
	lines := make([]string, 0, 6)
	agentID := input.ChildAgentID
	if agentID == "" {
		agentID = record.ChildAgentID
	}
	if agentID != "" {
		lines = append(lines, "Agent: "+agentID)
	}
	if status := strings.TrimSpace(string(record.Status)); status != "" {
		lines = append(lines, "Status: "+status)
	}
	if record.HandoffID != "" {
		lines = append(lines, "Handoff: "+record.HandoffID)
	}
	if record.ChildSessionID != "" {
		lines = append(lines, "Child Session: "+record.ChildSessionID)
	}
	if record.ChildTurnID != "" {
		lines = append(lines, "Child Turn: "+record.ChildTurnID)
	}
	return lines
}

func renderDelegateToolTranscriptOutput(m Model, ref *sessionToolCallRef, call *events.ToolCallState, width int) string {
	if call == nil {
		return ""
	}
	output := strings.TrimSpace(toolResultOutput(m, ref, call))
	record, hasRecord := parseDelegateToolViewOutput(output)
	errorText := strings.TrimSpace(toolResultError(m, ref, call))
	notice := ""
	if !toolResultBodyLoaded(m, ref) {
		notice = strings.TrimSpace(toolResultPreviewNotice(call))
	}
	if !hasRecord {
		switch {
		case errorText != "":
			body := "Error:\n" + errorText
			if notice != "" {
				body += "\n\n(" + notice + ")"
			}
			return body
		case call.Executing:
			return "(running...)"
		default:
			return ""
		}
	}

	lines := make([]string, 0, 24)
	if record.AssistantText != "" {
		lines = append(lines, renderDelegateTranscriptSection("Result", record.AssistantText, width)...)
	}
	if record.PendingPermission != nil {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, renderDelegateTranscriptSection("Pending Permission", delegatePendingPermissionBody(record.PendingPermission), width)...)
	}
	if record.PendingQuestion != nil {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, renderDelegateTranscriptSection("Pending Question", delegatePendingQuestionBody(record.PendingQuestion), width)...)
	}
	if record.Error != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, renderDelegateTranscriptSection("Child Error", record.Error, width)...)
	}
	if errorText != "" {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, renderDelegateTranscriptSection("Error", errorText, width)...)
	}
	if len(lines) == 0 && call.Executing {
		lines = append(lines, "(running...)")
	}
	body := strings.Join(lines, "\n")
	if notice != "" {
		if body != "" {
			body += "\n\n"
		}
		body += "(" + notice + ")"
	}
	return body
}

func renderDelegateTranscriptSection(label, body string, width int) []string {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	lines := []string{label + ":"}
	lines = append(lines, indentStructuredLines(wrapStructuredText(body, max(width-2, 24)), "  ")...)
	return lines
}

func delegatePendingPermissionBody(pending *tool.DelegatePendingPermission) string {
	if pending == nil {
		return ""
	}
	lines := make([]string, 0, 7)
	if pending.Kind != "" {
		lines = append(lines, "Kind: "+pending.Kind)
	}
	if pending.ToolName != "" {
		lines = append(lines, "Tool: "+pending.ToolName)
	}
	if pending.Access != "" {
		lines = append(lines, "Access: "+pending.Access)
	}
	if pending.Path != "" {
		lines = append(lines, "Path: "+pending.Path)
	}
	if pending.WorkingDirectory != "" {
		lines = append(lines, "Working Directory: "+pending.WorkingDirectory)
	}
	if pending.Command != "" {
		lines = append(lines, "Command: "+pending.Command)
	}
	if pending.Reason != "" {
		lines = append(lines, "Reason: "+pending.Reason)
	}
	return strings.Join(lines, "\n")
}

func delegatePendingQuestionBody(pending *tool.DelegatePendingQuestion) string {
	if pending == nil {
		return ""
	}
	lines := make([]string, 0, 4+len(pending.Options))
	if pending.ToolName != "" {
		lines = append(lines, "Tool: "+pending.ToolName)
	}
	if pending.Question != "" {
		lines = append(lines, "Question: "+pending.Question)
	}
	if len(pending.Options) > 0 {
		lines = append(lines, "Options:")
		for idx, option := range pending.Options {
			lines = append(lines, strconv.Itoa(idx+1)+". "+option)
		}
	}
	return strings.Join(lines, "\n")
}
