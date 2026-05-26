package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/tool"
)

func TestStepToolExecutorExecuteBatchAppliesSchedulerAndPreservesOrder(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{
			definition: tool.Definition{Name: tool.ReadToolName, InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "read output"},
		},
		stubTool{
			definition: tool.Definition{Name: "git_status", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "git status output"},
		},
		stubTool{
			definition: tool.Definition{Name: tool.ApplyPatchToolName, InputSchema: json.RawMessage(`{"type":"object"}`)},
			result:     tool.Result{Output: "patch output"},
		},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	stepExecutor := newStepToolExecutor(executor, []string{tool.ReadToolName, "git_status", tool.ApplyPatchToolName}, nil, nil, nil)
	patchArgs := `*** Begin Patch
*** Add File: notes.txt
+hello
*** End Patch
`
	execution, err := stepExecutor.ExecuteBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 3,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: tool.ReadToolName, Arguments: `{}`},
			{CallID: "call-2", ToolName: "git_status", Arguments: `{}`},
			{CallID: "call-3", ToolName: tool.ApplyPatchToolName, Arguments: patchArgs},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if got := execution.Schedule.Executable.CallIDs(); len(got) != 3 || got[0] != "call-1" || got[1] != "call-2" || got[2] != "call-3" {
		t.Fatalf("executable call IDs = %v", got)
	}
	if got := stepToolCallIDs(execution.Schedule.Deferred); len(got) != 0 {
		t.Fatalf("deferred call IDs = %v", got)
	}
	results := execution.Results
	if len(results) != 3 {
		t.Fatalf("results = %#v, want all provider-declared calls", results)
	}
	if results[0].CallID != "call-1" || results[0].Output != "read output" || results[0].Status != ToolExecutionStatusExecuted {
		t.Fatalf("first result = %#v", results[0])
	}
	if results[1].CallID != "call-2" || results[1].Output != "git status output" || results[1].Status != ToolExecutionStatusExecuted {
		t.Fatalf("second result = %#v", results[1])
	}
	if results[2].CallID != "call-3" || results[2].Output != "patch output" || results[2].Status != ToolExecutionStatusExecuted {
		t.Fatalf("third result = %#v", results[2])
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	starts := make([]string, 0, 2)
	ends := make([]string, 0, 1)
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolExecStartPayload:
			starts = append(starts, payload.CallID)
		case events.ToolExecEndPayload:
			ends = append(ends, payload.CallID)
		}
	}
	if len(starts) != 3 || starts[0] != "call-1" || starts[1] != "call-2" || starts[2] != "call-3" {
		t.Fatalf("tool starts = %#v, want call-1 then call-2 then call-3", starts)
	}
	if len(ends) != 3 || ends[0] != "call-1" || ends[1] != "call-2" || ends[2] != "call-3" {
		t.Fatalf("tool ends = %#v, want call-1 then call-2 then call-3", ends)
	}
}

func TestStepToolExecutorExecuteBatchStopsAfterWorkspaceMutation(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{
			definition: tool.Definition{Name: tool.ReadToolName, InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "read output"},
		},
		tool.NewApplyPatchTool(),
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("before\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	firstPatch := `*** Begin Patch
*** Update File: notes.txt
@@
-before
+after
*** End Patch
`
	stalePatch := `*** Begin Patch
*** Update File: notes.txt
@@
-before
+stale
*** End Patch
`
	stepExecutor := newStepToolExecutor(executor, []string{tool.ReadToolName, tool.ApplyPatchToolName}, nil, nil, nil)
	execution, err := stepExecutor.ExecuteBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 3,
		Calls: []stepToolCall{
			{CallID: "call-read", ToolName: tool.ReadToolName, Arguments: `{}`},
			{CallID: "call-patch", ToolName: tool.ApplyPatchToolName, Arguments: firstPatch},
			{CallID: "call-stale", ToolName: tool.ApplyPatchToolName, Arguments: stalePatch},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if got := execution.Schedule.Executable.CallIDs(); len(got) != 2 || got[0] != "call-read" || got[1] != "call-patch" {
		t.Fatalf("executable call IDs = %v", got)
	}
	if got := stepToolCallIDs(execution.Schedule.Deferred); len(got) != 1 || got[0] != "call-stale" {
		t.Fatalf("deferred call IDs = %v", got)
	}
	if len(execution.Results) != 2 {
		t.Fatalf("results = %#v, want read and first patch only", execution.Results)
	}
	content, err := os.ReadFile(filepath.Join(root, "notes.txt"))
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if got := string(content); got != "after\n" {
		t.Fatalf("content = %q", got)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolExecStartPayload:
			if payload.CallID == "call-stale" {
				t.Fatal("stale patch should not have started")
			}
		case events.ToolExecEndPayload:
			if payload.CallID == "call-stale" {
				t.Fatal("stale patch should not have completed")
			}
		}
	}
}

func TestStepToolExecutorExecuteBatchUsesToolMetadataForScheduling(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{
			definition: tool.Definition{Name: "metadata_read", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "metadata read output"},
		},
		stubTool{
			definition: tool.Definition{Name: "metadata_write", InputSchema: json.RawMessage(`{"type":"object"}`)},
			result:     tool.Result{Output: "metadata write output"},
		},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	stepExecutor := newStepToolExecutor(executor, []string{"metadata_read", "metadata_write"}, nil, nil, nil)
	execution, err := stepExecutor.ExecuteBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 5,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: "metadata_read", Arguments: `{}`},
			{CallID: "call-2", ToolName: "metadata_read", Arguments: `{}`},
			{CallID: "call-3", ToolName: "metadata_write", Arguments: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if got := execution.Schedule.Executable.CallIDs(); len(got) != 3 || got[0] != "call-1" || got[1] != "call-2" || got[2] != "call-3" {
		t.Fatalf("executable call IDs = %v", got)
	}
	if got := stepToolCallIDs(execution.Schedule.Deferred); len(got) != 0 {
		t.Fatalf("deferred call IDs = %v", got)
	}
	if len(execution.Results) != 3 {
		t.Fatalf("results = %#v, want all provider-declared calls", execution.Results)
	}
}

func TestStepToolExecutorExecuteBatchBuffersEventsUntilOrderedCommit(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	var observedToolEventsDuringSecondCall atomic.Int64
	observedToolEventsDuringSecondCall.Store(-1)
	executor, err := NewToolExecutor(sessions,
		observingStubTool{
			definition: tool.Definition{Name: "metadata_read_a", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "first"},
		},
		observingStubTool{
			definition: tool.Definition{Name: "metadata_read_b", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "second"},
			observe: func() {
				replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
				if err != nil {
					observedToolEventsDuringSecondCall.Store(-2)
					return
				}
				observedToolEventsDuringSecondCall.Store(int64(countToolExecEvents(replayed)))
			},
		},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	stepExecutor := newStepToolExecutor(executor, []string{"metadata_read_a", "metadata_read_b"}, nil, nil, nil)
	execution, err := stepExecutor.ExecuteBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 6,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: "metadata_read_a", Arguments: `{}`},
			{CallID: "call-2", ToolName: "metadata_read_b", Arguments: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if len(execution.Results) != 2 {
		t.Fatalf("results = %#v, want two buffered results", execution.Results)
	}
	if got := observedToolEventsDuringSecondCall.Load(); got != 0 {
		t.Fatalf("tool exec events during second call = %d, want 0 before ordered commit", got)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	starts := make([]string, 0, 2)
	ends := make([]string, 0, 2)
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolExecStartPayload:
			starts = append(starts, payload.CallID)
		case events.ToolExecEndPayload:
			ends = append(ends, payload.CallID)
		}
	}
	if len(starts) != 2 || starts[0] != "call-1" || starts[1] != "call-2" {
		t.Fatalf("tool starts = %#v, want call-1 then call-2", starts)
	}
	if len(ends) != 2 || ends[0] != "call-1" || ends[1] != "call-2" {
		t.Fatalf("tool ends = %#v, want call-1 then call-2", ends)
	}
}

func TestStepToolExecutorExecuteBatchRunsEligibleToolsConcurrently(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	secondStarted := make(chan struct{})
	executor, err := NewToolExecutor(sessions,
		concurrentStubTool{
			definition: tool.Definition{Name: "metadata_read_a", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			execute: func() tool.Result {
				select {
				case <-secondStarted:
					return tool.Result{Output: "first"}
				case <-time.After(time.Second):
					return tool.Result{Error: "second call did not start while first call was running"}
				}
			},
		},
		concurrentStubTool{
			definition: tool.Definition{Name: "metadata_read_b", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			execute: func() tool.Result {
				close(secondStarted)
				return tool.Result{Output: "second"}
			},
		},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	stepExecutor := newStepToolExecutor(executor, []string{"metadata_read_a", "metadata_read_b"}, nil, nil, nil)
	execution, err := stepExecutor.ExecuteBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 7,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: "metadata_read_a", Arguments: `{}`},
			{CallID: "call-2", ToolName: "metadata_read_b", Arguments: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if len(execution.Results) != 2 {
		t.Fatalf("results = %#v, want two parallel-safe results", execution.Results)
	}
	if execution.Results[0].CallID != "call-1" || execution.Results[0].Output != "first" || execution.Results[0].Error != "" {
		t.Fatalf("first result = %#v", execution.Results[0])
	}
	if execution.Results[1].CallID != "call-2" || execution.Results[1].Output != "second" || execution.Results[1].Error != "" {
		t.Fatalf("second result = %#v", execution.Results[1])
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	starts := make([]string, 0, 2)
	ends := make([]string, 0, 2)
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolExecStartPayload:
			starts = append(starts, payload.CallID)
		case events.ToolExecEndPayload:
			ends = append(ends, payload.CallID)
		}
	}
	if len(starts) != 2 || starts[0] != "call-1" || starts[1] != "call-2" {
		t.Fatalf("tool starts = %#v, want call-1 then call-2", starts)
	}
	if len(ends) != 2 || ends[0] != "call-1" || ends[1] != "call-2" {
		t.Fatalf("tool ends = %#v, want call-1 then call-2", ends)
	}
}

func TestStepToolExecutorExecuteBatchExecutesDuplicateReadsInSameParallelEligibleBatch(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions, tool.NewReadTool())
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte("{\"name\":\"demo\"}\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: root,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	stepExecutor := newStepToolExecutor(executor, []string{tool.ReadToolName}, nil, nil, nil)
	execution, err := stepExecutor.ExecuteBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 8,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: tool.ReadToolName, Arguments: `{"paths":["package.json"]}`},
			{CallID: "call-2", ToolName: tool.ReadToolName, Arguments: `{"paths":["package.json"],"offset":0,"limit":1000}`},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if got := execution.Schedule.Executable.CallIDs(); len(got) != 2 || got[0] != "call-1" || got[1] != "call-2" {
		t.Fatalf("executable call IDs = %v", got)
	}
	if len(execution.Results) != 2 {
		t.Fatalf("results = %#v, want two read results", execution.Results)
	}
	if execution.Results[0].Status != ToolExecutionStatusExecuted || execution.Results[0].Output != expectedReadSingleLineOutputForPath("package.json", "{\"name\":\"demo\"}") {
		t.Fatalf("first result = %#v", execution.Results[0])
	}
	if execution.Results[1].Status != ToolExecutionStatusExecuted ||
		execution.Results[1].Output != execution.Results[0].Output ||
		execution.Results[1].ReusedFromCallID != "" {
		t.Fatalf("second result = %#v", execution.Results[1])
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	starts := make([]string, 0, 2)
	reusedFrom := make(map[string]string)
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolExecStartPayload:
			starts = append(starts, payload.CallID)
		case events.ToolExecEndPayload:
			reusedFrom[payload.CallID] = payload.ReusedFromCallID
		}
	}
	if len(starts) != 2 || starts[0] != "call-1" || starts[1] != "call-2" {
		t.Fatalf("tool starts = %#v, want both duplicate reads to execute", starts)
	}
	if reusedFrom["call-1"] != "" || reusedFrom["call-2"] != "" {
		t.Fatalf("reusedFrom = %#v, want no reused tool results", reusedFrom)
	}
}

func TestStepToolExecutorExecuteBatchStopsOnPending(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		tool.NewQuestionTool(),
		stubTool{
			definition: tool.Definition{Name: tool.ReadToolName, InputSchema: json.RawMessage(`{"type":"object"}`)},
			result:     tool.Result{Output: "read output"},
		},
	)
	if err != nil {
		t.Fatalf("NewToolExecutor() error = %v", err)
	}
	if _, err := sessions.CreateSession(context.Background(), CreateSessionInput{
		SessionID:     "session-1",
		WorkspaceRoot: t.TempDir(),
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	stepExecutor := newStepToolExecutor(executor, []string{tool.QuestionToolName, tool.ReadToolName}, nil, nil, nil)
	execution, err := stepExecutor.ExecuteBatch(context.Background(), "session-1", "turn-1", stepToolBatch{
		StepIndex: 4,
		Calls: []stepToolCall{
			{CallID: "call-question", ToolName: tool.QuestionToolName, Arguments: `{"question":"Which path?","options":["a","b"]}`},
			{CallID: "call-read", ToolName: tool.ReadToolName, Arguments: `{}`},
		},
	})
	if err != nil {
		t.Fatalf("ExecuteBatch() error = %v", err)
	}
	if got := execution.Schedule.Executable.CallIDs(); len(got) != 1 || got[0] != "call-question" {
		t.Fatalf("executable call IDs = %v", got)
	}
	if got := stepToolCallIDs(execution.Schedule.Deferred); len(got) != 1 || got[0] != "call-read" {
		t.Fatalf("deferred call IDs = %v", got)
	}
	results := execution.Results
	if len(results) != 1 {
		t.Fatalf("results = %#v, want pending question only", results)
	}
	if results[0].CallID != "call-question" || results[0].PendingRequestID == "" || results[0].Status != ToolExecutionStatusPending {
		t.Fatalf("question result = %#v", results[0])
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	starts := make([]string, 0, 1)
	for _, event := range replayed {
		if payload, ok := event.Payload.(events.ToolExecStartPayload); ok {
			starts = append(starts, payload.CallID)
		}
	}
	if len(starts) != 1 || starts[0] != "call-question" {
		t.Fatalf("tool starts = %#v, want only pending question", starts)
	}
	orderedTypes := make([]events.Type, 0, 2)
	for _, event := range replayed {
		switch event.Type {
		case events.TypeToolExecStart, events.TypeQuestionRequested:
			orderedTypes = append(orderedTypes, event.Type)
		}
	}
	if len(orderedTypes) != 2 || orderedTypes[0] != events.TypeToolExecStart || orderedTypes[1] != events.TypeQuestionRequested {
		t.Fatalf("event order = %#v, want tool start before question request", orderedTypes)
	}
}

type observingStubTool struct {
	definition tool.Definition
	result     tool.Result
	observe    func()
}

func (s observingStubTool) Definition() tool.Definition {
	return s.definition
}

func (s observingStubTool) Execute(context.Context, tool.ExecutionContext, json.RawMessage) (tool.Result, error) {
	if s.observe != nil {
		s.observe()
	}
	return s.result, nil
}

type concurrentStubTool struct {
	definition tool.Definition
	execute    func() tool.Result
}

func (s concurrentStubTool) Definition() tool.Definition {
	return s.definition
}

func (s concurrentStubTool) Execute(context.Context, tool.ExecutionContext, json.RawMessage) (tool.Result, error) {
	if s.execute == nil {
		return tool.Result{}, nil
	}
	return s.execute(), nil
}

func countToolExecEvents(replayed []events.Event) int {
	count := 0
	for _, event := range replayed {
		switch event.Payload.(type) {
		case events.ToolExecStartPayload, events.ToolExecEndPayload:
			count++
		}
	}
	return count
}
