package events

import "slices"

type AgentHandoffState struct {
	HandoffID            string
	ToolCallID           string
	ParentSessionID      string
	ParentTurnID         string
	ParentAgentID        string
	ChildSessionID       string
	ChildTurnID          string
	ChildAgentID         string
	Task                 string
	ContextSummary       string
	SourceHandoffIDs     []string
	ProvidedKinds        []string
	ExplorationEntries   []AgentHandoffExplorationEntry
	Model                string
	AllowedTools         []string
	PreviewActive        bool
	PreviewToolName      string
	PreviewAction        string
	PreviewAssistantText string
	Status               AgentResultStatus
	AssistantText        string
	Error                string
	PermissionRequestID  string
	PermissionKind       PermissionRequestKind
	PermissionToolName   string
	PermissionAccess     string
	PermissionPath       string
	PermissionDir        string
	PermissionCommand    string
	PermissionReason     string
	ExecutionApproval    *ExecutionApprovalState
	QuestionRequestID    string
	QuestionToolName     string
	QuestionText         string
	QuestionOptions      []string
	Reused               bool
	ReusedContent        string
}

func cloneAgentHandoffState(state *AgentHandoffState) *AgentHandoffState {
	if state == nil {
		return nil
	}
	return &AgentHandoffState{
		HandoffID:            state.HandoffID,
		ToolCallID:           state.ToolCallID,
		ParentSessionID:      state.ParentSessionID,
		ParentTurnID:         state.ParentTurnID,
		ParentAgentID:        state.ParentAgentID,
		ChildSessionID:       state.ChildSessionID,
		ChildTurnID:          state.ChildTurnID,
		ChildAgentID:         state.ChildAgentID,
		Task:                 state.Task,
		ContextSummary:       state.ContextSummary,
		SourceHandoffIDs:     append([]string(nil), state.SourceHandoffIDs...),
		ProvidedKinds:        append([]string(nil), state.ProvidedKinds...),
		ExplorationEntries:   append([]AgentHandoffExplorationEntry(nil), state.ExplorationEntries...),
		Model:                state.Model,
		AllowedTools:         slices.Clone(state.AllowedTools),
		PreviewActive:        state.PreviewActive,
		PreviewToolName:      state.PreviewToolName,
		PreviewAction:        state.PreviewAction,
		PreviewAssistantText: state.PreviewAssistantText,
		Status:               state.Status,
		AssistantText:        state.AssistantText,
		Error:                state.Error,
		PermissionRequestID:  state.PermissionRequestID,
		PermissionKind:       state.PermissionKind,
		PermissionToolName:   state.PermissionToolName,
		PermissionAccess:     state.PermissionAccess,
		PermissionPath:       state.PermissionPath,
		PermissionDir:        state.PermissionDir,
		PermissionCommand:    state.PermissionCommand,
		PermissionReason:     state.PermissionReason,
		ExecutionApproval:    cloneExecutionApprovalState(state.ExecutionApproval),
		QuestionRequestID:    state.QuestionRequestID,
		QuestionToolName:     state.QuestionToolName,
		QuestionText:         state.QuestionText,
		QuestionOptions:      append([]string(nil), state.QuestionOptions...),
		Reused:               state.Reused,
		ReusedContent:        state.ReusedContent,
	}
}
