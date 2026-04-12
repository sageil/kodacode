package message_test

import (
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
)

func mustMarshal(t *testing.T, c message.Content) string {
	t.Helper()
	s, err := message.MarshalContent(c)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestToProviderMessages_TextAndToolCall(t *testing.T) {
	msgs := []repository.Message{
		{ID: "m1", SessionID: "s1", Role: "user"},
		{ID: "m2", SessionID: "s1", Role: "assistant"},
	}
	parts := map[string][]repository.MessagePart{
		"m1": {
			{Type: "text", Content: mustMarshal(t, message.TextContent{Text: "hello"})},
		},
		"m2": {
			{Type: "tool_call", Content: mustMarshal(t, message.ToolCallContent{
				ID: "c1", Name: "bash", Arguments: `{"command":"ls"}`,
			})},
		},
	}
	got := message.ToProviderMessages(msgs, parts)
	if len(got) != 2 {
		t.Fatalf("ToProviderMessages() len = %d, want 2", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("ToProviderMessages()[0].Role = %q, want %q", got[0].Role, "user")
	}
	if len(got[0].Parts) != 1 {
		t.Fatalf("ToProviderMessages()[0].Parts len = %d, want 1", len(got[0].Parts))
	}
	tp, ok := got[0].Parts[0].(provider.TextPart)
	if !ok {
		t.Fatalf("ToProviderMessages()[0].Parts[0] type = %T, want provider.TextPart", got[0].Parts[0])
	}
	if tp.Text != "hello" {
		t.Errorf("ToProviderMessages()[0].Parts[0].Text = %q, want %q", tp.Text, "hello")
	}
}

func TestToProviderMessages_CompactedToolResult(t *testing.T) {
	msgs := []repository.Message{
		{ID: "m1", SessionID: "s1", Role: "assistant"},
	}
	now := time.Now()
	summary := "[pruned: 42 lines of command output]"
	parts := map[string][]repository.MessagePart{
		"m1": {
			{Type: "tool_call", Content: mustMarshal(t, message.ToolCallContent{
				ID: "c1", Name: "bash", Arguments: "{}",
			})},
			{
				Type:        "tool_result",
				Content:     mustMarshal(t, message.ToolResultContent{ToolCallID: "c1", Output: summary}),
				CompactedAt: &now,
			},
		},
	}
	got := message.ToProviderMessages(msgs, parts)
	if len(got) != 1 {
		t.Fatalf("ToProviderMessages() len = %d, want 1", len(got))
	}
	var trp provider.ToolResultPart
	var found bool
	for _, p := range got[0].Parts {
		if tr, ok := p.(provider.ToolResultPart); ok {
			trp = tr
			found = true
		}
	}
	if !found {
		t.Fatal("ToProviderMessages(): no ToolResultPart found in parts")
	}
	if trp.Output != summary {
		t.Errorf("ToProviderMessages() ToolResultPart.Output = %q, want %q", trp.Output, summary)
	}
}

func TestToProviderMessages_SummaryMessage(t *testing.T) {
	msgs := []repository.Message{
		{ID: "m1", SessionID: "s1", Role: "assistant", Summary: true},
	}
	parts := map[string][]repository.MessagePart{
		"m1": {
			{Type: "text", Content: mustMarshal(t, message.TextContent{Text: "## Goal\nBuild kodacode"})},
		},
	}
	got := message.ToProviderMessages(msgs, parts)
	if len(got) != 1 {
		t.Fatalf("ToProviderMessages() len = %d, want 1", len(got))
	}
	if got[0].Role != "system" {
		t.Errorf("ToProviderMessages() summary message Role = %q, want %q", got[0].Role, "system")
	}
}

func TestToProviderMessages_EmptyParts(t *testing.T) {
	msgs := []repository.Message{
		{ID: "m1", SessionID: "s1", Role: "user"},
	}
	got := message.ToProviderMessages(msgs, map[string][]repository.MessagePart{})
	if len(got) != 1 {
		t.Fatalf("ToProviderMessages() len = %d, want 1", len(got))
	}
	if len(got[0].Parts) != 0 {
		t.Errorf("ToProviderMessages()[0].Parts len = %d, want 0", len(got[0].Parts))
	}
}

func TestToProviderMessages_FilePartsReplayAsAttachmentSummaries(t *testing.T) {
	msgs := []repository.Message{
		{ID: "m1", SessionID: "s1", Role: "user"},
	}
	parts := map[string][]repository.MessagePart{
		"m1": {
			{Type: "file", Content: mustMarshal(t, message.FileContent{
				Path:       "/tmp/diagram.png",
				MimeType:   "image/png",
				StorageKey: "abc123.png",
				Size:       42,
			})},
		},
	}

	got := message.ToProviderMessages(msgs, parts)
	if len(got) != 1 || len(got[0].Parts) != 1 {
		t.Fatalf("ToProviderMessages() shape = %+v, want one message with one part", got)
	}
	part, ok := got[0].Parts[0].(provider.TextPart)
	if !ok {
		t.Fatalf("replayed file part type = %T, want provider.TextPart", got[0].Parts[0])
	}
	want := message.AttachmentSummary(message.FileContent{
		Path:       "/tmp/diagram.png",
		MimeType:   "image/png",
		StorageKey: "abc123.png",
		Size:       42,
	})
	if part.Text != want {
		t.Fatalf("replayed file summary = %q, want %q", part.Text, want)
	}
}

func TestEstimateTokens(t *testing.T) {
	parts := []repository.MessagePart{
		{Content: "1234"},     // 4 chars = 1 token
		{Content: "12345678"}, // 8 chars = 2 tokens
	}
	got := message.EstimateTokens(parts)
	if got != 3 {
		t.Errorf("EstimateTokens() = %d, want 3", got)
	}
}
