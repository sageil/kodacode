package service // NOT service_test — must access unexported resolveCompactionConfig

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/agent"
	"github.com/sageil/kodacode/v1/internal/config"
	"github.com/sageil/kodacode/v1/internal/message"
	"github.com/sageil/kodacode/v1/internal/pipeline"
	"github.com/sageil/kodacode/v1/internal/provider"
	"github.com/sageil/kodacode/v1/internal/repository"
)

func TestCompactionThreshold_PerModelOverride(t *testing.T) {
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.8),
		Models: map[string]config.ModelSessionConfig{
			"anthropic/claude-sonnet-4-6": {
				CompactionThreshold: f64p(0.85),
			},
		},
	}
	cc := resolveCompactionConfig(cfg, "anthropic", "claude-sonnet-4-6", 1048576)
	if cc.threshold != 0.85 {
		t.Errorf("threshold = %f, want 0.85", cc.threshold)
	}
}

func TestCompactionThreshold_FallsBackToGlobal(t *testing.T) {
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.7),
	}
	cc := resolveCompactionConfig(cfg, "openai", "gpt-4o", 1048576)
	if cc.threshold != 0.7 {
		t.Errorf("threshold = %f, want 0.7", cc.threshold)
	}
}

func TestCompactionConfig_UsesGlobalDefaults(t *testing.T) {
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.8),
		CompactionKeepTurns: intp(10),
		PruneProtectTokens:  intp(40000),
		PruneMinSavings:     intp(20000),
	}
	cc := resolveCompactionConfig(cfg, "any", "model", 1048576)
	if cc.threshold != 0.8 {
		t.Errorf("threshold = %f, want 0.8", cc.threshold)
	}
	if cc.keepTurns != 10 {
		t.Errorf("keepTurns = %d, want 10", cc.keepTurns)
	}
	if cc.pruneProtect != 40000 {
		t.Errorf("pruneProtect = %d, want 40000", cc.pruneProtect)
	}
	if cc.pruneMinSavings != 20000 {
		t.Errorf("pruneMinSavings = %d, want 20000", cc.pruneMinSavings)
	}
}

func TestCompactionConfig_PerModelOverridesAll(t *testing.T) {
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.8),
		CompactionKeepTurns: intp(10),
		PruneProtectTokens:  intp(40000),
		PruneMinSavings:     intp(20000),
		Models: map[string]config.ModelSessionConfig{
			"test/model": {
				CompactionThreshold: f64p(0.9),
				CompactionKeepTurns: intp(5),
				PruneProtectTokens:  intp(50000),
				PruneMinSavings:     intp(30000),
			},
		},
	}
	cc := resolveCompactionConfig(cfg, "test", "model", 1048576)
	if cc.threshold != 0.9 {
		t.Errorf("threshold = %f, want 0.9", cc.threshold)
	}
	if cc.keepTurns != 5 {
		t.Errorf("keepTurns = %d, want 5", cc.keepTurns)
	}
	if cc.pruneProtect != 50000 {
		t.Errorf("pruneProtect = %d, want 50000", cc.pruneProtect)
	}
	if cc.pruneMinSavings != 30000 {
		t.Errorf("pruneMinSavings = %d, want 30000", cc.pruneMinSavings)
	}
}

func TestCompactionThreshold_SmallModelFloor(t *testing.T) {
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.4),
	}
	cc128 := resolveCompactionConfig(cfg, "github-copilot", "gpt-5-mini", 128000)
	if cc128.threshold != 0.60 {
		t.Errorf("128K model threshold = %f, want 0.60", cc128.threshold)
	}
	cc32 := resolveCompactionConfig(cfg, "local", "small", 32000)
	if cc32.threshold != 0.70 {
		t.Errorf("32K model threshold = %f, want 0.70", cc32.threshold)
	}
	cc1m := resolveCompactionConfig(cfg, "google", "gemini-3-pro", 1048576)
	if cc1m.threshold != 0.4 {
		t.Errorf("1M model threshold = %f, want 0.4", cc1m.threshold)
	}
}

func TestCompactionThreshold_PerModelOverrideBeatsFloor(t *testing.T) {
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.3),
		Models: map[string]config.ModelSessionConfig{
			"small/model": {CompactionThreshold: f64p(0.75)},
		},
	}
	cc := resolveCompactionConfig(cfg, "small", "model", 64000)
	if cc.threshold != 0.75 {
		t.Errorf("per-model override threshold = %f, want 0.75", cc.threshold)
	}
}

func TestCompactionConfig_FallsBackToBareModelKey(t *testing.T) {
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.8),
		Models: map[string]config.ModelSessionConfig{
			"gpt-4o": {CompactionThreshold: f64p(0.9)},
		},
	}

	cc := resolveCompactionConfig(cfg, "openai", "gpt-4o", 1048576)
	if cc.threshold != 0.9 {
		t.Errorf("threshold = %f, want 0.9", cc.threshold)
	}
}

func TestSanitizeToolPairs_DropsOrphanedToolCalls(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.TextPart{Text: "I'll help"},
			provider.ToolCallPart{ID: "call_1", Name: "read", Arguments: `{}`},
			provider.ToolCallPart{ID: "call_2", Name: "write", Arguments: `{}`},
		}},
		// Only call_1 has a result; call_2 is orphaned
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "call_1", Output: "ok"},
		}},
	}
	result := sanitizeToolPairs(msgs)
	// Assistant message should keep text + call_1, drop call_2
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	assistantParts := result[1].Parts
	if len(assistantParts) != 2 {
		t.Fatalf("expected 2 assistant parts (text + call_1), got %d", len(assistantParts))
	}
	tc, ok := assistantParts[1].(provider.ToolCallPart)
	if !ok || tc.ID != "call_1" {
		t.Errorf("expected remaining tool call to be call_1, got %+v", assistantParts[1])
	}
}

func TestSanitizeToolPairs_DropsOrphanedToolResults(t *testing.T) {
	// Tool result without matching tool call (truncation cut the call)
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "call_orphan", Output: "data"},
		}},
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "continue"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "ok"}}},
	}
	result := sanitizeToolPairs(msgs)
	// Orphaned tool_result message should be dropped
	if len(result) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("expected first message role=user, got %s", result[0].Role)
	}
	if _, ok := result[0].Parts[0].(provider.TextPart); !ok {
		t.Errorf("expected first message to be text, got %T", result[0].Parts[0])
	}
}

func TestSanitizeToolPairs_NoOpWhenValid(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.ToolCallPart{ID: "call_1", Name: "read", Arguments: `{}`},
		}},
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "call_1", Output: "ok"},
		}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "done"}}},
	}
	result := sanitizeToolPairs(msgs)
	if len(result) != 4 {
		t.Fatalf("expected 4 messages unchanged, got %d", len(result))
	}
}

func TestSanitizeToolPairs_DropsEntireAssistantIfAllOrphaned(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.ToolCallPart{ID: "call_1", Name: "read", Arguments: `{}`},
		}},
		// No tool result for call_1
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "next"}}},
	}
	result := sanitizeToolPairs(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages (assistant dropped), got %d", len(result))
	}
}

func TestSanitizeToolPairs_DropsMalformedToolCallArguments(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "review it"}}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.TextPart{Text: "I'll delegate this."},
			provider.ToolCallPart{
				ID:        "call_1",
				Name:      "subagent",
				Arguments: `{"agent_id":"explorer","task":"backend"}{"agent_id":"explorer","task":"frontend"}`,
			},
		}},
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "call_1", Output: "tool error [subagent]: invalid arguments"},
		}},
	}

	result := sanitizeToolPairs(msgs)
	if len(result) != 2 {
		t.Fatalf("expected 2 messages after dropping malformed tool call pair, got %d", len(result))
	}
	if result[1].Role != "assistant" {
		t.Fatalf("second message role = %q, want assistant", result[1].Role)
	}
	if len(result[1].Parts) != 1 {
		t.Fatalf("assistant parts len = %d, want 1", len(result[1].Parts))
	}
	if _, ok := result[1].Parts[0].(provider.TextPart); !ok {
		t.Fatalf("assistant part type = %T, want provider.TextPart", result[1].Parts[0])
	}
}

func TestRejectedToolCallIDs_IdentifiesHallucinatedCalls(t *testing.T) {
	executions := []toolExecution{
		{call: provider.ToolCall{ID: "c1", Name: "bash"}, output: "files"},
		{call: provider.ToolCall{ID: "c2", Name: "subagent"}, output: "not available"},
	}
	tools := []provider.Tool{{Name: "bash"}, {Name: "read"}}
	rejected := rejectedToolCallIDs(executions, tools)
	if len(rejected) != 1 || !rejected["c2"] {
		t.Fatalf("expected {c2}, got %v", rejected)
	}
}

func TestRejectedToolCallIDs_NoneWhenAllAllowed(t *testing.T) {
	executions := []toolExecution{
		{call: provider.ToolCall{ID: "c1", Name: "bash"}, output: "ok"},
	}
	rejected := rejectedToolCallIDs(executions, []provider.Tool{{Name: "bash"}})
	if len(rejected) != 0 {
		t.Fatalf("expected 0 rejected, got %d", len(rejected))
	}
}

func TestStripUnknownToolCalls_RemovesUnknownTools(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.TextPart{Text: "I'll use bash and subagent"},
			provider.ToolCallPart{ID: "c1", Name: "bash", Arguments: `{"command":"ls"}`},
			provider.ToolCallPart{ID: "c2", Name: "subagent", Arguments: `{"agent_id":"explorer"}`},
		}},
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "c1", Output: "file1"},
			provider.ToolResultPart{ToolCallID: "c2", Output: "rejected"},
		}},
	}
	// Only "bash" is in the tool definitions; "subagent" should be stripped.
	tools := []provider.Tool{{Name: "bash"}, {Name: "read"}}
	result, n := stripUnknownToolCalls(msgs, tools)
	if n != 1 {
		t.Fatalf("expected 1 stripped, got %d", n)
	}
	// Assistant should have text + bash call, no subagent call.
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}
	assistantParts := result[1].Parts
	for _, p := range assistantParts {
		if tc, ok := p.(provider.ToolCallPart); ok && tc.Name == "subagent" {
			t.Fatal("subagent tool call should have been stripped")
		}
	}
	// Tool result for subagent should also be stripped.
	for _, p := range result[2].Parts {
		if tr, ok := p.(provider.ToolResultPart); ok && tr.ToolCallID == "c2" {
			t.Fatal("tool result for stripped subagent call should have been removed")
		}
	}
}

func TestStripUnknownToolCalls_NoOpWhenAllKnown(t *testing.T) {
	msgs := []provider.Message{
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.ToolCallPart{ID: "c1", Name: "bash", Arguments: `{}`},
		}},
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "c1", Output: "ok"},
		}},
	}
	tools := []provider.Tool{{Name: "bash"}}
	result, n := stripUnknownToolCalls(msgs, tools)
	if n != 0 {
		t.Fatalf("expected 0 stripped, got %d", n)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 messages unchanged, got %d", len(result))
	}
}

func TestIsInvalidToolCallArgsError(t *testing.T) {
	tests := []struct {
		err  string
		want bool
	}{
		{`openai: stream: POST "http://localhost:11434/v1/chat/completions": 400 Bad Request {"message":"invalid tool call arguments"}`, true},
		{`400 Bad Request: invalid tool call`, true},
		{`openai: stream: 400 invalid parameter stream_options`, false},
		{`429 Too Many Requests`, false},
		{`500 Internal Server Error`, false},
	}
	for _, tt := range tests {
		if got := isInvalidToolCallArgsError(tt.err); got != tt.want {
			t.Errorf("isInvalidToolCallArgsError(%q) = %v, want %v", tt.err, got, tt.want)
		}
	}
}

func TestIsStreamOptionsError_ExcludesToolCallArgs(t *testing.T) {
	// "invalid tool call arguments" should NOT match isStreamOptionsError
	// even though it contains "400" and "invalid".
	errMsg := `400 Bad Request {"message":"invalid tool call arguments"}`
	if isStreamOptionsError(errMsg) {
		t.Fatal("isStreamOptionsError should not match 'invalid tool call arguments'")
	}
	// But actual stream_options errors should still match.
	if !isStreamOptionsError(`400 Bad Request: invalid parameter stream_options`) {
		t.Fatal("isStreamOptionsError should match actual stream_options errors")
	}
}

func TestStripUnknownToolCalls_DropsEntireMessageIfAllUnknown(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "go"}}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.ToolCallPart{ID: "c1", Name: "subagent", Arguments: `{}`},
		}},
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{ToolCallID: "c1", Output: "rejected"},
		}},
	}
	tools := []provider.Tool{{Name: "bash"}}
	result, n := stripUnknownToolCalls(msgs, tools)
	if n != 1 {
		t.Fatalf("expected 1 stripped, got %d", n)
	}
	// Both the assistant (all tool calls removed) and the tool result message should be dropped.
	if len(result) != 1 {
		t.Fatalf("expected 1 message (user only), got %d", len(result))
	}
	if result[0].Role != "user" {
		t.Fatalf("expected remaining message to be user, got %q", result[0].Role)
	}
}

func TestSafeTruncateMessages_KeepsTurnsNotMessages(t *testing.T) {
	// Turn 1: user text + assistant tool_call + user tool_result (x5) + assistant text
	// Turn 2: user text + assistant text
	// With keepTurns=1, should keep only turn 2 (2 messages), not just 2 messages from end.
	msgs := []provider.Message{
		// Turn 1: user prompt
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "turn 1"}}},
		// Turn 1: 5 tool call/result pairs
		{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: "c1", Name: "read", Arguments: `{}`}}},
		{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: "c1", Output: "r1"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: "c2", Name: "read", Arguments: `{}`}}},
		{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: "c2", Output: "r2"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: "c3", Name: "read", Arguments: `{}`}}},
		{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: "c3", Output: "r3"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: "c4", Name: "read", Arguments: `{}`}}},
		{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: "c4", Output: "r4"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: "c5", Name: "read", Arguments: `{}`}}},
		{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: "c5", Output: "r5"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "turn 1 done"}}},
		// Turn 2: simple exchange
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "turn 2"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "turn 2 done"}}},
	}

	result := safeTruncateMessages(msgs, 1)

	// Should keep only turn 2: user("turn 2") + assistant("turn 2 done")
	if len(result) != 2 {
		t.Fatalf("keepTurns=1: got %d messages, want 2", len(result))
	}
	tp, ok := result[0].Parts[0].(provider.TextPart)
	if !ok || tp.Text != "turn 2" {
		t.Errorf("first message should be 'turn 2', got %v", result[0].Parts[0])
	}
}

func TestSafeTruncateMessages_ZeroKeepsTurnsReturnsAll(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "hi"}}},
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "bye"}}},
	}

	result := safeTruncateMessages(msgs, 0)
	if len(result) != len(msgs) {
		t.Fatalf("keepTurns=0: got %d messages, want %d (all)", len(result), len(msgs))
	}
}

func TestSafeTruncateMessages_KeepsTwoTurns(t *testing.T) {
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "turn 1"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "response 1"}}},
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "turn 2"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "response 2"}}},
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "turn 3"}}},
		{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "response 3"}}},
	}

	result := safeTruncateMessages(msgs, 2)
	if len(result) != 4 {
		t.Fatalf("keepTurns=2: got %d messages, want 4", len(result))
	}
	tp, ok := result[0].Parts[0].(provider.TextPart)
	if !ok || tp.Text != "turn 2" {
		t.Errorf("first message should be 'turn 2', got %v", result[0].Parts[0])
	}
}

func TestSafeTruncateMessages_FewTurnsWithManyToolCalls(t *testing.T) {
	// 3 turns, each with 10 tool pairs = 3 boundaries, 63 messages.
	// With keepTurns=5, all 3 turns should be kept (no truncation).
	var msgs []provider.Message
	for turn := range 3 {
		msgs = append(msgs, provider.Message{
			Role:  "user",
			Parts: []provider.MessagePart{provider.TextPart{Text: fmt.Sprintf("turn %d", turn+1)}},
		})
		for i := range 10 {
			id := fmt.Sprintf("c%d_%d", turn, i)
			msgs = append(msgs,
				provider.Message{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: id, Name: "read", Arguments: `{}`}}},
				provider.Message{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: id, Output: "ok"}}},
			)
		}
	}

	result := safeTruncateMessages(msgs, 5)
	if len(result) != len(msgs) {
		t.Fatalf("keepTurns=5 with 3 turns: got %d messages, want %d (all kept)", len(result), len(msgs))
	}
}

func TestSafeTruncateMessages_MidTurnFallback(t *testing.T) {
	// Single turn with 1 user prompt + 20 tool_call/result pairs.
	// With keepTurns=3, there's only 1 boundary so we can't cut by turns.
	// The fallback should keep the first user prompt + last keepTurns*2=6
	// messages, starting from an assistant message.
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "do everything"}}},
	}
	for i := range 20 {
		id := fmt.Sprintf("c%d", i)
		msgs = append(msgs,
			provider.Message{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: id, Name: "read", Arguments: `{}`}}},
			provider.Message{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: id, Output: "ok"}}},
		)
	}
	// 41 messages total (1 user + 20*2 tool pairs)

	result := safeTruncateMessages(msgs, 3)

	// Should be: first user prompt + last 6 messages (3 tool pairs)
	if len(result) > 8 {
		t.Fatalf("mid-turn fallback: got %d messages, want ≤8 (user prompt + ~6 tail)", len(result))
	}
	if len(result) < 4 {
		t.Fatalf("mid-turn fallback: got %d messages, want ≥4", len(result))
	}

	// First message should be the original user prompt.
	tp, ok := result[0].Parts[0].(provider.TextPart)
	if !ok || tp.Text != "do everything" {
		t.Errorf("first message should be original prompt, got %v", result[0].Parts[0])
	}

	// Last message should be a tool_result (the most recent one).
	last := result[len(result)-1]
	if _, ok := last.Parts[0].(provider.ToolResultPart); !ok {
		t.Errorf("last message should be tool_result, got %T", last.Parts[0])
	}
}

func TestCompactionPrompt_ContainsAllHeadings(t *testing.T) {
	prompt := agent.CompactionPrompt()
	headings := []string{
		"## Goal",
		"## Instructions",
		"## Discoveries",
		"## Accomplished",
		"## Relevant Files",
	}
	for _, h := range headings {
		if !strings.Contains(prompt, h) {
			t.Errorf("CompactionPrompt() missing heading: %s", h)
		}
	}
}

func TestBuildTurnMessages_ExcludesSummaryMessagesAndReturnsLatestSummary(t *testing.T) {
	msgs := []repository.Message{
		{ID: "m1", SessionID: "s1", Role: "assistant", Summary: true},
		{ID: "m2", SessionID: "s1", Role: "user"},
	}
	parts := map[string][]repository.MessagePart{
		"m1": {
			{Type: "text", Content: mustMarshalText(t, "## Goal\nOld context")},
		},
		"m2": {
			{Type: "text", Content: mustMarshalText(t, "continue")},
		},
	}

	providerMsgs, summaryText := buildTurnMessages(msgs, parts)
	if len(providerMsgs) != 1 {
		t.Fatalf("buildTurnMessages() messages = %d, want 1", len(providerMsgs))
	}
	if providerMsgs[0].Role != "user" {
		t.Fatalf("buildTurnMessages() first role = %q, want user", providerMsgs[0].Role)
	}
	if summaryText != "## Conversation Summary\n## Goal\nOld context" {
		t.Fatalf("buildTurnMessages() summary = %q", summaryText)
	}
}

func TestInjectSummary_PreservesExistingVolatileContext(t *testing.T) {
	req := &pipeline.TurnRequest{
		SummaryText: "## Conversation Summary\nold summary",
		SystemParts: []string{
			"stable",
			"semi",
			"# USER OVERRIDES — MANDATORY\n- keep tests unchanged\n\n## Conversation Summary\nold summary\n\nUncommitted changes:\nM internal/service/compaction.go",
		},
	}

	injectSummary(req, "new summary")

	want := "# USER OVERRIDES — MANDATORY\n- keep tests unchanged\n\n## Conversation Summary\nnew summary\n\nUncommitted changes:\nM internal/service/compaction.go"
	if req.SystemParts[2] != want {
		t.Fatalf("injectSummary() volatile part = %q, want %q", req.SystemParts[2], want)
	}
}

func TestMaybeCompact_PruneOnlyReloadsRequestMessages(t *testing.T) {
	repo := &compactionRepoStub{
		messages: []repository.Message{
			{ID: "m1", SessionID: "s1", Role: "user"},
			{ID: "m2", SessionID: "s1", Role: "assistant"},
			{ID: "m3", SessionID: "s1", Role: "user"},
		},
		parts: []repository.MessagePart{
			{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "start")},
			{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "tool_call", Content: mustMarshalToolCall(t, "call_1", "read")},
			{ID: "p3", MessageID: "m3", SessionID: "s1", Type: "tool_result", Content: mustMarshalToolResult(t, "call_1", strings.Repeat("x", 400))},
		},
	}

	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Model:     provider.Model{ID: "test-model", ContextSize: 100},
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "start"}}},
			{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: "call_1", Name: "read", Arguments: `{}`}}},
			{Role: "user", Parts: []provider.MessagePart{provider.ToolResultPart{ToolCallID: "call_1", Output: strings.Repeat("x", 400)}}},
		},
	}
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.5),
		CompactionKeepTurns: intp(0),
		PruneProtectTokens:  intp(0),
		PruneMinSavings:     intp(1),
	}

	if err := maybeCompact(context.Background(), cfg, repo, func(name string) bool { return name == "read" }, utilityProvider{prov: testProvider{id: "test-provider"}, modelID: "test-utility"}, nil, req, 100, nil, nil); err != nil {
		t.Fatalf("maybeCompact() error = %v", err)
	}

	got := req.Messages[len(req.Messages)-1]
	tr, ok := got.Parts[0].(provider.ToolResultPart)
	if !ok {
		t.Fatalf("last part type = %T, want ToolResultPart", got.Parts[0])
	}
	if tr.Output == strings.Repeat("x", 400) {
		t.Fatal("expected tool result output to be refreshed from pruned repository state")
	}
	if !strings.HasPrefix(tr.Output, "[pruned:") {
		t.Fatalf("expected prune summary, got %q", tr.Output)
	}
}

func TestMaybeCompact_FallbackPreservesExistingSummary(t *testing.T) {
	repo := &compactionRepoStub{
		messages: []repository.Message{
			{ID: "m1", SessionID: "s1", Role: "assistant", Summary: true},
			{ID: "m2", SessionID: "s1", Role: "user"},
		},
		parts: []repository.MessagePart{
			{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "## Goal\nPrior summary")},
			{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "text", Content: mustMarshalText(t, strings.Repeat("current turn ", 20))},
		},
	}

	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Model:     provider.Model{ID: "test-model", ContextSize: 100},
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: strings.Repeat("current turn ", 20)}}},
		},
	}
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.5),
		CompactionKeepTurns: intp(1),
		PruneProtectTokens:  intp(0),
		PruneMinSavings:     intp(1000),
	}

	if err := maybeCompact(context.Background(), cfg, repo, nil, utilityProvider{prov: testProvider{id: "test-provider", chatErr: errors.New("summary failed")}, modelID: "test-utility"}, nil, req, 100, nil, nil); err != nil {
		t.Fatalf("maybeCompact() error = %v", err)
	}

	wantSummary := "## Conversation Summary\n## Goal\nPrior summary"
	if req.SummaryText != wantSummary {
		t.Fatalf("SummaryText = %q, want %q", req.SummaryText, wantSummary)
	}
	if len(req.SystemParts) < 3 {
		t.Fatalf("SystemParts len = %d, want at least 3", len(req.SystemParts))
	}
	if req.SystemParts[2] != wantSummary {
		t.Fatalf("SystemParts[2] = %q, want %q", req.SystemParts[2], wantSummary)
	}
}

func TestMaybeCompact_PreservesWorkflowStateAcrossTruncation(t *testing.T) {
	repo := &compactionRepoStub{
		messages: []repository.Message{
			{ID: "m1", SessionID: "s1", Role: "user"},
			{ID: "m2", SessionID: "s1", Role: "assistant"},
			{ID: "m3", SessionID: "s1", Role: "user"},
			{ID: "m4", SessionID: "s1", Role: "user"},
		},
		parts: []repository.MessagePart{
			{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "plan this change")},
			{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "tool_call", Content: mustMarshalToolCallWithArgs(t, "planner-1", "subagent", `{"agent_id":"planner","task":"plan"}`)},
			{ID: "p3", MessageID: "m3", SessionID: "s1", Type: "text", Content: mustMarshalText(t, encodePlanApprovalDecision(planApprovalApproved, planApprovalProceedOption))},
			{ID: "p4", MessageID: "m4", SessionID: "s1", Type: "text", Content: mustMarshalText(t, strings.Repeat("current turn ", 20))},
		},
	}

	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Model:     provider.Model{ID: "test-model", ContextSize: 100},
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "plan this change"}}},
			{Role: "assistant", Parts: []provider.MessagePart{provider.ToolCallPart{ID: "planner-1", Name: "subagent", Arguments: `{"agent_id":"planner","task":"plan"}`}}},
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: encodePlanApprovalDecision(planApprovalApproved, planApprovalProceedOption)}}},
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: strings.Repeat("current turn ", 20)}}},
		},
	}
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.5),
		CompactionKeepTurns: intp(1),
		PruneProtectTokens:  intp(0),
		PruneMinSavings:     intp(1000),
	}

	if err := maybeCompact(context.Background(), cfg, repo, nil, utilityProvider{prov: testProvider{id: "test-provider", chatErr: errors.New("summary failed")}, modelID: "test-utility"}, nil, req, 100, nil, nil); err != nil {
		t.Fatalf("maybeCompact() error = %v", err)
	}

	if req.Workflow == nil {
		t.Fatal("Workflow = nil, want primed workflow state")
	}
	if req.Workflow.Phase != pipeline.WorkflowPhaseApproved {
		t.Fatalf("Workflow.Phase = %q, want approved", req.Workflow.Phase)
	}
	if req.Workflow.Plan.EffectiveStatus != pipeline.WorkflowApprovalApproved {
		t.Fatalf("Workflow.Plan.EffectiveStatus = %q, want approved", req.Workflow.Plan.EffectiveStatus)
	}
	for _, msg := range req.Messages {
		if strings.Contains(provider.TextFromParts(msg.Parts), planApprovalDecisionMarkerTag) {
			t.Fatalf("truncated messages should not retain explicit plan approval markers: %#v", req.Messages)
		}
	}
}

func TestMaybeCompact_FallsBackToAlternateUtilityCandidate(t *testing.T) {
	repo := &compactionRepoStub{
		messages: []repository.Message{
			{ID: "m1", SessionID: "s1", Role: "user"},
			{ID: "m2", SessionID: "s1", Role: "assistant"},
			{ID: "m3", SessionID: "s1", Role: "user"},
		},
		parts: []repository.MessagePart{
			{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, strings.Repeat("first turn ", 40))},
			{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "text", Content: mustMarshalText(t, strings.Repeat("assistant turn ", 40))},
			{ID: "p3", MessageID: "m3", SessionID: "s1", Type: "text", Content: mustMarshalText(t, strings.Repeat("current turn ", 40))},
		},
	}

	req := &pipeline.TurnRequest{
		SessionID:  "s1",
		ProviderID: "primary",
		Model:      provider.Model{ID: "primary-model", ContextSize: 100},
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: strings.Repeat("first turn ", 40)}}},
			{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: strings.Repeat("assistant turn ", 40)}}},
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: strings.Repeat("current turn ", 40)}}},
		},
	}
	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.5),
		CompactionKeepTurns: intp(1),
		PruneProtectTokens:  intp(0),
		PruneMinSavings:     intp(1000),
	}
	tracker := newUtilityHealthTracker()
	summary := strings.TrimSpace(strings.Repeat("Durable summary content. ", 12))
	utility := utilityProvider{
		prov:    testProvider{id: "cheap", chatErr: errors.New("404 model not found")},
		modelID: "cheap-model",
		alternates: []utilityProvider{{
			prov:    testProvider{id: "good", response: summary},
			modelID: "good-model",
		}},
	}

	if err := maybeCompact(context.Background(), cfg, repo, nil, utility, nil, req, 100, nil, tracker); err != nil {
		t.Fatalf("maybeCompact() error = %v", err)
	}
	if !strings.Contains(req.SummaryText, summary) {
		t.Fatalf("SummaryText = %q, want to contain fallback summary", req.SummaryText)
	}
	if !tracker.isUnavailableAt(utility.withoutAlternates(), time.Now()) {
		t.Fatal("expected permanently failing utility model to be marked unavailable")
	}
}

func TestPruneToolOutputs_ProtectsRecentTurnAndPrunesOldReadOnlyResults(t *testing.T) {
	oldOutput := strings.Repeat("a", 400)
	recentOutput := strings.Repeat("b", 400)
	repo := &compactionRepoStub{
		messages: []repository.Message{
			{ID: "m1", SessionID: "s1", Role: "user"},
			{ID: "m2", SessionID: "s1", Role: "assistant"},
			{ID: "m3", SessionID: "s1", Role: "user"},
			{ID: "m4", SessionID: "s1", Role: "user"},
			{ID: "m5", SessionID: "s1", Role: "assistant"},
			{ID: "m6", SessionID: "s1", Role: "user"},
		},
		parts: []repository.MessagePart{
			{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "first turn")},
			{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "tool_call", Content: mustMarshalToolCall(t, "call_1", "read")},
			{ID: "p3", MessageID: "m3", SessionID: "s1", Type: "tool_result", Content: mustMarshalToolResult(t, "call_1", oldOutput)},
			{ID: "p4", MessageID: "m4", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "second turn")},
			{ID: "p5", MessageID: "m5", SessionID: "s1", Type: "tool_call", Content: mustMarshalToolCall(t, "call_2", "read")},
			{ID: "p6", MessageID: "m6", SessionID: "s1", Type: "tool_result", Content: mustMarshalToolResult(t, "call_2", recentOutput)},
		},
	}

	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Model:     provider.Model{ID: "test-model"},
	}
	cfg := &config.SessionConfig{
		CompactionKeepTurns: intp(1),
		PruneProtectTokens:  intp(0),
		PruneMinSavings:     intp(1),
	}

	if err := pruneToolOutputs(context.Background(), cfg, repo, func(name string) bool { return name == "read" }, req); err != nil {
		t.Fatalf("pruneToolOutputs() error = %v", err)
	}

	oldPart := repo.part("p3")
	recentPart := repo.part("p6")
	oldResult := mustUnmarshalToolResult(t, oldPart.Content)
	recentResult := mustUnmarshalToolResult(t, recentPart.Content)

	if oldResult.Output == oldOutput {
		t.Fatal("expected old read-only result to be pruned")
	}
	if oldPart.CompactedAt == nil {
		t.Fatal("expected old pruned part to record CompactedAt")
	}
	if recentResult.Output != recentOutput {
		t.Fatalf("recent protected result = %q, want original output", recentResult.Output)
	}
}

func f64p(v float64) *float64 { return &v }
func intp(v int) *int         { return &v }

func mustMarshalText(t *testing.T, text string) string {
	t.Helper()
	content, err := message.MarshalContent(message.TextContent{Text: text})
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func mustMarshalToolCall(t *testing.T, id, name string) string {
	return mustMarshalToolCallWithArgs(t, id, name, `{}`)
}

func mustMarshalToolCallWithArgs(t *testing.T, id, name, args string) string {
	t.Helper()
	content, err := message.MarshalContent(message.ToolCallContent{ID: id, Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func mustMarshalToolResult(t *testing.T, id, output string) string {
	t.Helper()
	content, err := message.MarshalContent(message.ToolResultContent{ToolCallID: id, Output: output})
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func mustUnmarshalToolResult(t *testing.T, content string) message.ToolResultContent {
	t.Helper()
	value, err := message.UnmarshalContent("tool_result", content)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := value.(message.ToolResultContent)
	if !ok {
		t.Fatalf("tool_result content type = %T", value)
	}
	return result
}

type compactionRepoStub struct {
	messages []repository.Message
	parts    []repository.MessagePart
}

func (r *compactionRepoStub) Create(_ context.Context, m repository.Message) (repository.Message, error) {
	m.ID = fmt.Sprintf("gen-%d", len(r.messages)+1)
	r.messages = append(r.messages, m)
	return m, nil
}

func (r *compactionRepoStub) CreateWithParts(ctx context.Context, m repository.Message, parts []repository.MessagePart) (repository.Message, error) {
	created, err := r.Create(ctx, m)
	if err != nil {
		return repository.Message{}, err
	}
	created.Parts = make([]repository.MessagePart, 0, len(parts))
	for _, p := range parts {
		p.MessageID = created.ID
		if p.SessionID == "" {
			p.SessionID = created.SessionID
		}
		part, err := r.CreatePart(ctx, p)
		if err != nil {
			return repository.Message{}, err
		}
		created.Parts = append(created.Parts, part)
	}
	for i := range r.messages {
		if r.messages[i].ID == created.ID {
			r.messages[i] = created
			break
		}
	}
	return created, nil
}

func (r *compactionRepoStub) Get(_ context.Context, _ string) (repository.Message, error) {
	panic("unexpected Get")
}

func (r *compactionRepoStub) ListBySession(_ context.Context, sessionID string) ([]repository.Message, error) {
	var out []repository.Message
	for _, m := range r.messages {
		if m.SessionID == sessionID {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *compactionRepoStub) DeleteBySession(_ context.Context, _ string) error {
	panic("unexpected DeleteBySession")
}

func (r *compactionRepoStub) ListMessagesWithParts(_ context.Context, sessionID string) ([]repository.Message, error) {
	msgs, err := r.ListBySession(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	partsByMessage := make(map[string][]repository.MessagePart)
	for _, p := range r.parts {
		if p.SessionID == sessionID {
			partsByMessage[p.MessageID] = append(partsByMessage[p.MessageID], p)
		}
	}
	for i := range msgs {
		msgs[i].Parts = partsByMessage[msgs[i].ID]
	}
	return msgs, nil
}

func (r *compactionRepoStub) CreatePart(_ context.Context, p repository.MessagePart) (repository.MessagePart, error) {
	p.ID = fmt.Sprintf("gen-p-%d", len(r.parts)+1)
	r.parts = append(r.parts, p)
	return p, nil
}

func (r *compactionRepoStub) ListPartsByMessage(_ context.Context, messageID string) ([]repository.MessagePart, error) {
	var out []repository.MessagePart
	for _, p := range r.parts {
		if p.MessageID == messageID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *compactionRepoStub) ListPartsBySession(_ context.Context, sessionID string) ([]repository.MessagePart, error) {
	var out []repository.MessagePart
	for _, p := range r.parts {
		if p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *compactionRepoStub) UpdatePart(_ context.Context, part repository.MessagePart) error {
	for i := range r.parts {
		if r.parts[i].ID == part.ID {
			r.parts[i] = part
			return nil
		}
	}
	return repository.ErrNotFound
}

func (r *compactionRepoStub) DeletePart(_ context.Context, _ string) error { return nil }

func (r *compactionRepoStub) DeletePartsBySession(_ context.Context, _ string) error {
	panic("unexpected DeletePartsBySession")
}

func (r *compactionRepoStub) BatchUpdateParts(_ context.Context, parts []repository.MessagePart) error {
	for _, p := range parts {
		if err := r.UpdatePart(context.Background(), p); err != nil {
			return err
		}
	}
	return nil
}

func (r *compactionRepoStub) part(id string) repository.MessagePart {
	for _, part := range r.parts {
		if part.ID == id {
			return part
		}
	}
	return repository.MessagePart{}
}

type testProvider struct {
	id       string
	chatErr  error
	response string
}

func (p testProvider) ID() string {
	if p.id == "" {
		return "test-provider"
	}
	return p.id
}

func (p testProvider) Name() string {
	return "test-provider"
}

func (p testProvider) Models(context.Context) ([]provider.Model, error) {
	return nil, nil
}

func (p testProvider) Chat(context.Context, string, []provider.Message, provider.ChatOptions) (<-chan provider.StreamChunk, error) {
	if p.chatErr != nil {
		return nil, p.chatErr
	}
	ch := make(chan provider.StreamChunk, 1)
	if p.response != "" {
		ch <- provider.StreamChunk{Delta: p.response}
	}
	close(ch)
	return ch, nil
}

func TestTruncateMessagesToFit(t *testing.T) {
	// Build a conversation with 10 user/assistant pairs.
	// Each user message is tagged with its turn number for identification.
	var msgs []provider.Message
	for i := range 10 {
		tag := fmt.Sprintf("turn-%d", i)
		msgs = append(msgs,
			provider.Message{Role: "user", Parts: []provider.MessagePart{
				provider.TextPart{Text: tag + " " + strings.Repeat("u", 400)},
			}},
			provider.Message{Role: "assistant", Parts: []provider.MessagePart{
				provider.TextPart{Text: tag + " " + strings.Repeat("a", 400)},
			}},
		)
	}
	total := estimateProviderMessages(msgs)

	t.Run("no truncation needed", func(t *testing.T) {
		result := truncateMessagesToFit(msgs, total+1000)
		if len(result) != len(msgs) {
			t.Fatalf("expected %d messages, got %d", len(msgs), len(result))
		}
	})

	t.Run("keeps first and last turns", func(t *testing.T) {
		limit := total / 2
		result := truncateMessagesToFit(msgs, limit)
		resultTokens := estimateProviderMessages(result)
		if resultTokens > limit {
			t.Fatalf("result tokens %d > limit %d", resultTokens, limit)
		}
		// First message should be from turn-0 (the original user goal).
		first, ok := result[0].Parts[0].(provider.TextPart)
		if !ok {
			t.Fatal("first part is not TextPart")
		}
		if !strings.HasPrefix(first.Text, "turn-0") {
			t.Fatalf("first message = %q, want turn-0 prefix", first.Text[:20])
		}
		// Last message should be from turn-9.
		last, ok := result[len(result)-1].Parts[0].(provider.TextPart)
		if !ok {
			t.Fatal("last part is not TextPart")
		}
		if !strings.HasPrefix(last.Text, "turn-9") {
			t.Fatalf("last message = %q, want turn-9 prefix", last.Text[:20])
		}
	})

	t.Run("very small limit falls back to tail only", func(t *testing.T) {
		result := truncateMessagesToFit(msgs, 250)
		resultTokens := estimateProviderMessages(result)
		if resultTokens > 250 {
			t.Fatalf("result tokens %d > limit 250", resultTokens)
		}
		if result[0].Role != "user" {
			t.Fatalf("first message role = %q, want user", result[0].Role)
		}
	})
}

func TestBuildTurnMessages_FiltersPreCompactionMessages(t *testing.T) {
	msgs := []repository.Message{
		{ID: "01", SessionID: "s1", Role: "user"},
		{ID: "02", SessionID: "s1", Role: "assistant"},
		{ID: "03", SessionID: "s1", Role: "assistant", Summary: true, CompactionParentID: "02"},
		{ID: "04", SessionID: "s1", Role: "user"},
		{ID: "05", SessionID: "s1", Role: "assistant"},
	}
	parts := map[string][]repository.MessagePart{
		"01": {{Type: "text", Content: mustMarshalText(t, "old question")}},
		"02": {{Type: "text", Content: mustMarshalText(t, "old answer")}},
		"03": {{Type: "text", Content: mustMarshalText(t, "summary of conversation")}},
		"04": {{Type: "text", Content: mustMarshalText(t, "new question")}},
		"05": {{Type: "text", Content: mustMarshalText(t, "new answer")}},
	}

	providerMsgs, summaryText := buildTurnMessages(msgs, parts)
	if len(providerMsgs) != 2 {
		t.Fatalf("got %d messages, want 2 (only post-compaction)", len(providerMsgs))
	}
	if providerMsgs[0].Role != "user" {
		t.Fatalf("first message role = %q, want user", providerMsgs[0].Role)
	}
	if summaryText == "" {
		t.Fatal("expected non-empty summary text")
	}
}

func TestBuildTurnMessages_NoSummary_LoadsAll(t *testing.T) {
	msgs := []repository.Message{
		{ID: "01", SessionID: "s1", Role: "user"},
		{ID: "02", SessionID: "s1", Role: "assistant"},
		{ID: "03", SessionID: "s1", Role: "user"},
	}
	parts := map[string][]repository.MessagePart{
		"01": {{Type: "text", Content: mustMarshalText(t, "hello")}},
		"02": {{Type: "text", Content: mustMarshalText(t, "hi")}},
		"03": {{Type: "text", Content: mustMarshalText(t, "bye")}},
	}

	providerMsgs, summaryText := buildTurnMessages(msgs, parts)
	if len(providerMsgs) != 3 {
		t.Fatalf("got %d messages, want 3 (all loaded)", len(providerMsgs))
	}
	if summaryText != "" {
		t.Fatalf("expected empty summary, got %q", summaryText)
	}
}

func TestBuildTurnMessages_MultipleCompactions_UsesLatest(t *testing.T) {
	msgs := []repository.Message{
		{ID: "01", SessionID: "s1", Role: "user"},
		{ID: "02", SessionID: "s1", Role: "assistant"},
		{ID: "03", SessionID: "s1", Role: "assistant", Summary: true, CompactionParentID: "01"},
		{ID: "04", SessionID: "s1", Role: "user"},
		{ID: "05", SessionID: "s1", Role: "assistant"},
		{ID: "06", SessionID: "s1", Role: "assistant", Summary: true, CompactionParentID: "04"},
		{ID: "07", SessionID: "s1", Role: "user"},
	}
	parts := map[string][]repository.MessagePart{
		"01": {{Type: "text", Content: mustMarshalText(t, "q1")}},
		"02": {{Type: "text", Content: mustMarshalText(t, "a1")}},
		"03": {{Type: "text", Content: mustMarshalText(t, "summary 1")}},
		"04": {{Type: "text", Content: mustMarshalText(t, "q2")}},
		"05": {{Type: "text", Content: mustMarshalText(t, "a2")}},
		"06": {{Type: "text", Content: mustMarshalText(t, "summary 2")}},
		"07": {{Type: "text", Content: mustMarshalText(t, "q3")}},
	}

	providerMsgs, summaryText := buildTurnMessages(msgs, parts)
	// Only msg 05 and 07 should survive (after cutoff "04").
	// msg 05 has ID > "04", msg 07 has ID > "04". msg 04 is <= "04" so filtered.
	if len(providerMsgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(providerMsgs))
	}
	if providerMsgs[0].Role != "assistant" {
		t.Fatalf("first message role = %q, want assistant", providerMsgs[0].Role)
	}
	if summaryText == "" {
		t.Fatal("expected non-empty summary text")
	}
}

func TestBuildTurnMessages_UsesTranscriptOrderNotLexicographicIDs(t *testing.T) {
	msgs := []repository.Message{
		{ID: "z-last", SessionID: "s1", Role: "user"},
		{ID: "a-cutoff", SessionID: "s1", Role: "assistant"},
		{ID: "m-summary", SessionID: "s1", Role: "assistant", Summary: true, CompactionParentID: "a-cutoff"},
		{ID: "b-new", SessionID: "s1", Role: "user"},
	}
	parts := map[string][]repository.MessagePart{
		"z-last":    {{Type: "text", Content: mustMarshalText(t, "older user turn")}},
		"a-cutoff":  {{Type: "text", Content: mustMarshalText(t, "older assistant turn")}},
		"m-summary": {{Type: "text", Content: mustMarshalText(t, "summary")}},
		"b-new":     {{Type: "text", Content: mustMarshalText(t, "latest user turn")}},
	}

	providerMsgs, summaryText := buildTurnMessages(msgs, parts)
	if len(providerMsgs) != 1 {
		t.Fatalf("got %d messages, want 1 post-cutoff message", len(providerMsgs))
	}
	text, ok := providerMsgs[0].Parts[0].(provider.TextPart)
	if !ok {
		t.Fatalf("part type = %T, want provider.TextPart", providerMsgs[0].Parts[0])
	}
	if text.Text != "latest user turn" {
		t.Fatalf("text = %q, want %q", text.Text, "latest user turn")
	}
	if summaryText == "" {
		t.Fatal("expected non-empty summary text")
	}
}

// countingRepoStub wraps compactionRepoStub to count DB read calls.
type countingRepoStub struct {
	compactionRepoStub
	listMessagesWithPartsCalls int
	listPartsBySessionCalls    int
}

func (r *countingRepoStub) ListMessagesWithParts(ctx context.Context, sessionID string) ([]repository.Message, error) {
	r.listMessagesWithPartsCalls++
	return r.compactionRepoStub.ListMessagesWithParts(ctx, sessionID)
}

func (r *countingRepoStub) ListPartsBySession(ctx context.Context, sessionID string) ([]repository.MessagePart, error) {
	r.listPartsBySessionCalls++
	return r.compactionRepoStub.ListPartsBySession(ctx, sessionID)
}

func TestPostTurn_SkipsPruneWhenThresholdZero(t *testing.T) {
	repo := &countingRepoStub{
		compactionRepoStub: compactionRepoStub{
			messages: []repository.Message{
				{ID: "m1", SessionID: "s1", Role: "user"},
				{ID: "m2", SessionID: "s1", Role: "assistant"},
			},
			parts: []repository.MessagePart{
				{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "hello")},
				{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "hi")},
			},
		},
	}

	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0),
		CompactionKeepTurns: intp(5),
	}

	mw := NewCompactionMiddleware(cfg, repo, provider.NewRegistry(), nil, &config.Config{}, nil, nil, nil)
	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Model:     provider.Model{ID: "test-model", ContextSize: 128000},
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
			{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "hi"}}},
		},
	}

	err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}

	if repo.listMessagesWithPartsCalls != 0 {
		t.Errorf("ListMessagesWithParts called %d times, want 0 (threshold=0 should skip pruning)", repo.listMessagesWithPartsCalls)
	}
}

func TestPostTurn_SkipsPruneWhenFewMessages(t *testing.T) {
	repo := &countingRepoStub{
		compactionRepoStub: compactionRepoStub{
			messages: []repository.Message{
				{ID: "m1", SessionID: "s1", Role: "user"},
				{ID: "m2", SessionID: "s1", Role: "assistant"},
			},
			parts: []repository.MessagePart{
				{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "hello")},
				{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "hi")},
			},
		},
	}

	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0.8),
		CompactionKeepTurns: intp(5),
	}

	mw := NewCompactionMiddleware(cfg, repo, provider.NewRegistry(), nil, &config.Config{}, nil, nil, nil)
	req := &pipeline.TurnRequest{
		SessionID: "s1",
		Model:     provider.Model{ID: "test-model", ContextSize: 128000},
		Messages: []provider.Message{
			{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
			{Role: "assistant", Parts: []provider.MessagePart{provider.TextPart{Text: "hi"}}},
		},
	}

	err := mw(context.Background(), req, func(_ context.Context, _ *pipeline.TurnRequest) error {
		return nil
	})
	if err != nil {
		t.Fatalf("middleware error = %v", err)
	}

	if repo.listMessagesWithPartsCalls != 0 {
		t.Errorf("ListMessagesWithParts called %d times, want 0 (too few messages to prune)", repo.listMessagesWithPartsCalls)
	}
}

func TestPostTurn_OrphanCleanup(t *testing.T) {
	repo := &countingRepoStub{
		compactionRepoStub: compactionRepoStub{
			messages: []repository.Message{
				{ID: "m1", SessionID: "s1", Role: "user"},
				{ID: "m2", SessionID: "s1", Role: "assistant"},
			},
			parts: []repository.MessagePart{
				{ID: "p1", MessageID: "m1", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "hello")},
				{ID: "p2", MessageID: "m2", SessionID: "s1", Type: "text", Content: mustMarshalText(t, "hi")},
			},
		},
	}

	cfg := &config.SessionConfig{
		CompactionThreshold: f64p(0),
		CompactionKeepTurns: intp(5),
	}

	mw := NewCompactionMiddleware(cfg, repo, provider.NewRegistry(), nil, &config.Config{}, nil, nil, nil)
	mkReq := func() *pipeline.TurnRequest {
		return &pipeline.TurnRequest{
			SessionID: "s1",
			Model:     provider.Model{ID: "test-model", ContextSize: 128000},
			Messages: []provider.Message{
				{Role: "user", Parts: []provider.MessagePart{provider.TextPart{Text: "hello"}}},
			},
		}
	}
	next := func(_ context.Context, _ *pipeline.TurnRequest) error { return nil }

	// First turn of session: orphan cleanup runs once (startup path).
	if err := mw(context.Background(), mkReq(), next); err != nil {
		t.Fatalf("turn 1 error = %v", err)
	}
	if repo.listPartsBySessionCalls == 0 {
		t.Error("first turn: expected startup orphan cleanup to run")
	}

	// Second turn, no StreamInterrupted: no orphan cleanup.
	repo.listPartsBySessionCalls = 0
	if err := mw(context.Background(), mkReq(), next); err != nil {
		t.Fatalf("turn 2 error = %v", err)
	}
	if repo.listPartsBySessionCalls != 0 {
		t.Errorf("turn 2: ListPartsBySession called %d times, want 0 (no stream interruption)", repo.listPartsBySessionCalls)
	}

	// Third turn with StreamInterrupted: orphan cleanup runs again.
	repo.listPartsBySessionCalls = 0
	req3 := mkReq()
	req3.StreamInterrupted = true
	if err := mw(context.Background(), req3, next); err != nil {
		t.Fatalf("turn 3 error = %v", err)
	}
	if repo.listPartsBySessionCalls == 0 {
		t.Error("turn 3: expected orphan cleanup on StreamInterrupted")
	}
}

func TestBoundedSessionSetEvictsOldestEntries(t *testing.T) {
	set := newBoundedSessionSet(2)

	if !set.markSeen("s1") {
		t.Fatal("first sighting of s1 should report needs-cleanup")
	}
	if !set.markSeen("s2") {
		t.Fatal("first sighting of s2 should report needs-cleanup")
	}
	if set.markSeen("s1") {
		t.Fatal("repeat sighting of s1 should not report needs-cleanup")
	}
	if !set.markSeen("s3") {
		t.Fatal("first sighting of s3 should report needs-cleanup")
	}
	if !set.markSeen("s2") {
		t.Fatal("s2 should have been evicted after s3 was added")
	}
}
