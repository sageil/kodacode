package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestCompleteStepAfterStreamEOFReturnsToolResultAfterProgress(t *testing.T) {
	progress := newStepToolProgress()
	progress.ExecutedTools = 1
	committed := false

	result, err := completeProviderRequestAfterStreamEOF(providerRequestEOFInput{
		Progress:      &progress,
		ToolBatchSize: 2,
		CommitToolStepBoundary: func() error {
			committed = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("completeProviderRequestAfterStreamEOF() error = %v", err)
	}
	if result.Outcome != assistantRoundtripOutcomeToolResult || result.ExecutedTools != 1 || result.ToolBatchSize != 2 {
		t.Fatalf("result = %#v", result)
	}
	if !committed {
		t.Fatal("boundary was not committed")
	}
}

func TestCompleteStepAfterStreamEOFCommitsAssistantWithoutToolProgress(t *testing.T) {
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
	runner := &TurnRunner{sessions: sessions}
	state := turnLoopState{Conversation: []provider.Input{{Kind: provider.InputKindUserMessage, Content: "hi"}}}
	segment := "done"
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:    runner,
		Context:   context.Background(),
		SessionID: "session-1",
		TurnID:    "turn-1",
		State:     &state,
		Segment:   &segment,
	})
	progress := newStepToolProgress()

	result, err := completeProviderRequestAfterStreamEOF(providerRequestEOFInput{
		Progress: &progress,
		Preview:  preview,
	})
	if err != nil {
		t.Fatalf("completeProviderRequestAfterStreamEOF() error = %v", err)
	}
	if result.Outcome != assistantRoundtripOutcomeAssistantDone || result.ExecutedTools != 0 || result.ReusedTools != 0 {
		t.Fatalf("result = %#v", result)
	}
	if state.AssistantText != "done" || segment != "" {
		t.Fatalf("AssistantText=%q segment=%q", state.AssistantText, segment)
	}
	if len(state.Conversation) != 2 || state.Conversation[1].Kind != provider.InputKindAssistantMessage || state.Conversation[1].Content != "done" {
		t.Fatalf("conversation = %#v", state.Conversation)
	}
}
