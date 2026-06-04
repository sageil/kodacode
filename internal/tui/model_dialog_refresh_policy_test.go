package tui

import (
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestTurnWorkStateUpdatedDoesNotRefreshVisibleSurfaces(t *testing.T) {
	event := events.Event{
		TurnID: "turn-1",
		Type:   events.TypeTurnWorkStateUpdated,
	}

	if shouldSyncTranscriptForEvent(event) {
		t.Fatal("turn work state update should not refresh transcript")
	}
	if shouldSyncInspectorForEvent(event) {
		t.Fatal("turn work state update should not refresh inspector")
	}
	if shouldSyncCostDialogForEvent(event) {
		t.Fatal("turn work state update should not refresh cost dialog")
	}
	if shouldSyncTraceDialogForEvent(event, "turn-1") {
		t.Fatal("turn work state update should not refresh trace dialog")
	}
}

func TestShouldSyncCostDialogForEvent(t *testing.T) {
	tests := []struct {
		name  string
		event events.Event
		want  bool
	}{
		{
			name: "assistant preview delta",
			event: events.Event{
				Type:    events.TypeAssistantPreviewDelta,
				Payload: events.AssistantPreviewDeltaPayload{Content: "stream"},
			},
			want: false,
		},
		{
			name: "tool exec output",
			event: events.Event{
				Type:    events.TypeToolExecOutput,
				Payload: events.ToolExecOutputPayload{CallID: "call-1", Chunk: "stdout"},
			},
			want: false,
		},
		{
			name: "provider usage recorded",
			event: events.Event{
				Type:    events.TypeTurnProviderUsageRecorded,
				Payload: events.TurnProviderUsageRecordedPayload{Model: "openai/gpt-5-mini", Step: 1, Attempt: 1, EstimatedRequestTokens: 120},
			},
			want: true,
		},
		{
			name: "task progress update",
			event: events.Event{
				Type: events.TypeTaskProgressUpdated,
				Payload: events.TaskProgressUpdatedPayload{
					TaskID:   "task-1",
					Progress: "checking refresh policy",
				},
			},
			want: false,
		},
		{
			name: "session title update",
			event: events.Event{
				Type: events.TypeSessionTitleUpdated,
				Payload: events.SessionTitleUpdatedPayload{
					Title: "trace dialog cleanup",
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncCostDialogForEvent(tt.event); got != tt.want {
				t.Fatalf("shouldSyncCostDialogForEvent(%q) = %v, want %v", tt.event.Type, got, tt.want)
			}
		})
	}
}

func TestShouldSyncTraceDialogForEvent(t *testing.T) {
	tests := []struct {
		name   string
		turnID string
		event  events.Event
		want   bool
	}{
		{
			name:   "assistant preview delta on traced turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID:  "turn-1",
				Type:    events.TypeAssistantPreviewDelta,
				Payload: events.AssistantPreviewDeltaPayload{Content: "stream"},
			},
			want: false,
		},
		{
			name:   "prompt compiled on traced turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypePromptCompiled,
				Payload: events.PromptCompiledPayload{
					Shape:        "generic",
					Instructions: "compiled",
				},
			},
			want: true,
		},
		{
			name:   "provider usage on different turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID:  "turn-2",
				Type:    events.TypeTurnProviderUsageRecorded,
				Payload: events.TurnProviderUsageRecordedPayload{Model: "openai/gpt-5-mini", Step: 1, Attempt: 1, EstimatedRequestTokens: 120},
			},
			want: false,
		},
		{
			name:   "workflow evidence on traced turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeWorkflowEvidenceRecorded,
				Payload: events.WorkflowEvidenceRecordedPayload{
					EvidenceID: "evidence-1",
					WorkflowID: "delivery",
					PhaseID:    "verify",
					Type:       events.WorkflowEvidenceTypeVerificationResult,
				},
			},
			want: true,
		},
		{
			name:   "background observed on traced turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeExecutionBackgroundObserved,
				Payload: events.ExecutionBackgroundObservedPayload{
					ExecutionID: "exec-1",
					ToolCallID:  "call-1",
					ToolName:    "bash",
					OutputTail:  "tail",
					OutputBytes: 42,
				},
			},
			want: false,
		},
		{
			name:   "background exited on traced turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeExecutionBackgroundExited,
				Payload: events.ExecutionBackgroundExitedPayload{
					ExecutionID: "exec-1",
					ToolCallID:  "call-1",
					ToolName:    "bash",
				},
			},
			want: true,
		},
		{
			name:   "task created on traced turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeTaskCreated,
				Payload: events.TaskCreatedPayload{
					TaskID: "task-1",
					Title:  "inspect refresh policy",
					Status: events.TaskStatusPending,
				},
			},
			want: false,
		},
		{
			name:   "workspace write restored on traced turn",
			turnID: "turn-1",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeWorkspaceWriteRestored,
				Payload: events.WorkspaceWriteRestoredPayload{
					SourceTurnID: "turn-0",
					Restores: []events.WorkspaceWriteRestoreItem{{
						CallID: "call-1",
						Path:   "/repo/app.go",
					}},
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncTraceDialogForEvent(tt.event, tt.turnID); got != tt.want {
				t.Fatalf("shouldSyncTraceDialogForEvent(%q, %q) = %v, want %v", tt.event.Type, tt.turnID, got, tt.want)
			}
		})
	}
}

func TestShouldSyncToolDetailDialogForEvent(t *testing.T) {
	ref := sessionToolCallRef{TurnID: "turn-1", CallID: "call-1"}
	tests := []struct {
		name  string
		event events.Event
		want  bool
	}{
		{
			name: "assistant preview delta",
			event: events.Event{
				TurnID:  "turn-1",
				Type:    events.TypeAssistantPreviewDelta,
				Payload: events.AssistantPreviewDeltaPayload{Content: "stream"},
			},
			want: false,
		},
		{
			name: "execution output for selected call",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeExecutionOutput,
				Payload: events.ExecutionOutputPayload{
					ExecutionID: "exec-1",
					ToolCallID:  "call-1",
					Chunk:       "stdout",
				},
			},
			want: true,
		},
		{
			name: "execution output for different call",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeExecutionOutput,
				Payload: events.ExecutionOutputPayload{
					ExecutionID: "exec-2",
					ToolCallID:  "call-2",
					Chunk:       "stdout",
				},
			},
			want: false,
		},
		{
			name: "question answered for selected call",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeQuestionAnswered,
				Payload: events.QuestionAnsweredPayload{
					QuestionID: "question-1",
					ToolCallID: "call-1",
					Answer:     "yes",
				},
			},
			want: true,
		},
		{
			name: "background observed for selected call",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeExecutionBackgroundObserved,
				Payload: events.ExecutionBackgroundObservedPayload{
					ExecutionID: "exec-1",
					ToolCallID:  "call-1",
					ToolName:    "bash",
					OutputTail:  "tail",
					OutputBytes: 42,
				},
			},
			want: false,
		},
		{
			name: "background exited for selected call",
			event: events.Event{
				TurnID: "turn-1",
				Type:   events.TypeExecutionBackgroundExited,
				Payload: events.ExecutionBackgroundExitedPayload{
					ExecutionID: "exec-1",
					ToolCallID:  "call-1",
					ToolName:    "bash",
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldSyncToolDetailDialogForEvent(tt.event, ref); got != tt.want {
				t.Fatalf("shouldSyncToolDetailDialogForEvent(%q) = %v, want %v", tt.event.Type, got, tt.want)
			}
		})
	}
}
