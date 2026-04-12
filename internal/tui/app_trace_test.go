package tui

import (
	"strings"
	"testing"
)

func TestRenderTurnSummaryTable_SingleTurn(t *testing.T) {
	a := App{
		traceEnabled: true,
		stepTraces: [][]stepTraceTUI{
			{
				{
					Step:    1,
					ModelID: "m1",
					Usage: &stepTraceUsageTUI{
						InputTokens:     5000,
						OutputTokens:    1200,
						CacheReadTokens: 3000,
					},
					CostMicroUSD: 1500,
					Tools:        []stepToolTraceTUI{{Name: "read", Elapsed: 120}},
					WallClock:    4500,
				},
				{
					Step:    2,
					ModelID: "m1",
					Usage: &stepTraceUsageTUI{
						InputTokens:  6000,
						OutputTokens: 800,
					},
					CostMicroUSD: 1000,
					WallClock:    2000,
				},
			},
		},
	}

	got := a.renderTurnSummaryTable()

	if !strings.Contains(got, "┌") {
		t.Error("missing table border")
	}
	if !strings.Contains(got, "$0.0025") {
		t.Error("missing aggregated cost")
	}
	if !strings.Contains(got, "/trace") {
		t.Error("missing hint for /trace N")
	}
}

func TestRenderTurnSummaryTable_InputTokensAggregated(t *testing.T) {
	a := App{
		traceEnabled: true,
		stepTraces: [][]stepTraceTUI{
			{{
				Step:    1,
				ModelID: "m1",
				Usage: &stepTraceUsageTUI{
					InputTokens:      1000,
					CacheReadTokens:  2000,
					CacheWriteTokens: 500,
				},
				CostMicroUSD: 100,
				WallClock:    1000,
			}},
		},
	}
	got := a.renderTurnSummaryTable()
	if !strings.Contains(got, "3.5k") {
		t.Errorf("expected aggregated input 3.5k in output: %s", got)
	}
}

func TestRenderCostMessage_IncludesTraceTable(t *testing.T) {
	a := App{
		traceEnabled: true,
		stepTraces: [][]stepTraceTUI{
			{{
				Step: 1, ModelID: "m1",
				Usage:        &stepTraceUsageTUI{InputTokens: 5000, OutputTokens: 200},
				CostMicroUSD: 500, WallClock: 3000,
				Tools: []stepToolTraceTUI{{Name: "bash"}, {Name: "read"}},
			}},
		},
	}
	a.cfg.Model = "github-copilot/gpt-4.1"

	sb := StatusBar{inputTokens: 5000, outputTokens: 200, maxInputTokens: 64000}
	got := a.renderCostMessage(sb)

	if !strings.Contains(got, "Session cost") {
		t.Error("missing cost header")
	}
	if !strings.Contains(got, "Turns: 1") {
		t.Error("missing turns count")
	}
	if !strings.Contains(got, "Steps: 1") {
		t.Error("missing steps count")
	}
	if !strings.Contains(got, "┌") {
		t.Error("missing embedded trace table")
	}
}

func TestRenderCostMessage_NoTraceWhenDisabled(t *testing.T) {
	a := App{traceEnabled: false}
	a.cfg.Model = "openai/gpt-4.1"
	sb := StatusBar{inputTokens: 5000, outputTokens: 200}
	got := a.renderCostMessage(sb)

	if !strings.Contains(got, "Session cost") {
		t.Error("missing cost header")
	}
	if strings.Contains(got, "┌") {
		t.Error("should not have trace table when disabled")
	}
}

func TestRenderTraceDetail_InvalidArg(t *testing.T) {
	a := App{traceEnabled: true, stepTraces: [][]stepTraceTUI{{{Step: 1}}}}
	for _, arg := range []string{"abc", "0", "99"} {
		got := a.renderTraceDetail(arg)
		if !strings.Contains(got, "Invalid turn number") {
			t.Errorf("expected invalid for %q", arg)
		}
	}
}

func TestRenderTraceDetail_ShowsCacheColumns(t *testing.T) {
	a := App{
		traceEnabled: true,
		stepTraces: [][]stepTraceTUI{
			{{
				Step: 1, ModelID: "m1",
				Usage: &stepTraceUsageTUI{
					InputTokens: 1000, OutputTokens: 500,
					CacheReadTokens: 2000, CacheWriteTokens: 300,
				},
				CostMicroUSD: 100, WallClock: 1000,
			}},
		},
	}
	got := a.renderTraceDetail("1")
	if !strings.Contains(got, "Turn 1 Detail") {
		t.Error("missing header")
	}
	if !strings.Contains(got, "Cache R") {
		t.Error("detail view should have Cache R column")
	}
}

func TestRenderTraceDetail_WithSegments(t *testing.T) {
	a := App{
		traceEnabled: true,
		stepTraces: [][]stepTraceTUI{
			{{
				Step: 1, ModelID: "m1",
				Usage:        &stepTraceUsageTUI{InputTokens: 5000},
				CostMicroUSD: 100, WallClock: 1000,
				Segments: &segmentBytesTUI{
					StablePrompt: 1000, SemiStable: 2000, Volatile: 500,
					Messages: 5000, ToolSchemas: 1500, Total: 10000,
				},
			}},
		},
	}
	got := a.renderTraceDetail("1")
	if !strings.Contains(got, "Prompt Segments") {
		t.Error("missing segment section")
	}
}

func TestRenderTraceDetail_WithTools(t *testing.T) {
	a := App{
		traceEnabled: true,
		stepTraces: [][]stepTraceTUI{
			{{
				Step: 1, ModelID: "m1",
				CostMicroUSD: 100, WallClock: 1000,
				Tools: []stepToolTraceTUI{
					{Name: "bash", Elapsed: 3400},
					{Name: "read", Elapsed: 50, Error: "not found"},
				},
			}},
		},
	}
	got := a.renderTraceDetail("1")
	if !strings.Contains(got, "Tool Timing") {
		t.Error("missing tool timing section")
	}
}

func TestRenderTraceDetail_Disabled(t *testing.T) {
	a := App{}
	got := a.renderTraceDetail("1")
	if !strings.Contains(got, "Trace capture is disabled") {
		t.Fatalf("expected disabled message, got: %s", got)
	}
}

func TestFormatTraceTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "-"},
		{500, "500"},
		{1000, "1.0k"},
		{1500, "1.5k"},
		{1000000, "1.0M"},
		{2500000, "2.5M"},
	}
	for _, tt := range tests {
		got := formatTraceTokens(tt.input)
		if got != tt.want {
			t.Errorf("formatTraceTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{50, "50ms"},
		{999, "999ms"},
		{1000, "1.0s"},
		{2500, "2.5s"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.ms)
		if got != tt.want {
			t.Errorf("formatDuration(%d) = %q, want %q", tt.ms, got, tt.want)
		}
	}
}

func TestCountSteps(t *testing.T) {
	traces := [][]stepTraceTUI{
		{{Step: 1}, {Step: 2}},
		{{Step: 1}},
	}
	if got := countSteps(traces); got != 3 {
		t.Errorf("countSteps = %d, want 3", got)
	}
}
