package provider

import "testing"

func TestNormalizeConversationToolCallIDsRewritesPairsDeterministically(t *testing.T) {
	original := Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "chatcmpl-tool-94d45310586e92ee", ToolName: "read", Arguments: `{"paths":["README.md"]}`},
			{Kind: InputKindToolResult, CallID: "chatcmpl-tool-94d45310586e92ee", ToolName: "read", Output: "readme"},
			{Kind: InputKindToolCall, CallID: "call-local-shell", ToolName: "bash", Arguments: `{"cmd":"pwd"}`},
			{Kind: InputKindToolResult, CallID: "call-local-shell", ToolName: "bash", Output: "/tmp"},
		},
	}

	rewritten := normalizeConversationToolCallIDs(original)

	if rewritten.Inputs[1].CallID != "T00000001" || rewritten.Inputs[2].CallID != "T00000001" {
		t.Fatalf("first tool pair ids = (%q, %q), want T00000001", rewritten.Inputs[1].CallID, rewritten.Inputs[2].CallID)
	}
	if rewritten.Inputs[3].CallID != "T00000002" || rewritten.Inputs[4].CallID != "T00000002" {
		t.Fatalf("second tool pair ids = (%q, %q), want T00000002", rewritten.Inputs[3].CallID, rewritten.Inputs[4].CallID)
	}
	if original.Inputs[1].CallID != "chatcmpl-tool-94d45310586e92ee" {
		t.Fatalf("original first tool id mutated to %q", original.Inputs[1].CallID)
	}
	if original.Inputs[3].CallID != "call-local-shell" {
		t.Fatalf("original second tool id mutated to %q", original.Inputs[3].CallID)
	}
}

func TestBuildConversationToolCallIDAliasesProvidesReverseLookup(t *testing.T) {
	req := Request{
		SessionID: "session-1",
		TurnID:    "turn-1",
		AgentID:   "builder",
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "continue"},
			{Kind: InputKindToolCall, CallID: "call-read", ToolName: "read", Arguments: `{"paths":["README.md"]}`},
			{Kind: InputKindToolResult, CallID: "call-read", ToolName: "read", Output: "readme"},
			{Kind: InputKindToolCall, CallID: "call-patch", ToolName: "apply_patch", Arguments: "diff --git a/README.md b/README.md\n--- a/README.md\n+++ b/README.md\n@@ -1 +1 @@\n-old\n+new\n\n"},
		},
	}

	aliases := BuildConversationToolCallIDAliases(req)

	if got, ok := aliases.OriginalToProvider("call-read"); !ok || got != "T00000001" {
		t.Fatalf("OriginalToProvider(call-read) = (%q, %v), want (T00000001, true)", got, ok)
	}
	if got, ok := aliases.ProviderToOriginal("T00000002"); !ok || got != "call-patch" {
		t.Fatalf("ProviderToOriginal(T00000002) = (%q, %v), want (call-patch, true)", got, ok)
	}
	if _, ok := aliases.ProviderToOriginal("T00000009"); ok {
		t.Fatal("ProviderToOriginal(T00000009) = true, want false")
	}
}
