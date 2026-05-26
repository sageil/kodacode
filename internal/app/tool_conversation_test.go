package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestNormalizeToolCallBatchReturnsDetachedCopyWhenBatchIsTooSmall(t *testing.T) {
	inputs := []provider.Input{
		{Kind: provider.InputKindUserMessage, Content: "inspect files"},
	}

	got := normalizeToolCallBatch(inputs, []string{"call-1"})
	if len(got) != len(inputs) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(inputs))
	}

	got[0].Content = "mutated copy"
	if inputs[0].Content != "inspect files" {
		t.Fatalf("inputs[0].Content = %q, want original content", inputs[0].Content)
	}
}

func TestNormalizeToolCallBatchReturnsDetachedCopyWhenBatchDoesNotReorder(t *testing.T) {
	inputs := []provider.Input{
		{Kind: provider.InputKindUserMessage, Content: "inspect files"},
		{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["a.go"]}`},
		{Kind: provider.InputKindAssistantMessage, Content: "runtime note"},
	}

	got := normalizeToolCallBatch(inputs, []string{"call-1", "call-2"})
	if len(got) != len(inputs) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(inputs))
	}

	got[0].Content = "mutated copy"
	if inputs[0].Content != "inspect files" {
		t.Fatalf("inputs[0].Content = %q, want original content", inputs[0].Content)
	}
}

func TestNormalizeToolCallBatchMovesInterleavedAssistantTextAfterBatch(t *testing.T) {
	inputs := []provider.Input{
		{Kind: provider.InputKindUserMessage, Content: "inspect files"},
		{Kind: provider.InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["a.go"]}`},
		{Kind: provider.InputKindAssistantMessage, Content: "runtime note"},
		{Kind: provider.InputKindToolResult, CallID: "call-1", ToolName: "read", Output: "a"},
		{Kind: provider.InputKindToolCall, CallID: "call-2", ToolName: "read", Arguments: `{"paths":["b.go"]}`},
		{Kind: provider.InputKindToolResult, CallID: "call-2", ToolName: "read", Output: "b"},
		{Kind: provider.InputKindAssistantMessage, Content: "after batch"},
	}

	got := normalizeToolCallBatch(inputs, []string{"call-1", "call-2"})
	if len(got) != len(inputs) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(inputs))
	}
	want := []struct {
		kind    provider.InputKind
		callID  string
		content string
	}{
		{kind: provider.InputKindUserMessage, content: "inspect files"},
		{kind: provider.InputKindToolCall, callID: "call-1"},
		{kind: provider.InputKindToolCall, callID: "call-2"},
		{kind: provider.InputKindToolResult, callID: "call-1"},
		{kind: provider.InputKindToolResult, callID: "call-2"},
		{kind: provider.InputKindAssistantMessage, content: "runtime note"},
		{kind: provider.InputKindAssistantMessage, content: "after batch"},
	}
	for idx, expected := range want {
		if got[idx].Kind != expected.kind || got[idx].CallID != expected.callID || got[idx].Content != expected.content {
			t.Fatalf("got[%d] = %#v, want kind=%q callID=%q content=%q", idx, got[idx], expected.kind, expected.callID, expected.content)
		}
	}
}

func TestNormalizeToolCallBatchesReturnsDetachedCopyWhenNoBatchesProvided(t *testing.T) {
	inputs := []provider.Input{
		{Kind: provider.InputKindUserMessage, Content: "inspect files"},
		{Kind: provider.InputKindAssistantMessage, Content: "done"},
	}

	got := normalizeToolCallBatches(inputs, nil)
	if len(got) != len(inputs) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(inputs))
	}

	got[1].Content = "mutated copy"
	if inputs[1].Content != "done" {
		t.Fatalf("inputs[1].Content = %q, want original content", inputs[1].Content)
	}
}
