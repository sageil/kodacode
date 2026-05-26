package provider

import "testing"

func TestEstimateRequestTokenBreakdown(t *testing.T) {
	req := Request{
		Instructions:    "fallback prompt",
		CacheablePrefix: "Stable guidance.",
		DynamicSuffix:   "Dynamic note.",
		Tools: []Tool{{
			Name:        "read",
			Description: "Read a file.",
			InputSchema: `{"type":"object","properties":{"path":{"type":"string"}}}`,
		}},
		Inputs: []Input{
			{Kind: InputKindUserMessage, Content: "inspect the file"},
			{Kind: InputKindToolResult, ToolName: "read", Output: "file contents"},
		},
	}

	got := EstimateRequestTokenBreakdown(req)

	wantPrompt := EstimateTextTokens(JoinPromptSections("Stable guidance.", "Dynamic note."))
	wantConversation := EstimateInputTokens(req.Inputs[0]) + EstimateInputTokens(req.Inputs[1])
	wantToolName := EstimateTextTokens("read")
	wantToolDescription := EstimateTextTokens("Read a file.")
	wantToolSchema := EstimateTextTokens(`{"type":"object","properties":{"path":{"type":"string"}}}`)
	wantTotal := wantPrompt + wantConversation + wantToolName + wantToolDescription + wantToolSchema

	if got.PromptTokens != wantPrompt {
		t.Fatalf("PromptTokens = %d, want %d", got.PromptTokens, wantPrompt)
	}
	if got.ConversationTokens != wantConversation {
		t.Fatalf("ConversationTokens = %d, want %d", got.ConversationTokens, wantConversation)
	}
	if got.ToolNameTokens != wantToolName {
		t.Fatalf("ToolNameTokens = %d, want %d", got.ToolNameTokens, wantToolName)
	}
	if got.ToolDescriptionTokens != wantToolDescription {
		t.Fatalf("ToolDescriptionTokens = %d, want %d", got.ToolDescriptionTokens, wantToolDescription)
	}
	if got.ToolSchemaTokens != wantToolSchema {
		t.Fatalf("ToolSchemaTokens = %d, want %d", got.ToolSchemaTokens, wantToolSchema)
	}
	if got.ToolCount != 1 {
		t.Fatalf("ToolCount = %d, want 1", got.ToolCount)
	}
	if got.TotalTokens != wantTotal {
		t.Fatalf("TotalTokens = %d, want %d", got.TotalTokens, wantTotal)
	}
	if got.ToolSurfaceTokens() != wantToolName+wantToolDescription+wantToolSchema {
		t.Fatalf("ToolSurfaceTokens() = %d, want %d", got.ToolSurfaceTokens(), wantToolName+wantToolDescription+wantToolSchema)
	}
	if total := EstimateRequestTokens(req); total != wantTotal {
		t.Fatalf("EstimateRequestTokens() = %d, want %d", total, wantTotal)
	}
}

func TestEstimateTextTokensTreatsNonASCIIAsAtLeastOneTokenPerRune(t *testing.T) {
	if got := EstimateTextTokens("abcd"); got != 1 {
		t.Fatalf("EstimateTextTokens(ascii) = %d, want 1", got)
	}
	if got := EstimateTextTokens("日本語"); got != 3 {
		t.Fatalf("EstimateTextTokens(cjk) = %d, want 3", got)
	}
	if got := EstimateTextTokens("ab日本"); got != 3 {
		t.Fatalf("EstimateTextTokens(mixed) = %d, want 3", got)
	}
}
