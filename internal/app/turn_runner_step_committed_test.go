package app

import (
	"context"
	"testing"

	"github.com/sageil/kodacode/internal/events"
	"github.com/sageil/kodacode/internal/provider"
)

func TestHandleStepAnthropicThinkingCommittedAppendsConversationAndCommits(t *testing.T) {
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
	state := turnLoopState{}
	stepStart := -1
	committed := false

	result, err := runner.handleStepAnthropicThinkingCommitted(context.Background(), "session-1", "turn-1", provider.Event{
		Kind: provider.EventKindAnthropicThinkingCommitted,
		AnthropicThinking: &provider.AnthropicThinkingBlock{
			Type:      provider.AnthropicThinkingBlockTypeThinking,
			Thinking:  "inspect",
			Signature: "sig-1",
		},
	}, false, &state, &stepStart, func() {
		committed = true
	})
	if err != nil {
		t.Fatalf("handleStepAnthropicThinkingCommitted() error = %v", err)
	}
	if !result.Accepted || !result.DurableProgress {
		t.Fatalf("result = %#v", result)
	}
	if !committed || stepStart != 0 {
		t.Fatalf("committed=%v stepStart=%d", committed, stepStart)
	}
	if len(state.Conversation) != 1 || state.Conversation[0].Kind != provider.InputKindAnthropicThinking {
		t.Fatalf("conversation = %#v", state.Conversation)
	}
}

func TestHandleStepOpenAIReasoningCommittedAppendsConversationAndCommits(t *testing.T) {
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
	state := turnLoopState{}
	stepStart := -1
	committed := false
	item := []byte(`{"type":"reasoning","encrypted_content":"enc_1"}`)

	result, err := runner.handleStepOpenAIReasoningCommitted(context.Background(), "session-1", "turn-1", provider.Event{
		Kind:                provider.EventKindOpenAIReasoningCommitted,
		OpenAIReasoningItem: item,
	}, false, &state, &stepStart, func() {
		committed = true
	})
	if err != nil {
		t.Fatalf("handleStepOpenAIReasoningCommitted() error = %v", err)
	}
	if !result.Accepted || !result.DurableProgress {
		t.Fatalf("result = %#v", result)
	}
	if !committed || stepStart != 0 {
		t.Fatalf("committed=%v stepStart=%d", committed, stepStart)
	}
	if len(state.Conversation) != 1 || state.Conversation[0].Kind != provider.InputKindOpenAIReasoning {
		t.Fatalf("conversation = %#v", state.Conversation)
	}
}

func TestHandleStepCommittedReasoningContinuesAfterBatchableResults(t *testing.T) {
	runner := &TurnRunner{}
	state := turnLoopState{}
	stepStart := -1
	committed := false

	result, err := runner.handleStepAnthropicThinkingCommitted(context.Background(), "session-1", "turn-1", provider.Event{
		Kind: provider.EventKindAnthropicThinkingCommitted,
		AnthropicThinking: &provider.AnthropicThinkingBlock{
			Type:      provider.AnthropicThinkingBlockTypeThinking,
			Thinking:  "inspect",
			Signature: "sig-1",
		},
	}, true, &state, &stepStart, func() {
		committed = true
	})
	if err != nil {
		t.Fatalf("handleStepAnthropicThinkingCommitted() error = %v", err)
	}
	if !result.Accepted || result.DurableProgress {
		t.Fatalf("result = %#v", result)
	}
	if committed || stepStart != -1 || len(state.Conversation) != 0 {
		t.Fatalf("committed=%v stepStart=%d conversation=%#v", committed, stepStart, state.Conversation)
	}
}
