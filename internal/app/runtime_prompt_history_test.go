package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

type promptHistoryStubClient struct{}

func (promptHistoryStubClient) Stream(context.Context, provider.Request) (provider.Stream, error) {
	return promptHistoryStubStream{}, nil
}

type promptHistoryStubStream struct{}

func (promptHistoryStubStream) Recv() (provider.Event, error) {
	return provider.Event{}, io.EOF
}

func (promptHistoryStubStream) Close() error {
	return nil
}

func TestRuntimeListPromptHistoryOrdersGloballyAndKeepsDuplicates(t *testing.T) {
	runtime := newPersistentRuntimeWithClient(t, t.TempDir(), promptHistoryStubClient{})
	ctx := context.Background()
	rootA := filepath.Join(t.TempDir(), "repo-a")
	rootB := filepath.Join(t.TempDir(), "repo-b")
	for _, root := range []string{rootA, rootB} {
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatalf("MkdirAll(%q) error = %v", root, err)
		}
	}

	sessionOne, err := runtime.CreateSession(ctx, rootA)
	if err != nil {
		t.Fatalf("CreateSession(sessionOne) error = %v", err)
	}
	for _, turn := range []struct {
		id     string
		prompt string
	}{
		{id: "turn-1", prompt: "review cache middleware"},
		{id: "turn-2", prompt: "investigate duplicate reads"},
	} {
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionOne,
			TurnID:    turn.id,
			Type:      events.TypeUserMessage,
			Payload:   events.UserMessagePayload{Content: turn.prompt},
		}); err != nil {
			t.Fatalf("append sessionOne %s user error = %v", turn.id, err)
		}
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionOne,
			TurnID:    turn.id,
			Type:      events.TypeTurnDone,
			Payload:   events.TurnDonePayload{},
		}); err != nil {
			t.Fatalf("append sessionOne %s done error = %v", turn.id, err)
		}
	}
	if _, err := runtime.Sessions.SetTitle(ctx, sessionOne, "Cache review"); err != nil {
		t.Fatalf("SetTitle(sessionOne) error = %v", err)
	}

	sessionTwo, err := runtime.CreateSession(ctx, rootB)
	if err != nil {
		t.Fatalf("CreateSession(sessionTwo) error = %v", err)
	}
	for _, turn := range []struct {
		id     string
		prompt string
	}{
		{id: "turn-1", prompt: "investigate duplicate reads"},
		{id: "turn-2", prompt: "trace provider retries"},
	} {
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionTwo,
			TurnID:    turn.id,
			Type:      events.TypeUserMessage,
			Payload:   events.UserMessagePayload{Content: turn.prompt},
		}); err != nil {
			t.Fatalf("append sessionTwo %s user error = %v", turn.id, err)
		}
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionTwo,
			TurnID:    turn.id,
			Type:      events.TypeTurnDone,
			Payload:   events.TurnDonePayload{},
		}); err != nil {
			t.Fatalf("append sessionTwo %s done error = %v", turn.id, err)
		}
	}
	if _, err := runtime.Sessions.SetTitle(ctx, sessionTwo, "Retry analysis"); err != nil {
		t.Fatalf("SetTitle(sessionTwo) error = %v", err)
	}

	entries, err := runtime.ListPromptHistory(ctx, 10)
	if err != nil {
		t.Fatalf("ListPromptHistory() error = %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("entry count = %d, want 4 (%#v)", len(entries), entries)
	}
	if entries[0].Prompt != "trace provider retries" || entries[0].SessionTitle != "Retry analysis" {
		t.Fatalf("entry[0] = %#v", entries[0])
	}
	if entries[1].Prompt != "investigate duplicate reads" || entries[1].SessionTitle != "Retry analysis" {
		t.Fatalf("entry[1] = %#v", entries[1])
	}
	if entries[2].Prompt != "investigate duplicate reads" || entries[2].SessionTitle != "Cache review" {
		t.Fatalf("entry[2] = %#v", entries[2])
	}
	if entries[3].Prompt != "review cache middleware" || entries[3].SessionTitle != "Cache review" {
		t.Fatalf("entry[3] = %#v", entries[3])
	}
}

func TestRuntimeListPromptHistoryHonorsLimit(t *testing.T) {
	runtime := newPersistentRuntimeWithClient(t, t.TempDir(), promptHistoryStubClient{})
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll(root) error = %v", err)
	}

	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for idx, prompt := range []string{"one", "two", "three"} {
		turnID := "turn-" + string(rune('1'+idx))
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeUserMessage,
			Payload:   events.UserMessagePayload{Content: prompt},
		}); err != nil {
			t.Fatalf("append user %q error = %v", prompt, err)
		}
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionID,
			TurnID:    turnID,
			Type:      events.TypeTurnDone,
			Payload:   events.TurnDonePayload{},
		}); err != nil {
			t.Fatalf("append done %q error = %v", prompt, err)
		}
	}

	entries, err := runtime.ListPromptHistory(ctx, 2)
	if err != nil {
		t.Fatalf("ListPromptHistory() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if entries[0].Prompt != "three" || entries[1].Prompt != "two" {
		t.Fatalf("entries = %#v", entries)
	}
}

func TestRuntimeListPromptHistoryExcludesAutoReviewPrompts(t *testing.T) {
	runtime := newPersistentRuntimeWithClient(t, t.TempDir(), promptHistoryStubClient{})
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("MkdirAll(root) error = %v", err)
	}

	sessionID, err := runtime.CreateSession(ctx, root)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	for _, turn := range []struct {
		id     string
		prompt string
	}{
		{id: "turn-1", prompt: "review middleware"},
		{id: "turn-2", prompt: autoReviewUserText},
		{id: "turn-3", prompt: "optimize query builder"},
	} {
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionID,
			TurnID:    turn.id,
			Type:      events.TypeUserMessage,
			Payload:   events.UserMessagePayload{Content: turn.prompt},
		}); err != nil {
			t.Fatalf("append user %q error = %v", turn.prompt, err)
		}
		if _, err := runtime.Sessions.append(ctx, events.Draft{
			SessionID: sessionID,
			TurnID:    turn.id,
			Type:      events.TypeTurnDone,
			Payload:   events.TurnDonePayload{},
		}); err != nil {
			t.Fatalf("append done %q error = %v", turn.prompt, err)
		}
	}

	entries, err := runtime.ListPromptHistory(ctx, 10)
	if err != nil {
		t.Fatalf("ListPromptHistory() error = %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2 (%#v)", len(entries), entries)
	}
	if entries[0].Prompt != "optimize query builder" || entries[1].Prompt != "review middleware" {
		t.Fatalf("entries = %#v", entries)
	}
}
