package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestApplyStepToolResultToConversationPreservesContextAndCanonicalArguments(t *testing.T) {
	state := turnLoopState{
		Conversation: []provider.Input{{Kind: provider.InputKindUserMessage, Content: "read it"}},
	}
	stepStart := -1
	update := applyStepToolResultToConversation(&state, &stepStart, stepToolCall{
		CallID:                 "call-1",
		ToolName:               "read",
		Arguments:              `{"path":"app.go"}`,
		GoogleThoughtSignature: []byte("sig-1"),
		OpenAIReasoningContent: "inspect first",
	}, stepToolResult{
		CallID:             "call-1",
		ToolName:           "read",
		CanonicalArguments: `{"paths":["app.go"]}`,
		Output:             "contents",
		Status:             ToolExecutionStatusExecuted,
	})

	if update.Pending {
		t.Fatal("update.Pending = true, want false")
	}
	if update.Arguments != `{"paths":["app.go"]}` {
		t.Fatalf("update.Arguments = %q", update.Arguments)
	}
	if stepStart != 1 || state.LatestToolStepStart != 1 {
		t.Fatalf("stepStart=%d LatestToolStepStart=%d, want 1", stepStart, state.LatestToolStepStart)
	}
	if len(state.Conversation) != 3 {
		t.Fatalf("conversation length = %d, want 3", len(state.Conversation))
	}
	call := state.Conversation[1]
	if call.Kind != provider.InputKindToolCall || call.CallID != "call-1" || call.Arguments != `{"paths":["app.go"]}` {
		t.Fatalf("tool call input = %#v", call)
	}
	if got := string(call.GoogleThoughtSignature); got != "sig-1" {
		t.Fatalf("GoogleThoughtSignature = %q", got)
	}
	if call.OpenAIReasoningContent != "inspect first" {
		t.Fatalf("OpenAIReasoningContent = %q", call.OpenAIReasoningContent)
	}
	result := state.Conversation[2]
	if result.Kind != provider.InputKindToolResult || result.CallID != "call-1" || result.Output != "contents" || result.Error != "" {
		t.Fatalf("tool result input = %#v", result)
	}
}

func TestApplyStepToolResultToConversationLeavesPendingWithoutResult(t *testing.T) {
	state := turnLoopState{}
	stepStart := -1
	update := applyStepToolResultToConversation(&state, &stepStart, stepToolCall{
		CallID:    "call-question",
		ToolName:  "question",
		Arguments: `{"question":"Proceed?"}`,
	}, stepToolResult{
		CallID:           "call-question",
		ToolName:         "question",
		Status:           ToolExecutionStatusPending,
		PendingRequestID: "question-1",
	})

	if !update.Pending {
		t.Fatal("update.Pending = false, want true")
	}
	if update.Arguments != `{"question":"Proceed?"}` {
		t.Fatalf("update.Arguments = %q", update.Arguments)
	}
	if len(state.Conversation) != 1 {
		t.Fatalf("conversation length = %d, want pending call only", len(state.Conversation))
	}
	if state.Conversation[0].Kind != provider.InputKindToolCall {
		t.Fatalf("conversation[0] = %#v", state.Conversation[0])
	}
}
