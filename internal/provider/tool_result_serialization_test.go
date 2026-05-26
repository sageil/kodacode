package provider

import "testing"

func TestSerializeToolResultForModelReturnsRawOutput(t *testing.T) {
	input := Input{
		Kind:     InputKindToolResult,
		CallID:   "call-1",
		ToolName: "read",
		Output:   "package main\n",
	}

	got := serializeToolResultForModel(input)
	want := "package main\n\n(patch planning: use apply_patch with a structured patch for existing-file source edits. Do not include read line-number prefixes in patch lines.)"
	if got != want {
		t.Fatalf("serializeToolResultForModel() = %q", got)
	}
}

func TestSerializeToolResultForModelAppendsWebSearchReuseHint(t *testing.T) {
	input := Input{
		Kind:     InputKindToolResult,
		CallID:   "call-4",
		ToolName: "web_search",
		Output:   "Search results for \"AAPL quote\"",
	}

	got := serializeToolResultForModel(input)
	want := "Search results for \"AAPL quote\"\n(web search reuse: reuse these candidate URLs when sufficient. If one fetched page fails, times out, or is too large, prefer another URL from these results before issuing another web_search unless the query itself needs to change.)"
	if got != want {
		t.Fatalf("serializeToolResultForModel() = %q", got)
	}
}

func TestSerializeToolResultForModelAppendsWebFetchRecoveryHintOnError(t *testing.T) {
	input := Input{
		Kind:     InputKindToolResult,
		CallID:   "call-5",
		ToolName: "web_fetch",
		Error:    "Get https://finance.yahoo.com/quote/AAPL: context deadline exceeded",
	}

	got := serializeToolResultForModel(input)
	want := "Get https://finance.yahoo.com/quote/AAPL: context deadline exceeded\n(web fetch recovery: if you already have candidate URLs from a recent web_search, prefer another returned URL before issuing another web_search. Search again only when the query itself needs to change.)"
	if got != want {
		t.Fatalf("serializeToolResultForModel() = %q", got)
	}
}

func TestSerializeToolResultForModelEncodesOutputAndError(t *testing.T) {
	input := Input{
		Kind:     InputKindToolResult,
		CallID:   "call-1",
		ToolName: "read",
		Output:   "package main\n",
		Error:    "read failed",
	}

	got := serializeToolResultForModel(input)
	want := `{"error":"read failed","output":"package main\n"}`
	if got != want {
		t.Fatalf("serializeToolResultForModel() = %q, want %q", got, want)
	}
}

func TestEstimateInputTokensUsesSerializedToolResult(t *testing.T) {
	input := Input{
		Kind:     InputKindToolResult,
		CallID:   "call-1",
		ToolName: "read",
		Output:   "package main\n",
	}

	got := EstimateInputTokens(input)
	want := EstimateTextTokens(input.ToolName) + EstimateTextTokens(serializeToolResultForModel(input))
	if got != want {
		t.Fatalf("EstimateInputTokens() = %d, want %d", got, want)
	}
}
