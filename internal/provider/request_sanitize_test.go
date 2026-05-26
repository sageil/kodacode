package provider

import (
	"strings"
	"testing"
)

func TestSanitizeMalformedToolReplayRequestReplacesMalformedToolPair(t *testing.T) {
	sanitized := sanitizeMalformedToolReplayRequest(Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "inspect"},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":["README.md"]`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Error: "`read` failed. JSON ended before the object was complete."},
		},
	})

	if len(sanitized.Inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2", len(sanitized.Inputs))
	}
	if sanitized.Inputs[1].Kind != InputKindAssistantMessage {
		t.Fatalf("input[1] = %#v, want assistant summary", sanitized.Inputs[1])
	}
	if got := sanitized.Inputs[1].Content; !strings.Contains(got, "read tool call") || !strings.Contains(got, "malformed JSON") || !strings.Contains(got, "Recorded tool error") {
		t.Fatalf("input[1].Content = %q, want malformed tool replay summary", got)
	}
}

func TestSanitizeMalformedToolReplayRequestKeepsValidBatchToolResultsAdjacent(t *testing.T) {
	sanitized := sanitizeMalformedToolReplayRequest(Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "inspect"},
			{Kind: InputKindToolCall, CallID: "call-locate-routes", ToolName: "locate", Arguments: `{"path":"src/routes"}`},
			{Kind: InputKindToolCall, CallID: "call-locate-middleware", ToolName: "locate", Arguments: `{"path":"src/middleware"}`},
			{Kind: InputKindToolCall, CallID: "call-read-bad", ToolName: "read", Arguments: `{"paths": client/src/stores}`},
			{Kind: InputKindToolResult, CallID: "call-locate-routes", ToolName: "locate", Output: "src/routes/index.ts"},
			{Kind: InputKindToolResult, CallID: "call-locate-middleware", ToolName: "locate", Output: "src/middleware/auth.ts"},
			{Kind: InputKindToolResult, CallID: "call-read-bad", ToolName: "read", Error: "`read` failed. \"paths\" has an unquoted string value."},
		},
	})

	if len(sanitized.Inputs) != 6 {
		t.Fatalf("len(inputs) = %d, want 6: %#v", len(sanitized.Inputs), sanitized.Inputs)
	}
	expected := []struct {
		kind   InputKind
		callID string
	}{
		{InputKindUserMessage, ""},
		{InputKindToolCall, "call-locate-routes"},
		{InputKindToolCall, "call-locate-middleware"},
		{InputKindToolResult, "call-locate-routes"},
		{InputKindToolResult, "call-locate-middleware"},
		{InputKindAssistantMessage, ""},
	}
	for idx, want := range expected {
		if sanitized.Inputs[idx].Kind != want.kind || sanitized.Inputs[idx].CallID != want.callID {
			t.Fatalf("input[%d] = %#v, want kind %q call_id %q", idx, sanitized.Inputs[idx], want.kind, want.callID)
		}
	}
	if got := sanitized.Inputs[5].Content; !strings.Contains(got, "read tool call") || !strings.Contains(got, "malformed JSON") || !strings.Contains(got, "Recorded tool error") {
		t.Fatalf("input[5].Content = %q, want malformed read summary", got)
	}
}

func TestSanitizeMalformedToolReplayRequestPreservesValidJSONToolReplay(t *testing.T) {
	req := Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "inspect"},
			{Kind: InputKindToolCall, CallID: "call-1", ToolName: "read", Arguments: `{"paths":123}`},
			{Kind: InputKindToolResult, CallID: "call-1", ToolName: "read", Error: "`read` failed. `paths` must be an array of strings; got number."},
		},
	}

	sanitized := sanitizeMalformedToolReplayRequest(req)

	if len(sanitized.Inputs) != len(req.Inputs) {
		t.Fatalf("len(inputs) = %d, want %d", len(sanitized.Inputs), len(req.Inputs))
	}
	if sanitized.Inputs[1].Kind != InputKindToolCall || sanitized.Inputs[1].Arguments != `{"paths":123}` {
		t.Fatalf("input[1] = %#v, want preserved tool call", sanitized.Inputs[1])
	}
	if sanitized.Inputs[2].Kind != InputKindToolResult {
		t.Fatalf("input[2] = %#v, want preserved tool result", sanitized.Inputs[2])
	}
}

func TestSanitizeMalformedToolReplayRequestPreservesCustomToolReplay(t *testing.T) {
	patch := "diff --git a/README.md b/README.md\n" +
		"--- a/README.md\n" +
		"+++ b/README.md\n" +
		"@@ -1 +1 @@\n" +
		"-old\n" +
		"+new\n"
	req := Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "apply"},
			{Kind: InputKindToolCall, ToolKind: ToolKindCustom, CallID: "call-1", ToolName: "apply_patch", Arguments: patch},
			{Kind: InputKindToolResult, ToolKind: ToolKindCustom, CallID: "call-1", ToolName: "apply_patch", Output: "Done!"},
		},
	}

	sanitized := sanitizeMalformedToolReplayRequest(req)

	if len(sanitized.Inputs) != len(req.Inputs) {
		t.Fatalf("len(inputs) = %d, want %d", len(sanitized.Inputs), len(req.Inputs))
	}
	if sanitized.Inputs[1].Kind != InputKindToolCall || sanitized.Inputs[1].ToolKind != ToolKindCustom || sanitized.Inputs[1].Arguments != patch {
		t.Fatalf("input[1] = %#v, want preserved custom tool call", sanitized.Inputs[1])
	}
}
