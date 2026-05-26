package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestRuntimeRunSessionTurnWithinProviderRequestLimitDoesNotBlockActiveTask(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"]`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Runner.maxProviderRequestsPerTurn = 3

	sessionID, taskID := createRuntimeWorkflowTask(t, runtime, root, "Resume implementation", events.TaskStatusInProgress)
	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "keep reading until done",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}
	if result.TurnID != "turn-1" {
		t.Fatalf("result turn id = %q, want original turn", result.TurnID)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	task := state.Tasks[taskID]
	if task == nil {
		t.Fatalf("task %q missing", taskID)
	}
	if task.Status != events.TaskStatusInProgress {
		t.Fatalf("task status = %q, want in_progress", task.Status)
	}
	if task.BlockReason != "" {
		t.Fatalf("task block reason = %q, want empty", task.BlockReason)
	}
}

func TestRuntimeRunSessionTurnAsksBeforeBlockingActiveTaskAfterProviderRequestLimit(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":1,"limit":1`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":2,"limit":1`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-4", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":3,"limit":1`},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-4", ToolName: "read", InputDelta: `}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-4", ToolName: "read"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Runner.maxProviderRequestsPerTurn = 3

	sessionID, taskID := createRuntimeWorkflowTask(t, runtime, root, "Resume implementation", events.TaskStatusInProgress)
	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "keep reading until done",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingQuestion == nil {
		t.Fatalf("result = %#v", result)
	}
	if result.PendingQuestion.Purpose != events.QuestionPurposeTurnLoopResolution ||
		!questionOptionAllowed(result.PendingQuestion.Options, loopResolutionAnswerBlock) {
		t.Fatalf("pending question = %#v", result.PendingQuestion)
	}
	if questionOptionAllowed(result.PendingQuestion.Options, providerRequestLimitAnswerAllowSessionYOLO) {
		t.Fatalf("pending question unexpectedly allows per-session provider limit disable: %#v", result.PendingQuestion)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	task := state.Tasks[taskID]
	if task == nil {
		t.Fatalf("task %q missing", taskID)
	}
	if task.Status != events.TaskStatusInProgress {
		t.Fatalf("task status = %q, want in_progress", task.Status)
	}
	if task.BlockReason != "" {
		t.Fatalf("task block reason = %q, want empty", task.BlockReason)
	}
	if len(state.TurnOrder) != 1 {
		t.Fatalf("turn order = %#v, want one pending turn", state.TurnOrder)
	}
}

func TestRuntimeAnswerSessionQuestionDisablesProviderRequestLimitForReviewPlanSession(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nline2\nline3\nline4\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":2,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-4", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":3,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-4", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Runner.SetSessionConfig(SessionConfig{
		MaxProviderRequestsPerTurn: 2,
		MaxRetries:                 defaultProviderRetryAttempts,
	})

	sessionID, taskID := createRuntimeWorkflowTaskWithID(t, runtime, root, "task-1", "Address review findings", events.TaskStatusInProgress)
	if _, err := runtime.Sessions.ReviewTask(context.Background(), ReviewTaskInput{
		SessionID:     sessionID,
		TurnID:        "turn-review",
		TaskID:        taskID,
		ReviewStatus:  events.TaskReviewStatusFail,
		ReviewSummary: "needs another implementation pass",
	}); err != nil {
		t.Fatalf("ReviewTask() error = %v", err)
	}

	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "keep reading until done",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}
	if !questionOptionAllowed(first.PendingQuestion.Options, providerRequestLimitAnswerAllowOnce) ||
		!questionOptionAllowed(first.PendingQuestion.Options, loopResolutionAnswerStop) ||
		!questionOptionAllowed(first.PendingQuestion.Options, providerRequestLimitAnswerAllowSessionYOLO) ||
		questionOptionAllowed(first.PendingQuestion.Options, loopResolutionAnswerBlock) {
		t.Fatalf("pending question options = %#v", first.PendingQuestion.Options)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    providerRequestLimitAnswerAllowSessionYOLO,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if len(client.requests) != 5 {
		t.Fatalf("provider requests = %d, want 5", len(client.requests))
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if !state.ProviderRequestLimitDisabled {
		t.Fatal("provider request limit disabled = false, want true")
	}
}

func TestRuntimeAnswerSessionQuestionDisablesProviderRequestLimitForEngineerReviewPlanExecuteWorkflowWithoutTask(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\nline2\nline3\nline4\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":0,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":1,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "read", InputDelta: `{"paths":["app.go"],"offset":2,"limit":1}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "read"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)
	runtime.Runner.SetSessionConfig(SessionConfig{
		MaxProviderRequestsPerTurn: 2,
		MaxRetries:                 defaultProviderRetryAttempts,
	})

	first, err := runtime.RunSessionTurn(context.Background(), RunSessionInput{
		WorkspaceRoot: root,
		UserText:      "Perform a complete performance review and create a detailed plan of execution",
		AgentID:       "engineer",
	})
	if err != nil {
		t.Fatalf("RunSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}
	if !questionOptionAllowed(first.PendingQuestion.Options, providerRequestLimitAnswerAllowOnce) ||
		!questionOptionAllowed(first.PendingQuestion.Options, loopResolutionAnswerStop) ||
		!questionOptionAllowed(first.PendingQuestion.Options, providerRequestLimitAnswerAllowSessionYOLO) ||
		questionOptionAllowed(first.PendingQuestion.Options, loopResolutionAnswerBlock) {
		t.Fatalf("pending question options = %#v", first.PendingQuestion.Options)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    providerRequestLimitAnswerAllowSessionYOLO,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCompleted {
		t.Fatalf("second result = %#v", second)
	}
	if len(client.requests) != 4 {
		t.Fatalf("provider requests = %d, want 4", len(client.requests))
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), first.SessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if activeWorkflowTask(state) != nil {
		t.Fatalf("active workflow task = %#v, want none", activeWorkflowTask(state))
	}
	if !state.ProviderRequestLimitDisabled {
		t.Fatal("provider request limit disabled = false, want true")
	}
}

func TestRuntimeRunSessionTurnPausesBeforeBlockingActiveTaskAfterRepeatedToolLoop(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{streams: repeatedInvalidWriteStreams(5, "notes.md")}
	runtime := newRuntimeWithClient(t, client)

	sessionID, taskID := createRuntimeWorkflowTask(t, runtime, root, "Resume implementation", events.TaskStatusInProgress)
	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "keep reading until done",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusPending || result.PendingRequestID == "" || result.PendingQuestion == nil {
		t.Fatalf("result = %#v", result)
	}
	if result.PendingQuestion.Purpose != events.QuestionPurposeTurnLoopResolution {
		t.Fatalf("pending question = %#v", result.PendingQuestion)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	task := state.Tasks[taskID]
	if task == nil {
		t.Fatalf("task %q missing", taskID)
	}
	if task.Status != events.TaskStatusInProgress {
		t.Fatalf("task status = %q, want in_progress", task.Status)
	}
	if task.BlockReason != "" {
		t.Fatalf("task block reason = %q, want empty", task.BlockReason)
	}
	if len(state.TurnOrder) != 1 {
		t.Fatalf("turn order = %#v, want one pending turn", state.TurnOrder)
	}
}

func TestRuntimeAnswerSessionQuestionBlocksActiveTaskAfterLoopPause(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{streams: repeatedInvalidWriteStreams(5, "notes.md")}
	runtime := newRuntimeWithClient(t, client)

	sessionID, taskID := createRuntimeWorkflowTask(t, runtime, root, "Resume implementation", events.TaskStatusInProgress)
	first, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "keep reading until done",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if first.Status != TurnRunStatusPending || first.PendingRequestID == "" || first.PendingQuestion == nil {
		t.Fatalf("first result = %#v", first)
	}

	second, err := runtime.AnswerSessionQuestion(context.Background(), AnswerSessionQuestionInput{
		SessionID: first.SessionID,
		TurnID:    first.TurnID,
		RequestID: first.PendingRequestID,
		Answer:    loopResolutionAnswerBlock,
	})
	if err != nil {
		t.Fatalf("AnswerSessionQuestion() error = %v", err)
	}
	if second.Status != TurnRunStatusCanceled {
		t.Fatalf("second result = %#v", second)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	task := state.Tasks[taskID]
	if task == nil {
		t.Fatalf("task %q missing", taskID)
	}
	if task.Status != events.TaskStatusBlocked {
		t.Fatalf("task status = %q, want blocked", task.Status)
	}
	if task.BlockReason != taskBlockReasonExecutionStalledNoProgress {
		t.Fatalf("task block reason = %q", task.BlockReason)
	}
}

func TestRuntimeRunSessionTurnDoesNotBlockActiveTaskForSerialDistinctExploration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "app.go")
	if err := os.WriteFile(path, []byte("package main\n// package\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: "search", InputDelta: `{"path":".","query":"package","max_matches":1,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: "search", InputDelta: `{"path":".","query":"package","max_matches":2,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: "search", InputDelta: `{"path":".","query":"package","max_matches":3,"case_sensitive":false,"mode":"lexical"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: "search"},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "done"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, taskID := createRuntimeWorkflowTask(t, runtime, root, "Resume implementation", events.TaskStatusInProgress)
	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "keep reading until done",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	task := state.Tasks[taskID]
	if task == nil {
		t.Fatalf("task %q missing", taskID)
	}
	if task.Status != events.TaskStatusInProgress {
		t.Fatalf("task status = %q, want in_progress", task.Status)
	}
	if task.BlockReason != "" {
		t.Fatalf("task block reason = %q, want empty", task.BlockReason)
	}
}

func TestRuntimeRunSessionTurnRejectsTaskCompletionWithoutRecordedWork(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"complete","task_id":"task-1","summary":"done"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "Task completed."},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, taskID := createRuntimeWorkflowTaskWithID(t, runtime, root, "task-1", "Finish the task honestly", events.TaskStatusInProgress)
	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "finish the task",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	task := state.Tasks[taskID]
	if task == nil {
		t.Fatalf("task %q missing", taskID)
	}
	if task.Status != events.TaskStatusCompleted {
		t.Fatalf("task status = %q, want completed", task.Status)
	}
	call := state.Turns["turn-1"].ToolCalls["call-1"]
	if call == nil {
		t.Fatalf("tool call = %#v", call)
	}
	if got := call.Error; got != "" {
		t.Fatalf("tool call error = %q", got)
	}
	if got := state.Turns["turn-1"].AssistantText; got != "Task completed." {
		t.Fatalf("assistant text = %q", got)
	}
	if result.AssistantText != state.Turns["turn-1"].AssistantText {
		t.Fatalf("result assistant text = %q, want %q", result.AssistantText, state.Turns["turn-1"].AssistantText)
	}
}

func TestRuntimeRunSessionTurnKeepsParentInProgressUntilChildrenComplete(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","task_id":"task-103","parent_task_id":"task-9","title":"Apply Backend Performance Optimizations","kind":"backend"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","task_id":"task-107","parent_task_id":"task-9","title":"Apply Frontend Performance Optimizations","kind":"frontend"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"update","task_id":"task-9","status":"complete","progress":"Performance review complete. Backend and frontend follow-up tasks created."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-4", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"update","task_id":"task-103","status":"in_progress","progress":"Starting backend optimizations."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-4", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: "started backend work"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, parentTaskID := createRuntimeWorkflowTaskWithID(t, runtime, root, "task-9", "Project Performance Review and Recommendations", events.TaskStatusInProgress)
	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "lets apply all",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	state, err := runtime.Sessions.Snapshot(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	parent := state.Tasks[parentTaskID]
	if parent == nil {
		t.Fatalf("parent task missing")
	}
	if parent.Status != events.TaskStatusInProgress {
		t.Fatalf("parent task = %#v", parent)
	}
	if parent.Progress != "" {
		t.Fatalf("parent progress = %q, want empty", parent.Progress)
	}
	if len(state.TaskOrder) != 3 {
		t.Fatalf("task order = %#v, want parent plus two follow-up tasks", state.TaskOrder)
	}
	var backend, frontend *events.TaskState
	for _, taskID := range state.TaskOrder {
		task := state.Tasks[taskID]
		if task == nil {
			continue
		}
		switch task.Title {
		case "Apply Backend Performance Optimizations":
			backend = task
		case "Apply Frontend Performance Optimizations":
			frontend = task
		}
	}
	if backend == nil || frontend == nil {
		t.Fatalf("tasks = %#v", state.Tasks)
	}
	if backend.Status != events.TaskStatusInProgress || backend.Progress != "Starting backend optimizations." {
		t.Fatalf("backend task = %#v", backend)
	}
	if frontend.Status != events.TaskStatusPending {
		t.Fatalf("frontend task = %#v", frontend)
	}
	for _, callID := range []string{"call-1", "call-2", "call-4"} {
		call := state.Turns["turn-1"].ToolCalls[callID]
		if call == nil || !call.Succeeded || call.Error != "" {
			t.Fatalf("tool call %s = %#v", callID, call)
		}
	}
	call := state.Turns["turn-1"].ToolCalls["call-3"]
	if call == nil || call.Succeeded || call.Error != "task_workflow failed: complete child tasks before completing the parent." {
		t.Fatalf("tool call call-3 = %#v", call)
	}
	if got := state.Turns["turn-1"].AssistantText; got != "started backend work" {
		t.Fatalf("assistant text = %q", got)
	}
	if result.AssistantText != state.Turns["turn-1"].AssistantText {
		t.Fatalf("result assistant text = %q, want %q", result.AssistantText, state.Turns["turn-1"].AssistantText)
	}
}

func TestRuntimeRunSessionTurnDoesNotAppendWorkflowErrorToAssistantText(t *testing.T) {
	root := t.TempDir()
	client := &fakeProvider{
		streams: []provider.Stream{
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","task_id":"task-103","parent_task_id":"task-9","title":"Apply Backend Performance Optimizations","kind":"backend"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-1", ToolName: tool.TaskWorkflowToolName},
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"create","task_id":"task-107","parent_task_id":"task-9","title":"Apply Frontend Performance Optimizations","kind":"frontend"}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-2", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-3", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"update","task_id":"task-9","status":"complete","progress":"Performance review complete. Backend and frontend follow-up tasks created."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-3", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindToolCallDelta, ToolCallID: "call-4", ToolName: tool.TaskWorkflowToolName, InputDelta: `{"action":"update","task_id":"task-103","status":"in_progress","progress":"Starting backend optimizations."}`},
				{Kind: provider.EventKindToolCallDone, ToolCallID: "call-4", ToolName: tool.TaskWorkflowToolName},
			}),
			provider.NewSliceStream([]provider.Event{
				{Kind: provider.EventKindAssistantDelta, AssistantDelta: " started backend work"},
			}),
		},
	}
	runtime := newRuntimeWithClient(t, client)

	sessionID, _ := createRuntimeWorkflowTaskWithID(t, runtime, root, "task-9", "Project Performance Review and Recommendations", events.TaskStatusInProgress)
	result, err := runtime.runExistingSessionTurn(context.Background(), runExistingTurnInput{
		SessionID: sessionID,
		TurnID:    "turn-1",
		UserText:  "lets apply all",
		AgentID:   "engineer",
	})
	if err != nil {
		t.Fatalf("runExistingSessionTurn() error = %v", err)
	}
	if result.Status != TurnRunStatusCompleted {
		t.Fatalf("result = %#v", result)
	}

	replayed, err := runtime.Store.Replay(context.Background(), events.Query{SessionID: sessionID, AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	var commits []string
	for _, event := range replayed {
		if event.TurnID != "turn-1" {
			continue
		}
		payload, ok := event.Payload.(events.AssistantCommitPayload)
		if !ok {
			continue
		}
		commits = append(commits, payload.Content)
	}
	if len(commits) != 1 {
		t.Fatalf("assistant commits = %#v, want one provider-authored commit", commits)
	}
	if commits[0] != " started backend work" {
		t.Fatalf("first assistant commit = %q", commits[0])
	}
	if result.AssistantText != " started backend work" {
		t.Fatalf("result assistant text = %q", result.AssistantText)
	}
}

func createRuntimeWorkflowTask(t *testing.T, runtime *Runtime, workspaceRoot, title, status string) (string, string) {
	t.Helper()

	sessionID, err := runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	task, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-setup",
		Title:     title,
		Status:    status,
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	return sessionID, task.TaskID
}

func createRuntimeWorkflowTaskWithID(t *testing.T, runtime *Runtime, workspaceRoot, taskID, title, status string) (string, string) {
	t.Helper()

	sessionID, err := runtime.CreateSession(context.Background(), workspaceRoot)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	task, err := runtime.Sessions.CreateTask(context.Background(), CreateTaskInput{
		SessionID: sessionID,
		TurnID:    "turn-setup",
		TaskID:    taskID,
		Title:     title,
		Status:    status,
	})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	return sessionID, task.TaskID
}
