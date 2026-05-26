package app

import (
	"testing"

	"github.com/sageil/kodacode/internal/provider"
)

func TestStepToolCallCollectorBuildsCallsFromProviderEvents(t *testing.T) {
	collector := newStepToolCallCollector(true)
	collector.appendOpenAIReasoningDelta("inspect first. ")
	collector.appendToolCallDelta(provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-1",
		ToolName:   "read",
		InputDelta: `{"paths":`,
	})
	collector.appendToolCallDelta(provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-1",
		ToolName:   "read",
		InputDelta: `["app.go"]}`,
	})

	call := collector.completeToolCall(provider.Event{
		Kind:                   provider.EventKindToolCallDone,
		ToolCallID:             "call-1",
		ToolName:               "read",
		GoogleThoughtSignature: []byte("sig-1"),
	})
	if call.CallID != "call-1" || call.ToolName != "read" || call.Arguments != `{"paths":["app.go"]}` {
		t.Fatalf("call = %#v", call)
	}
	if got := string(call.GoogleThoughtSignature); got != "sig-1" {
		t.Fatalf("GoogleThoughtSignature = %q, want sig-1", got)
	}
	call.GoogleThoughtSignature[0] = 'X'
	next := collector.completeToolCall(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-2",
		ToolName:   "search",
	})
	if next.Arguments != "{}" {
		t.Fatalf("default arguments = %q, want {}", next.Arguments)
	}
	if call.OpenAIReasoningContent != "inspect first. " {
		t.Fatalf("first OpenAIReasoningContent = %q", call.OpenAIReasoningContent)
	}
	if next.OpenAIReasoningContent != "" {
		t.Fatalf("second OpenAIReasoningContent = %q, want empty", next.OpenAIReasoningContent)
	}
}

func TestStepToolCallCollectorPreservesCustomToolInput(t *testing.T) {
	collector := newStepToolCallCollector(false)
	firstDelta := "diff --git a/README.md b/README.md\n"
	secondDelta := "--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-old\n+new\n"
	collector.appendToolCallDelta(provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-1",
		ToolName:   "apply_patch",
		ToolKind:   provider.ToolKindCustom,
		InputDelta: firstDelta,
	})
	collector.appendToolCallDelta(provider.Event{
		Kind:       provider.EventKindToolCallDelta,
		ToolCallID: "call-1",
		ToolName:   "apply_patch",
		ToolKind:   provider.ToolKindCustom,
		InputDelta: secondDelta,
	})

	call := collector.completeToolCall(provider.Event{
		Kind:       provider.EventKindToolCallDone,
		ToolCallID: "call-1",
		ToolName:   "apply_patch",
		ToolKind:   provider.ToolKindCustom,
	})

	if call.ToolKind != provider.ToolKindCustom {
		t.Fatalf("ToolKind = %q, want custom", call.ToolKind)
	}
	if call.Arguments != firstDelta+secondDelta {
		t.Fatalf("Arguments = %q, want raw custom input", call.Arguments)
	}
}
