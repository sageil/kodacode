package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func TestExecuteStepToolBatchDeclaresBoundaryExecutesAndAppliesResults(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{
			definition: tool.Definition{Name: "metadata_read_a", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "first"},
		},
		stubTool{
			definition: tool.Definition{Name: "metadata_read_b", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "second"},
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

	runner := &TurnRunner{sessions: sessions, tools: executor}
	state := turnLoopState{Conversation: []provider.Input{{Kind: provider.InputKindUserMessage, Content: "inspect"}}}
	stepStart := -1
	progress := newStepToolProgress()
	commits := 0
	result, err := runner.executeStepToolBatch(context.Background(), stepToolBatchRunInput{
		SessionID:             "session-1",
		TurnID:                "turn-1",
		Model:                 provider.ModelRef{ProviderID: "test", ModelID: "model"},
		State:                 &state,
		StepConversationStart: &stepStart,
		Executor:              newStepToolExecutor(executor, []string{"metadata_read_a", "metadata_read_b"}, nil, nil, nil),
		Progress:              &progress,
		Batch: stepToolBatch{
			StepIndex: 2,
			Calls: []stepToolCall{
				{CallID: "call-1", ToolName: "metadata_read_a", Arguments: `{}`},
				{CallID: "call-2", ToolName: "metadata_read_b", Arguments: `{}`},
			},
		},
		CommitStepState: func() { commits++ },
	})
	if err != nil {
		t.Fatalf("executeStepToolBatch() error = %v", err)
	}
	if !result.DurableProgress || result.Failed || result.PendingRequestID != "" {
		t.Fatalf("result = %#v", result)
	}
	if got := result.Execution.Schedule.Executable.CallIDs(); len(got) != 2 || got[0] != "call-1" || got[1] != "call-2" {
		t.Fatalf("executable call IDs = %v", got)
	}
	if progress.ExecutedTools != 2 || progress.ReusedTools != 0 {
		t.Fatalf("progress = %#v", progress)
	}
	if stepStart != 1 || state.LatestToolStepStart != 1 {
		t.Fatalf("stepStart=%d LatestToolStepStart=%d", stepStart, state.LatestToolStepStart)
	}
	if len(state.Conversation) != 5 {
		t.Fatalf("conversation length = %d, want 5: %#v", len(state.Conversation), state.Conversation)
	}
	if state.Conversation[1].Kind != provider.InputKindToolCall || state.Conversation[1].CallID != "call-1" ||
		state.Conversation[2].Kind != provider.InputKindToolCall || state.Conversation[2].CallID != "call-2" ||
		state.Conversation[3].Kind != provider.InputKindToolResult || state.Conversation[3].CallID != "call-1" ||
		state.Conversation[4].Kind != provider.InputKindToolResult || state.Conversation[4].CallID != "call-2" {
		t.Fatalf("conversation = %#v", state.Conversation)
	}
	if commits < 3 {
		t.Fatalf("commits = %d, want boundary plus result commits", commits)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	types := make([]events.Type, 0, len(replayed))
	callIDs := make([]string, 0, len(replayed))
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolCallDeclaredPayload:
			types = append(types, event.Type)
			callIDs = append(callIDs, payload.CallID)
		case events.ToolCallBatchPayload:
			types = append(types, event.Type)
			callIDs = append(callIDs, payload.CallIDs...)
		case events.ToolExecStartPayload:
			types = append(types, event.Type)
			callIDs = append(callIDs, payload.CallID)
		case events.ToolExecEndPayload:
			types = append(types, event.Type)
			callIDs = append(callIDs, payload.CallID)
		}
	}
	wantTypes := []events.Type{
		events.TypeToolCallDeclared,
		events.TypeToolCallDeclared,
		events.TypeToolCallBatch,
		events.TypeToolExecStart,
		events.TypeToolExecEnd,
		events.TypeToolExecStart,
		events.TypeToolExecEnd,
	}
	if len(types) != len(wantTypes) {
		t.Fatalf("event types = %#v, want %#v", types, wantTypes)
	}
	for i := range wantTypes {
		if types[i] != wantTypes[i] {
			t.Fatalf("event types = %#v, want %#v", types, wantTypes)
		}
	}
	if len(callIDs) != 8 || callIDs[0] != "call-1" || callIDs[1] != "call-2" || callIDs[2] != "call-1" || callIDs[3] != "call-2" ||
		callIDs[4] != "call-1" || callIDs[5] != "call-1" || callIDs[6] != "call-2" || callIDs[7] != "call-2" {
		t.Fatalf("call IDs = %#v", callIDs)
	}
}

func TestExecuteStepToolBatchExecutesSequentialTail(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{
			definition: tool.Definition{Name: "metadata_read", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "read"},
		},
		stubTool{
			definition: tool.Definition{Name: tool.ApplyPatchToolName, InputSchema: json.RawMessage(`{"type":"object"}`)},
			result:     tool.Result{Output: "patch"},
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

	runner := &TurnRunner{sessions: sessions, tools: executor}
	state := turnLoopState{}
	stepStart := -1
	progress := newStepToolProgress()
	patchArgs := `*** Begin Patch
*** Add File: notes.txt
+hello
*** End Patch
`
	result, err := runner.executeStepToolBatch(context.Background(), stepToolBatchRunInput{
		SessionID:             "session-1",
		TurnID:                "turn-1",
		Model:                 provider.ModelRef{ProviderID: "test", ModelID: "model"},
		State:                 &state,
		StepConversationStart: &stepStart,
		Executor:              newStepToolExecutor(executor, []string{"metadata_read", tool.ApplyPatchToolName}, nil, nil, nil),
		Progress:              &progress,
		Batch: stepToolBatch{
			StepIndex: 3,
			Calls: []stepToolCall{
				{CallID: "call-read", ToolName: "metadata_read", Arguments: `{}`},
				{CallID: "call-patch", ToolName: tool.ApplyPatchToolName, Arguments: patchArgs},
			},
		},
	})
	if err != nil {
		t.Fatalf("executeStepToolBatch() error = %v", err)
	}
	if got := result.Execution.Schedule.Executable.CallIDs(); len(got) != 2 || got[0] != "call-read" || got[1] != "call-patch" {
		t.Fatalf("executable call IDs = %v", got)
	}
	if got := stepToolCallIDs(result.Execution.Schedule.Deferred); len(got) != 0 {
		t.Fatalf("deferred call IDs = %v", got)
	}
	if progress.ExecutedTools != 2 {
		t.Fatalf("progress = %#v", progress)
	}

	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	declaredPatch := false
	startedPatch := false
	completedPatch := false
	for _, event := range replayed {
		switch payload := event.Payload.(type) {
		case events.ToolCallDeclaredPayload:
			if payload.CallID == "call-patch" {
				declaredPatch = true
			}
		case events.ToolExecStartPayload:
			if payload.CallID == "call-patch" {
				startedPatch = true
			}
		case events.ToolExecEndPayload:
			if payload.CallID == "call-patch" {
				completedPatch = true
			}
		}
	}
	if !declaredPatch || !startedPatch || !completedPatch {
		t.Fatalf("patch events declared=%v started=%v completed=%v replayed=%#v", declaredPatch, startedPatch, completedPatch, replayed)
	}
}

func TestExecuteStepToolBatchReusesComputedSchedule(t *testing.T) {
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	executor, err := NewToolExecutor(sessions,
		stubTool{
			definition: tool.Definition{Name: "metadata_read_a", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "first"},
		},
		stubTool{
			definition: tool.Definition{Name: "metadata_read_b", InputSchema: json.RawMessage(`{"type":"object"}`), ParallelSafe: true},
			result:     tool.Result{Output: "second"},
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

	runner := &TurnRunner{sessions: sessions, tools: executor}
	state := turnLoopState{}
	progress := newStepToolProgress()
	result, err := runner.executeStepToolBatch(context.Background(), stepToolBatchRunInput{
		SessionID: "session-1",
		TurnID:    "turn-1",
		Model:     provider.ModelRef{ProviderID: "test", ModelID: "model"},
		State:     &state,
		Executor:  newStepToolExecutor(executor, []string{"metadata_read_a", "metadata_read_b"}, nil, nil, nil),
		CapabilityResolver: func(toolName, _ string) toolSchedulingCapability {
			if toolName == "metadata_read_a" {
				return toolSchedulingCapability{ExecutionClass: toolExecutionBlocking}
			}
			return toolSchedulingCapability{ExecutionClass: toolExecutionParallelRead, ParallelSafe: true}
		},
		Progress: &progress,
		Batch: stepToolBatch{
			StepIndex: 4,
			Calls: []stepToolCall{
				{CallID: "call-1", ToolName: "metadata_read_a", Arguments: `{}`},
				{CallID: "call-2", ToolName: "metadata_read_b", Arguments: `{}`},
			},
		},
	})
	if err != nil {
		t.Fatalf("executeStepToolBatch() error = %v", err)
	}
	if got := result.Execution.Schedule.Executable.CallIDs(); len(got) != 1 || got[0] != "call-1" {
		t.Fatalf("executable call IDs = %v, want [call-1]", got)
	}
	if got := stepToolCallIDs(result.Execution.Schedule.Deferred); len(got) != 1 || got[0] != "call-2" {
		t.Fatalf("deferred call IDs = %v, want [call-2]", got)
	}
	if progress.ExecutedTools != 1 {
		t.Fatalf("progress = %#v, want one executed tool", progress)
	}
	if len(state.Conversation) != 2 || state.Conversation[0].CallID != "call-1" || state.Conversation[1].CallID != "call-1" {
		t.Fatalf("conversation = %#v, want only call-1 call/result", state.Conversation)
	}
}
