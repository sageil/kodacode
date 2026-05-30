package app

import (
	"context"
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestRuntimeBranchSessionFromTurnCopiesHistoryThroughSelectedTurn(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	runtime := &Runtime{
		Config: Config{
			Execution: ExecutionConfig{PermissionMode: PermissionModeReadOnly},
		},
		Store:    store,
		Sessions: sessions,
	}

	sourceSessionID, err := runtime.CreateSession(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.SetTitle(ctx, sourceSessionID, "Original thread"); err != nil {
		t.Fatalf("SetTitle() error = %v", err)
	}
	appendTimelineTurnForTest(t, sessions, sourceSessionID, "turn-1", "first prompt", "first answer")
	appendTimelineTurnForTest(t, sessions, sourceSessionID, "turn-2", "second prompt", "second answer")
	sourceState, err := sessions.Snapshot(ctx, sourceSessionID)
	if err != nil {
		t.Fatalf("source Snapshot() error = %v", err)
	}
	sourceSequence := sourceState.Turns["turn-1"].CompletedAtSeq

	result, err := runtime.BranchSessionFromTurn(ctx, BranchSessionFromTurnInput{
		SourceSessionID: sourceSessionID,
		SourceTurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("BranchSessionFromTurn() error = %v", err)
	}

	branched, err := sessions.Snapshot(ctx, result.SessionID)
	if err != nil {
		t.Fatalf("branched Snapshot() error = %v", err)
	}
	if branched.Branch == nil {
		t.Fatal("branched state missing branch metadata")
	}
	if branched.Branch.ParentSessionID != sourceSessionID || branched.Branch.ParentTurnID != "turn-1" {
		t.Fatalf("branch metadata = %#v", branched.Branch)
	}
	if branched.Branch.ParentSequence != sourceSequence || result.SourceSequence != sourceSequence {
		t.Fatalf("source sequence = branch %d result %d want %d", branched.Branch.ParentSequence, result.SourceSequence, sourceSequence)
	}
	if got, want := branched.Title, "Branch: Original thread"; got != want {
		t.Fatalf("title = %q, want %q", got, want)
	}
	if got, want := branched.PermissionMode, string(PermissionModeReadOnly); got != want {
		t.Fatalf("permission mode = %q, want %q", got, want)
	}
	if got, want := branched.TurnOrder, []string{"turn-1"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("turn order = %#v, want %#v", got, want)
	}
	if turn := branched.Turns["turn-1"]; turn == nil || turn.UserText != "first prompt" || turn.AssistantText != "first answer" {
		t.Fatalf("branched turn-1 = %#v", turn)
	}
	if branched.Turns["turn-2"] != nil {
		t.Fatalf("branched state unexpectedly includes turn-2: %#v", branched.Turns["turn-2"])
	}
}

func TestRuntimeBranchSessionFromTurnRejectsIncompleteTurn(t *testing.T) {
	ctx := context.Background()
	store := events.NewMemoryStore()
	sessions, err := NewSessionService(store)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	runtime := &Runtime{Store: store, Sessions: sessions}

	sourceSessionID, err := runtime.CreateSession(ctx, t.TempDir())
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	if _, err := sessions.append(ctx, events.Draft{
		SessionID: sourceSessionID,
		TurnID:    "turn-1",
		Type:      events.TypeUserMessage,
		Payload:   events.UserMessagePayload{Content: "unfinished"},
	}); err != nil {
		t.Fatalf("append user message error = %v", err)
	}

	if _, err := runtime.BranchSessionFromTurn(ctx, BranchSessionFromTurnInput{
		SourceSessionID: sourceSessionID,
		SourceTurnID:    "turn-1",
	}); err == nil || !strings.Contains(err.Error(), "not complete") {
		t.Fatalf("BranchSessionFromTurn() error = %v, want not complete", err)
	}
}

func appendTimelineTurnForTest(t *testing.T, sessions *SessionService, sessionID, turnID, userText, assistantText string) {
	t.Helper()
	ctx := context.Background()
	drafts := []events.Draft{
		{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeUserMessage,
			Payload:   events.UserMessagePayload{Content: userText},
		},
		{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeAssistantCommit,
			Payload:   events.AssistantCommitPayload{Content: assistantText},
		},
		{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeTurnDone,
			Payload:   events.TurnDonePayload{},
		},
	}
	for _, draft := range drafts {
		if _, err := sessions.append(ctx, draft); err != nil {
			t.Fatalf("append %s/%s error = %v", draft.Type, draft.TurnID, err)
		}
	}
}
