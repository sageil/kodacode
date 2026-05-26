package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func newStepToolBoundaryTestRunner(t *testing.T) (*TurnRunner, *events.MemoryStore) {
	t.Helper()
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
	return &TurnRunner{sessions: sessions}, store
}

func TestStepToolBoundaryCommitsGrowingBatchOnce(t *testing.T) {
	runner, store := newStepToolBoundaryTestRunner(t)
	batch := stepToolBatch{
		StepIndex: 1,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: "read", Arguments: `{}`},
			{CallID: "call-2", ToolName: "search", Arguments: `{}`},
		},
	}
	state := turnLoopState{
		Conversation: []provider.Input{
			{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{}`},
			{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "one"},
			{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: "search", Arguments: `{}`},
			{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: "search", Output: "two"},
		},
	}
	committedSize := 0
	commits := 0
	boundary := newStepToolBoundary(stepToolBoundaryInput{
		Runner:                 runner,
		Context:                context.Background(),
		SessionID:              "session-1",
		TurnID:                 "turn-1",
		State:                  &state,
		Batch:                  &batch,
		CommittedToolBatchSize: &committedSize,
		CommitStepState: func() {
			commits++
		},
	})

	if err := boundary.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if committedSize != 2 {
		t.Fatalf("committedSize = %d", committedSize)
	}
	if commits != 1 {
		t.Fatalf("commits = %d", commits)
	}
	if len(state.Conversation) != 4 ||
		state.Conversation[0].Kind != provider.InputKindToolCall || state.Conversation[0].CallID != "call-1" ||
		state.Conversation[1].Kind != provider.InputKindToolCall || state.Conversation[1].CallID != "call-2" ||
		state.Conversation[2].Kind != provider.InputKindToolResult || state.Conversation[2].CallID != "call-1" ||
		state.Conversation[3].Kind != provider.InputKindToolResult || state.Conversation[3].CallID != "call-2" {
		t.Fatalf("conversation = %#v", state.Conversation)
	}

	if err := boundary.Commit(); err != nil {
		t.Fatalf("second Commit() error = %v", err)
	}
	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	batchEvents := 0
	for _, event := range replayed {
		if event.Type == events.TypeToolCallBatch {
			batchEvents++
		}
	}
	if batchEvents != 1 {
		t.Fatalf("batchEvents = %d", batchEvents)
	}
	if commits != 2 {
		t.Fatalf("commits after second commit = %d", commits)
	}
}

func TestStepToolBoundarySkipsSingleCallBatchEvent(t *testing.T) {
	runner, store := newStepToolBoundaryTestRunner(t)
	batch := stepToolBatch{
		StepIndex: 1,
		Calls: []stepToolCall{
			{CallID: "call-1", ToolName: "read", Arguments: `{}`},
		},
	}
	committedSize := 0
	commits := 0
	boundary := newStepToolBoundary(stepToolBoundaryInput{
		Runner:                 runner,
		Context:                context.Background(),
		SessionID:              "session-1",
		TurnID:                 "turn-1",
		Batch:                  &batch,
		CommittedToolBatchSize: &committedSize,
		CommitStepState: func() {
			commits++
		},
	})

	if err := boundary.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}
	if committedSize != 0 {
		t.Fatalf("committedSize = %d", committedSize)
	}
	if commits != 1 {
		t.Fatalf("commits = %d", commits)
	}
	replayed, err := store.Replay(context.Background(), events.Query{SessionID: "session-1", AfterSequence: -1})
	if err != nil {
		t.Fatalf("Replay() error = %v", err)
	}
	for _, event := range replayed {
		if event.Type == events.TypeToolCallBatch {
			t.Fatalf("unexpected batch event: %#v", event)
		}
	}
}
