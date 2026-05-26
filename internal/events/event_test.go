package events

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDraftValidateAcceptsMatchingPayload(t *testing.T) {
	ev := Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantPreviewDelta,
		Payload:   AssistantPreviewDeltaPayload{Content: "hello"},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestDraftValidateRejectsPayloadTypeMismatch(t *testing.T) {
	ev := Draft{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantCommit,
		Payload:   AssistantPreviewDeltaPayload{Content: "hello"},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want mismatch error")
	}
}

func TestEventValidateRequiresPersistedMetadata(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      TypeAssistantCommit,
		Payload:   AssistantCommitPayload{Content: "done"},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want persisted metadata error")
	}
}

func TestToolExecEndPayloadAllowsEmptySuccessfulResult(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "list",
			Succeeded: true,
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestToolExecEndPayloadRejectsEmptyFailedResult(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "bash",
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestToolExecEndPayloadAllowsStructuredErrorDetail(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:   "call-1",
			ToolName: "apply_patch",
			Error:    "read notes.txt again, then retry apply_patch",
			ErrorDetail: &ToolErrorDetail{
				Code:      "apply_patch_requires_fresh_read_after_failure",
				Message:   "read notes.txt again, then retry apply_patch",
				Retryable: true,
				Recovery:  "read_then_retry",
				Fields: map[string]string{
					"path": "notes.txt",
				},
			},
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestToolExecEndPayloadAllowsExecutionSummaryFields(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:          "call-1",
			ToolName:        "bash",
			ExecutionID:     "exec-1",
			ExecutionStatus: string(ExecutionStatusCompleted),
			Succeeded:       true,
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestToolExecEndPayloadRejectsExecutionIDWithoutExecutionStatus(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:      "call-1",
			ToolName:    "bash",
			ExecutionID: "exec-1",
			Succeeded:   true,
		},
	}

	if err := ev.Validate(); err == nil || !strings.Contains(err.Error(), "execution_status is required") {
		t.Fatalf("Validate() error = %v, want execution_status required", err)
	}
}

func TestToolExecEndPayloadAllowsWriteMutation(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:    "call-1",
			ToolName:  "write",
			Succeeded: true,
			WriteMutation: &WriteMutation{
				Path:    "/repo/notes.txt",
				Existed: true,
				Before:  "before\n",
				Mode:    0o644,
			},
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestToolExecEndPayloadRejectsInvalidWriteMutation(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:        "call-1",
			ToolName:      "write",
			Succeeded:     true,
			WriteMutation: &WriteMutation{},
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestToolExecEndPayloadRejectsInvalidStructuredResult(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeToolExecEnd,
		Payload: ToolExecEndPayload{
			CallID:           "call-1",
			ToolName:         "search",
			Succeeded:        true,
			StructuredResult: json.RawMessage(`{"broken"`),
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestSessionHistoryCheckpointAllowsEmptySuccessfulToolResult(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "_session",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeSessionHistoryCheckpoint,
		Payload: SessionHistoryCheckpointPayload{
			ThroughSequence: 0,
			Turns: []SessionHistoryTurnPayload{{
				TurnID:              "turn-1",
				UserText:            "inspect templates",
				ToolCalls:           []SessionHistoryToolCallPayload{{CallID: "call-1", ToolName: "list", Arguments: `{"path":"src/routes/templates","include_hidden":false}`}},
				ToolResults:         []SessionHistoryToolResultPayload{{CallID: "call-1", ToolName: "list", Succeeded: true}},
				EntryOrder:          []SessionHistoryEntryPayload{{Kind: "user_message", Index: 0}, {Kind: "tool_call", Index: 0}, {Kind: "tool_result", Index: 0}},
				TerminalStatus:      "completed",
				ToolCallCount:       1,
				SuccessfulToolCalls: 1,
			}},
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSessionHistoryCheckpointAllowsBlobBackedFailedToolResult(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "_session",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeSessionHistoryCheckpoint,
		Payload: SessionHistoryCheckpointPayload{
			ThroughSequence: 0,
			Turns: []SessionHistoryTurnPayload{{
				TurnID:          "turn-1",
				UserText:        "fetch docs",
				ToolCalls:       []SessionHistoryToolCallPayload{{CallID: "call-1", ToolName: "web_fetch", Arguments: `{"url":"https://example.com","method":"GET","headers":null,"body":null,"format":"text","selector":null,"timeout":5000}`}},
				ToolResults:     []SessionHistoryToolResultPayload{{CallID: "call-1", ToolName: "web_fetch", Succeeded: false, ErrorBlob: &ToolResultBlobRef{Ref: "session-1/turn-1/call-1-error.txt", Bytes: 5000}}},
				EntryOrder:      []SessionHistoryEntryPayload{{Kind: "user_message", Index: 0}, {Kind: "tool_call", Index: 0}, {Kind: "tool_result", Index: 0}},
				TerminalStatus:  "completed",
				ToolCallCount:   1,
				FailedToolCalls: 1,
				FailedToolNames: []string{"web_fetch"},
			}},
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestExecutionDeclaredPayloadRequiresCommand(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeExecutionDeclared,
		Payload: ExecutionDeclaredPayload{
			ExecutionID:      "exec-call-1",
			ToolCallID:       "call-1",
			ToolName:         "bash",
			Kind:             "bash",
			WorkingDirectory: "/repo",
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestExecutionApprovalRequestedPayloadRequiresWorkingDirectory(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeExecutionApprovalRequested,
		Payload: ExecutionApprovalRequestedPayload{
			RequestID:   "perm-1",
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			ToolName:    "bash",
			Command:     "npm test",
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestExecutionBackgroundObservedPayloadRequiresToolName(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeExecutionBackgroundObserved,
		Payload: ExecutionBackgroundObservedPayload{
			ExecutionID: "exec-call-1",
			ToolCallID:  "call-1",
			OutputTail:  "listening on 127.0.0.1:5173",
			OutputBytes: 29,
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestQuestionRequestedPayloadAllowsRuntimeOwnedQuestion(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeQuestionRequested,
		Payload: QuestionRequestedPayload{
			QuestionID: "q-1",
			Question:   "Continue or stop?",
			Options:    []string{"Continue", "Stop turn"},
			Purpose:    QuestionPurposeTurnLoopResolution,
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestQuestionRequestedPayloadRejectsHalfBoundToolReference(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeQuestionRequested,
		Payload: QuestionRequestedPayload{
			QuestionID: "q-1",
			ToolName:   "read",
			Question:   "Continue or stop?",
			Options:    []string{"Continue", "Stop turn"},
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestPermissionResolvedPayloadRequiresScopeForApproval(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypePermissionResolved,
		Payload: PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  PermissionDecisionApproved,
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestPermissionResolvedPayloadAllowsOnceApprovalWithoutGrant(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypePermissionResolved,
		Payload: PermissionResolvedPayload{
			RequestID: "perm-1",
			Decision:  PermissionDecisionApproved,
			Scope:     PermissionScopeOnce,
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestExecutionApprovalResolvedPayloadAllowsOnceApprovalWithoutGrant(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeExecutionApprovalResolved,
		Payload: ExecutionApprovalResolvedPayload{
			RequestID: "perm-1",
			Decision:  ExecutionApprovalDecisionAccept,
		},
	}

	if err := ev.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestContextCompactionStartedPayloadRequiresScopeAndTriggerMetrics(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeContextCompactionStarted,
		Payload: ContextCompactionStartedPayload{
			Scope:                  CompactionScopeHistory,
			InputLimitTokens:       0,
			TriggerTokens:          60,
			TargetTokens:           40,
			EstimatedRequestTokens: 100,
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}

func TestContextCompactionFailedPayloadRequiresMinimalSharedSchema(t *testing.T) {
	ev := Event{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Sequence:  0,
		Time:      time.Now().UTC(),
		Type:      TypeContextCompactionFailed,
		Payload: ContextCompactionFailedPayload{
			Scope:                  CompactionScopeHistory,
			Reason:                 "artifact_generation_failed",
			Detail:                 "",
			InputLimitTokens:       2048,
			TriggerTokens:          1600,
			TargetTokens:           1300,
			EstimatedRequestTokens: 2200,
		},
	}

	if err := ev.Validate(); err == nil {
		t.Fatal("Validate() error = nil, want validation error")
	}
}
