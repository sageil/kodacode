package service

import (
	"sync"
	"testing"
	"time"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestSessionTraces_CommitTurn(t *testing.T) {
	st := NewSessionTraces()

	steps := []StepTrace{
		{Step: 1, ModelID: "m1", CostMicroUSD: 100, WallClock: 500 * time.Millisecond},
		{
			Step: 2, ModelID: "m1", CostMicroUSD: 200, WallClock: 300 * time.Millisecond,
			Tools: []StepToolTrace{{Name: "bash", Elapsed: 150 * time.Millisecond}},
		},
	}
	st.CommitTurn(steps)

	if st.TurnCount() != 1 {
		t.Fatalf("expected 1 turn, got %d", st.TurnCount())
	}

	turns := st.AllTurns()
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if len(turns[0]) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(turns[0]))
	}
	if turns[0][1].Tools[0].Name != "bash" {
		t.Errorf("expected tool name bash, got %s", turns[0][1].Tools[0].Name)
	}
}

func TestSessionTraces_MultipleTurns(t *testing.T) {
	st := NewSessionTraces()

	st.CommitTurn([]StepTrace{{Step: 1, ModelID: "m1"}})
	st.CommitTurn([]StepTrace{{Step: 1, ModelID: "m2"}, {Step: 2, ModelID: "m2"}})

	if st.TurnCount() != 2 {
		t.Fatalf("expected 2 turns, got %d", st.TurnCount())
	}

	turns := st.AllTurns()
	if len(turns[0]) != 1 || len(turns[1]) != 2 {
		t.Fatalf("unexpected step counts: turn0=%d turn1=%d", len(turns[0]), len(turns[1]))
	}
}

func TestSessionTraces_Concurrent(t *testing.T) {
	st := NewSessionTraces()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			st.CommitTurn([]StepTrace{{Step: n, ModelID: "m1"}})
		}(i)
	}
	wg.Wait()

	if st.TurnCount() != 50 {
		t.Fatalf("expected 50 turns, got %d", st.TurnCount())
	}
}

func TestFinalizeStepTrace(t *testing.T) {
	s := StepTrace{
		WallClock: 1500 * time.Millisecond,
		Tools: []StepToolTrace{
			{Name: "bash", Elapsed: 250 * time.Millisecond},
			{Name: "read", Elapsed: 50 * time.Millisecond},
		},
	}
	finalizeStepTrace(&s)

	if s.WallClockMS != 1500 {
		t.Errorf("expected WallClockMS=1500, got %d", s.WallClockMS)
	}
	if s.Tools[0].ElapsedMS != 250 {
		t.Errorf("expected tool 0 ElapsedMS=250, got %d", s.Tools[0].ElapsedMS)
	}
	if s.Tools[1].ElapsedMS != 50 {
		t.Errorf("expected tool 1 ElapsedMS=50, got %d", s.Tools[1].ElapsedMS)
	}
}

func TestCaptureSegmentBytes(t *testing.T) {
	sysParts := []string{
		"stable prompt content",     // 21 bytes
		"semi-stable env block",     // 21 bytes
		"volatile pins and summary", // 25 bytes
	}
	msgs := []provider.Message{
		{Role: "user", Parts: []provider.MessagePart{
			provider.TextPart{Text: "hello world"}, // 11 bytes
		}},
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.TextPart{Text: "response"}, // 8 bytes
		}},
	}
	tools := []provider.Tool{
		{Name: "bash", Description: "Run command", Parameters: []byte(`{"type":"object"}`)},
	}

	sb := captureSegmentBytes(sysParts, msgs, tools)

	if sb.StablePrompt != 21 {
		t.Errorf("StablePrompt = %d, want 21", sb.StablePrompt)
	}
	if sb.SemiStable != 21 {
		t.Errorf("SemiStable = %d, want 21", sb.SemiStable)
	}
	if sb.Volatile != 25 {
		t.Errorf("Volatile = %d, want 25", sb.Volatile)
	}
	if sb.Messages != 19 { // 11 + 8
		t.Errorf("Messages = %d, want 19", sb.Messages)
	}
	// bash(4) + Run command(11) + {"type":"object"}(17) = 32
	if sb.ToolSchemas != 32 {
		t.Errorf("ToolSchemas = %d, want 32", sb.ToolSchemas)
	}
	if sb.Total != 21+21+25+19+32 {
		t.Errorf("Total = %d, want %d", sb.Total, 21+21+25+19+32)
	}
}

func TestCaptureSegmentBytes_Empty(t *testing.T) {
	sb := captureSegmentBytes(nil, nil, nil)
	if sb.Total != 0 {
		t.Errorf("Total = %d, want 0", sb.Total)
	}
}

func TestCaptureSegmentBytes_ToolCallParts(t *testing.T) {
	msgs := []provider.Message{
		{Role: "assistant", Parts: []provider.MessagePart{
			provider.ToolCallPart{Name: "read", Arguments: `{"path":"main.go"}`},
		}},
		{Role: "user", Parts: []provider.MessagePart{
			provider.ToolResultPart{Output: "file contents here"},
		}},
	}
	sb := captureSegmentBytes(nil, msgs, nil)
	// read(4) + {"path":"main.go"}(18) + file contents here(18) = 40
	if sb.Messages != 40 {
		t.Errorf("Messages = %d, want 40", sb.Messages)
	}
}

func TestSessionTraces_CommitTurnIsolation(t *testing.T) {
	st := NewSessionTraces()
	steps := []StepTrace{{Step: 1, ModelID: "m1", Usage: &provider.Usage{InputTokens: 100}}}
	st.CommitTurn(steps)

	steps[0].ModelID = "mutated"

	turns := st.AllTurns()
	if turns[0][0].ModelID != "m1" {
		t.Errorf("expected m1, got %s — CommitTurn did not copy", turns[0][0].ModelID)
	}
}
