package events

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Type string

const (
	TypeSessionConfigured            Type = "session_configured"
	TypeSessionModelRouteUpdated     Type = "session_model_route_updated"
	TypeSessionWorkspaceRootsAdded   Type = "session_workspace_roots_added"
	TypeSessionPermissionModeUpdated Type = "session_permission_mode_updated"
	TypeSessionMCPCatalogUpdated     Type = "session_mcp_catalog_updated"
	TypeSessionProviderLimitUpdated  Type = "session_provider_limit_updated"
	TypeSessionStateSnapshot         Type = "session_state_snapshot"
	TypeAssistantPreviewDelta        Type = "assistant_preview_delta"
	TypeAssistantPreviewReset        Type = "assistant_preview_reset"
	TypeAssistantWorklogCommit       Type = "assistant_worklog_commit"
	TypeAssistantCommit              Type = "assistant_commit"
	TypeReasoningDelta               Type = "reasoning_delta"
	TypeAnthropicThinkingCommitted   Type = "anthropic_thinking_committed"
	TypeOpenAIReasoningCommitted     Type = "openai_reasoning_committed"
	TypeToolCallDelta                Type = "tool_call_delta"
	TypeToolCallDeclared             Type = "tool_call_declared"
	TypeToolCallBatch                Type = "tool_call_batch"
	TypeToolExecStart                Type = "tool_exec_start"
	TypeToolExecOutput               Type = "tool_exec_output"
	TypeToolExecEnd                  Type = "tool_exec_end"
	TypeExecutionDeclared            Type = "execution_declared"
	TypeExecutionApprovalRequested   Type = "execution_approval_requested"
	TypeExecutionApprovalResolved    Type = "execution_approval_resolved"
	TypeExecutionStarted             Type = "execution_started"
	TypeExecutionOutput              Type = "execution_output"
	TypeExecutionBackgroundStarted   Type = "execution_background_started"
	TypeExecutionBackgroundObserved  Type = "execution_background_observed"
	TypeExecutionBackgroundReady     Type = "execution_background_ready"
	TypeExecutionBackgroundExited    Type = "execution_background_exited"
	TypeExecutionBackgroundLost      Type = "execution_background_lost"
	TypePermissionRequested          Type = "permission_requested"
	TypePermissionResolved           Type = "permission_resolved"
	TypeQuestionRequested            Type = "question_requested"
	TypeQuestionAnswered             Type = "question_answered"
	TypeTurnConfigured               Type = "turn_configured"
	TypeTurnContinuationStarted      Type = "turn_continuation_started"
	TypeTurnWorkStateUpdated         Type = "turn_work_state_updated"
	TypeSessionHistoryCheckpoint     Type = "session_history_checkpoint"
	TypeContextCompactionStarted     Type = "context_compaction_started"
	TypeContextCompactionFailed      Type = "context_compaction_failed"
	TypeTurnProviderUsageRecorded    Type = "turn_provider_usage_recorded"
	TypeTurnProviderUsageReported    Type = "turn_provider_usage_reported"
	TypeTurnRetryScheduled           Type = "turn_retry_scheduled"
	TypeReviewRecorded               Type = "review_recorded"
	TypePlanRecorded                 Type = "plan_recorded"
	TypeTurnDone                     Type = "turn_done"
	TypeTurnCanceled                 Type = "turn_canceled"
	TypeTurnError                    Type = "turn_error"
)

type Payload interface {
	eventType() Type
	validate() error
}

type Draft struct {
	SessionID string
	TurnID    string
	Type      Type
	Payload   Payload
}

func (d Draft) Validate() error {
	if strings.TrimSpace(d.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(d.TurnID) == "" {
		return errors.New("turn_id is required")
	}
	if d.Type == "" {
		return errors.New("type is required")
	}
	if d.Payload == nil {
		return errors.New("payload is required")
	}
	if got, want := d.Payload.eventType(), d.Type; got != want {
		return fmt.Errorf("payload type mismatch: got %q want %q", got, want)
	}
	return d.Payload.validate()
}

type Event struct {
	ID        string
	SessionID string
	TurnID    string
	Sequence  int64
	Time      time.Time
	Type      Type
	Payload   Payload
	Ephemeral bool
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.SessionID) == "" {
		return errors.New("session_id is required")
	}
	if strings.TrimSpace(e.TurnID) == "" {
		return errors.New("turn_id is required")
	}
	if e.Sequence < 0 {
		return errors.New("sequence must be >= 0")
	}
	if e.Time.IsZero() {
		return errors.New("time is required")
	}
	if e.Type == "" {
		return errors.New("type is required")
	}
	if e.Payload == nil {
		return errors.New("payload is required")
	}
	if got, want := e.Payload.eventType(), e.Type; got != want {
		return fmt.Errorf("payload type mismatch: got %q want %q", got, want)
	}
	return e.Payload.validate()
}
