package tui

import (
	"strings"

	"github.com/sageil/kodacode/internal/events"
)

func shouldSyncTranscriptForEvent(event events.Event) bool {
	switch event.Type {
	case events.TypeSessionConfigured,
		events.TypeSessionModelRouteUpdated,
		events.TypeSessionWorkspaceRootsAdded,
		events.TypeSessionPermissionModeUpdated,
		events.TypeSessionHistoryCheckpoint,
		events.TypeSessionStateSnapshot,
		events.TypeContextCompactionStarted,
		events.TypeTurnWorkStateUpdated,
		events.TypeTurnProviderUsageRecorded,
		events.TypeTurnProviderUsageReported,
		events.TypeToolCallDelta,
		events.TypeToolExecOutput,
		events.TypeExecutionOutput,
		events.TypeExecutionBackgroundObserved:
		return false
	default:
		return true
	}
}

func shouldSyncInspectorForEvent(event events.Event) bool {
	switch event.Type {
	case events.TypeAssistantPreviewDelta,
		events.TypeAssistantPreviewReset,
		events.TypeReasoningDelta,
		events.TypeTurnWorkStateUpdated,
		events.TypeToolCallDelta,
		events.TypeToolExecOutput,
		events.TypeExecutionOutput:
		return false
	default:
		return true
	}
}

func shouldSyncCostDialogForEvent(event events.Event) bool {
	switch event.Type {
	case events.TypeSessionConfigured,
		events.TypeSessionModelRouteUpdated,
		events.TypeSessionWorkspaceRootsAdded,
		events.TypeSessionPermissionModeUpdated,
		events.TypeSessionHistoryCheckpoint,
		events.TypeSessionStateSnapshot,
		events.TypeSessionTitleUpdated,
		events.TypePromptCompiled,
		events.TypeAssistantPreviewDelta,
		events.TypeAssistantPreviewReset,
		events.TypeAssistantWorklogCommit,
		events.TypeAssistantCommit,
		events.TypeReasoningDelta,
		events.TypeTurnWorkStateUpdated,
		events.TypeToolCallDelta,
		events.TypeToolExecOutput,
		events.TypeExecutionOutput,
		events.TypeExecutionBackgroundObserved,
		events.TypePermissionRequested,
		events.TypePermissionResolved,
		events.TypeQuestionRequested,
		events.TypeQuestionAnswered,
		events.TypeTaskCreated,
		events.TypeTaskProgressUpdated,
		events.TypeTaskBlocked,
		events.TypeTaskCompleted,
		events.TypeTaskReviewed,
		events.TypeWorkspaceWriteRestored:
		return false
	default:
		return true
	}
}

func shouldSyncTraceDialogForEvent(event events.Event, turnID string) bool {
	if strings.TrimSpace(turnID) == "" || event.TurnID != strings.TrimSpace(turnID) {
		return false
	}
	switch event.Type {
	case events.TypeSessionConfigured,
		events.TypeSessionModelRouteUpdated,
		events.TypeSessionWorkspaceRootsAdded,
		events.TypeSessionPermissionModeUpdated,
		events.TypeSessionHistoryCheckpoint,
		events.TypeSessionStateSnapshot,
		events.TypeSessionTitleUpdated,
		events.TypeAssistantPreviewDelta,
		events.TypeAssistantPreviewReset,
		events.TypeAssistantWorklogCommit,
		events.TypeAssistantCommit,
		events.TypeReasoningDelta,
		events.TypeTurnWorkStateUpdated,
		events.TypePermissionRequested,
		events.TypePermissionResolved,
		events.TypeExecutionApprovalRequested,
		events.TypeExecutionApprovalResolved,
		events.TypeExecutionBackgroundStarted,
		events.TypeExecutionBackgroundObserved,
		events.TypeExecutionBackgroundReady,
		events.TypeTaskCreated,
		events.TypeTaskProgressUpdated,
		events.TypeTaskBlocked,
		events.TypeTaskCompleted,
		events.TypeTaskReviewed,
		events.TypeWorkspaceWriteRestored:
		return false
	default:
		return true
	}
}

func shouldSyncToolDetailDialogForEvent(event events.Event, ref sessionToolCallRef) bool {
	if strings.TrimSpace(ref.TurnID) == "" || strings.TrimSpace(ref.CallID) == "" || event.TurnID != strings.TrimSpace(ref.TurnID) {
		return false
	}
	switch event.Type {
	case events.TypeToolCallDelta,
		events.TypeToolCallDeclared,
		events.TypeToolExecStart,
		events.TypeToolExecOutput,
		events.TypeToolExecEnd,
		events.TypeExecutionDeclared,
		events.TypeExecutionStarted,
		events.TypeExecutionOutput,
		events.TypeExecutionBackgroundExited,
		events.TypeExecutionBackgroundLost,
		events.TypeQuestionRequested,
		events.TypeQuestionAnswered:
		return eventAffectsToolCall(event, ref.CallID)
	default:
		return false
	}
}

func shouldSyncTaskDetailDialogForEvent(event events.Event, taskID string) bool {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return false
	}
	switch payload := event.Payload.(type) {
	case events.TaskCreatedPayload:
		return strings.TrimSpace(payload.TaskID) == taskID
	case events.TaskProgressUpdatedPayload:
		return strings.TrimSpace(payload.TaskID) == taskID
	case events.TaskBlockedPayload:
		return strings.TrimSpace(payload.TaskID) == taskID
	case events.TaskCompletedPayload:
		return strings.TrimSpace(payload.TaskID) == taskID
	case events.TaskReviewedPayload:
		return strings.TrimSpace(payload.TaskID) == taskID
	default:
		return false
	}
}

func shouldSyncHandoffDetailDialogForEvent(event events.Event, target inspectorHandoffTarget) bool {
	sessionID := normalizeToolTargetSessionID("", target.SessionID)
	turnID := strings.TrimSpace(target.TurnID)
	handoffID := strings.TrimSpace(target.HandoffID)
	if sessionID == "" || turnID == "" || handoffID == "" {
		return false
	}
	if strings.TrimSpace(event.SessionID) != sessionID || strings.TrimSpace(event.TurnID) != turnID {
		return false
	}
	switch payload := event.Payload.(type) {
	case events.AgentHandoffPayload:
		return strings.TrimSpace(payload.HandoffID) == handoffID
	case events.AgentHandoffPreviewPayload:
		return strings.TrimSpace(payload.HandoffID) == handoffID
	case events.AgentResultPayload:
		return strings.TrimSpace(payload.HandoffID) == handoffID
	case events.AgentResultReusedPayload:
		return strings.TrimSpace(payload.HandoffID) == handoffID
	default:
		return false
	}
}

func eventAffectsToolCall(event events.Event, callID string) bool {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		return false
	}
	switch payload := event.Payload.(type) {
	case events.ToolCallDeltaPayload:
		return payload.CallID == callID
	case events.ToolCallDeclaredPayload:
		return payload.CallID == callID
	case events.ToolExecStartPayload:
		return payload.CallID == callID
	case events.ToolExecOutputPayload:
		return payload.CallID == callID
	case events.ToolExecEndPayload:
		return payload.CallID == callID
	case events.ExecutionDeclaredPayload:
		return payload.ToolCallID == callID
	case events.ExecutionStartedPayload:
		return payload.ToolCallID == callID
	case events.ExecutionOutputPayload:
		return payload.ToolCallID == callID
	case events.ExecutionBackgroundStartedPayload:
		return payload.ToolCallID == callID
	case events.ExecutionBackgroundObservedPayload:
		return payload.ToolCallID == callID
	case events.ExecutionBackgroundReadyPayload:
		return payload.ToolCallID == callID
	case events.ExecutionBackgroundExitedPayload:
		return payload.ToolCallID == callID
	case events.ExecutionBackgroundLostPayload:
		return payload.ToolCallID == callID
	case events.QuestionRequestedPayload:
		return payload.ToolCallID == callID
	case events.QuestionAnsweredPayload:
		return payload.ToolCallID == callID
	default:
		return false
	}
}
