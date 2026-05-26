package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
	"github.com/sageil/kodacode/internal/tool"
)

func newStepAssistantPreviewTestRunner(t *testing.T) (*TurnRunner, *SessionService) {
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
	return &TurnRunner{sessions: sessions}, sessions
}

func TestStepAssistantPreviewCommitAssistant(t *testing.T) {
	runner, sessions := newStepAssistantPreviewTestRunner(t)
	state := turnLoopState{AssistantText: "old ", Conversation: []provider.Input{{Kind: provider.InputKindUserMessage, Content: "hi"}}}
	segment := "new"
	durableProgress := false
	committed := false
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:    runner,
		Context:   context.Background(),
		SessionID: "session-1",
		TurnID:    "turn-1",
		State:     &state,
		Segment:   &segment,
		MarkDurableProgress: func() {
			durableProgress = true
		},
		CommitStepState: func() {
			committed = true
		},
	})

	if err := preview.CommitAssistant(); err != nil {
		t.Fatalf("CommitAssistant() error = %v", err)
	}
	if state.AssistantText != "old new" {
		t.Fatalf("AssistantText = %q", state.AssistantText)
	}
	if segment != "" {
		t.Fatalf("segment = %q", segment)
	}
	if !durableProgress || !committed {
		t.Fatalf("durableProgress=%v committed=%v", durableProgress, committed)
	}
	if len(state.Conversation) != 2 || state.Conversation[1].Kind != provider.InputKindAssistantMessage || state.Conversation[1].Content != "new" {
		t.Fatalf("conversation = %#v", state.Conversation)
	}

	snapshot, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Turns["turn-1"].AssistantText != "old new" {
		t.Fatalf("snapshot assistant text = %q", snapshot.Turns["turn-1"].AssistantText)
	}
}

func TestStepAssistantPreviewCommitAssistantTrimsContinuationOverlap(t *testing.T) {
	runner, sessions := newStepAssistantPreviewTestRunner(t)
	state := turnLoopState{
		AssistantText: "1. Done\n\n4. En",
		Conversation:  []provider.Input{{Kind: provider.InputKindAssistantMessage, Content: "1. Done\n\n4. En"}},
	}
	segment := "4. Enforce consistent error handling."
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:      runner,
		Context:     context.Background(),
		SessionID:   "session-1",
		TurnID:      "turn-1",
		State:       &state,
		Segment:     &segment,
		TrimOverlap: true,
	})

	if err := preview.CommitAssistant(); err != nil {
		t.Fatalf("CommitAssistant() error = %v", err)
	}
	want := "1. Done\n\n4. Enforce consistent error handling."
	if state.AssistantText != want {
		t.Fatalf("AssistantText = %q, want %q", state.AssistantText, want)
	}
	if state.Conversation[0].Content != want {
		t.Fatalf("conversation assistant = %q, want %q", state.Conversation[0].Content, want)
	}

	snapshot, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if snapshot.Turns["turn-1"].AssistantText != want {
		t.Fatalf("snapshot assistant text = %q, want %q", snapshot.Turns["turn-1"].AssistantText, want)
	}
}

func TestStepAssistantPreviewCommitAssistantKeepsRepeatedTextWithoutOverlapTrim(t *testing.T) {
	runner, _ := newStepAssistantPreviewTestRunner(t)
	state := turnLoopState{AssistantText: "ha"}
	segment := "ha"
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:    runner,
		Context:   context.Background(),
		SessionID: "session-1",
		TurnID:    "turn-1",
		State:     &state,
		Segment:   &segment,
	})

	if err := preview.CommitAssistant(); err != nil {
		t.Fatalf("CommitAssistant() error = %v", err)
	}
	if state.AssistantText != "haha" {
		t.Fatalf("AssistantText = %q, want haha", state.AssistantText)
	}
}

func TestStepAssistantPreviewStartToolStepDiscardsWorklog(t *testing.T) {
	runner, sessions := newStepAssistantPreviewTestRunner(t)
	state := turnLoopState{}
	segment := "checking file"
	hasToolCalls := false
	durableProgress := false
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:       runner,
		Context:      context.Background(),
		SessionID:    "session-1",
		TurnID:       "turn-1",
		State:        &state,
		Segment:      &segment,
		HasToolCalls: &hasToolCalls,
		MarkDurableProgress: func() {
			durableProgress = true
		},
	})

	if err := preview.StartToolStep(tool.ReadToolName); err != nil {
		t.Fatalf("StartToolStep() error = %v", err)
	}
	if !hasToolCalls {
		t.Fatal("hasToolCalls = false")
	}
	if segment != "" {
		t.Fatalf("segment = %q", segment)
	}
	if !durableProgress {
		t.Fatal("durableProgress = false")
	}
	snapshot, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	transcript := snapshot.Turns["turn-1"].Transcript
	if len(transcript) != 1 || transcript[0].Kind != events.TranscriptEntryWorklog || transcript[0].Text != "checking file" {
		t.Fatalf("transcript = %#v", transcript)
	}
}

func TestStepAssistantPreviewStartToolStepKeepsQuestionPreview(t *testing.T) {
	runner, sessions := newStepAssistantPreviewTestRunner(t)
	state := turnLoopState{}
	segment := "plan text"
	hasToolCalls := false
	durableProgress := false
	preview := newStepAssistantPreview(stepAssistantPreviewInput{
		Runner:       runner,
		Context:      context.Background(),
		SessionID:    "session-1",
		TurnID:       "turn-1",
		State:        &state,
		Segment:      &segment,
		HasToolCalls: &hasToolCalls,
		MarkDurableProgress: func() {
			durableProgress = true
		},
	})

	if err := preview.StartToolStep(tool.QuestionToolName); err != nil {
		t.Fatalf("StartToolStep() error = %v", err)
	}
	if !hasToolCalls {
		t.Fatal("hasToolCalls = false")
	}
	if segment != "plan text" {
		t.Fatalf("segment = %q", segment)
	}
	if durableProgress {
		t.Fatal("durableProgress = true")
	}
	snapshot, err := sessions.Snapshot(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	if turn := snapshot.Turns["turn-1"]; turn != nil && len(turn.Transcript) != 0 {
		t.Fatalf("transcript = %#v", turn.Transcript)
	}
}
