package message_test

import (
	"testing"

	"github.com/sageil/kodacode/v1/internal/message"
)

func TestUnmarshalContent_Text(t *testing.T) {
	raw := `{"text":"hello"}`
	got, err := message.UnmarshalContent("text", raw)
	if err != nil {
		t.Fatal(err)
	}
	tc, ok := got.(message.TextContent)
	if !ok {
		t.Fatalf("want TextContent, got %T", got)
	}
	if tc.Text != "hello" {
		t.Errorf("UnmarshalContent(%q, %q) text = %q, want %q", "text", raw, tc.Text, "hello")
	}
}

func TestUnmarshalContent_ToolCall(t *testing.T) {
	raw := `{"id":"c1","name":"bash","arguments":"{\"command\":\"ls\"}"}`
	got, err := message.UnmarshalContent("tool_call", raw)
	if err != nil {
		t.Fatal(err)
	}
	tc, ok := got.(message.ToolCallContent)
	if !ok {
		t.Fatalf("want ToolCallContent, got %T", got)
	}
	if tc.ID != "c1" || tc.Name != "bash" {
		t.Errorf("UnmarshalContent(%q, %q) = %+v, want id=%q name=%q", "tool_call", raw, tc, "c1", "bash")
	}
}

func TestUnmarshalContent_ToolResult(t *testing.T) {
	raw := `{"tool_call_id":"c1","output":"ok"}`
	got, err := message.UnmarshalContent("tool_result", raw)
	if err != nil {
		t.Fatal(err)
	}
	tr, ok := got.(message.ToolResultContent)
	if !ok {
		t.Fatalf("want ToolResultContent, got %T", got)
	}
	if tr.ToolCallID != "c1" {
		t.Errorf("UnmarshalContent(%q, %q) ToolCallID = %q, want %q", "tool_result", raw, tr.ToolCallID, "c1")
	}
	if tr.Output != "ok" {
		t.Errorf("UnmarshalContent(%q, %q) Output = %q, want %q", "tool_result", raw, tr.Output, "ok")
	}
}

func TestUnmarshalContent_Reasoning(t *testing.T) {
	raw := `{"text":"thinking","tokens":42}`
	got, err := message.UnmarshalContent("reasoning", raw)
	if err != nil {
		t.Fatal(err)
	}
	rc, ok := got.(message.ReasoningContent)
	if !ok {
		t.Fatalf("want ReasoningContent, got %T", got)
	}
	if rc.Tokens != 42 {
		t.Errorf("UnmarshalContent(%q, %q) tokens = %d, want %d", "reasoning", raw, rc.Tokens, 42)
	}
}

func TestUnmarshalContent_File(t *testing.T) {
	raw := `{"path":"src/main.go","mime_type":"text/x-go","url":"file:///project/src/main.go"}`
	got, err := message.UnmarshalContent("file", raw)
	if err != nil {
		t.Fatal(err)
	}
	fc, ok := got.(message.FileContent)
	if !ok {
		t.Fatalf("want FileContent, got %T", got)
	}
	if fc.Path != "src/main.go" {
		t.Errorf("UnmarshalContent(%q, %q) Path = %q, want %q", "file", raw, fc.Path, "src/main.go")
	}
	if fc.MimeType != "text/x-go" {
		t.Errorf("UnmarshalContent(%q, %q) MimeType = %q, want %q", "file", raw, fc.MimeType, "text/x-go")
	}
}

func TestUnmarshalContent_UnknownType(t *testing.T) {
	_, err := message.UnmarshalContent("unknown", `{}`)
	if err == nil {
		t.Fatal("UnmarshalContent(\"unknown\", \"{}\") = nil error, want error for unknown type")
	}
}

func TestMarshalContent_RoundTrip(t *testing.T) {
	orig := message.TextContent{Text: "round trip"}
	raw, err := message.MarshalContent(orig)
	if err != nil {
		t.Fatal(err)
	}
	got, err := message.UnmarshalContent("text", raw)
	if err != nil {
		t.Fatal(err)
	}
	tc, ok := got.(message.TextContent)
	if !ok || tc.Text != "round trip" {
		t.Errorf("MarshalContent/UnmarshalContent round trip = %+v, want TextContent{Text: %q}", got, "round trip")
	}
}
