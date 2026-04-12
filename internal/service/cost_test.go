package service

import (
	"sync"
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestSessionCost_Add(t *testing.T) {
	sc := NewSessionCost()
	m := provider.Model{CostInput: 3.0, CostOutput: 15.0}
	sc.Add(&provider.Usage{InputTokens: 1000, OutputTokens: 500}, m)

	snap := sc.Snapshot()
	if snap.InputTokens != 1000 {
		t.Errorf("InputTokens = %d, want 1000", snap.InputTokens)
	}
	if snap.OutputTokens != 500 {
		t.Errorf("OutputTokens = %d, want 500", snap.OutputTokens)
	}
	wantCost := 1000.0/1_000_000*3.0 + 500.0/1_000_000*15.0
	if snap.TotalCost != wantCost {
		t.Errorf("TotalCost = %f, want %f", snap.TotalCost, wantCost)
	}
	if snap.SubagentCost != 0 {
		t.Errorf("SubagentCost = %f, want 0", snap.SubagentCost)
	}
}

func TestSessionCost_Add_AllTokenTypes(t *testing.T) {
	sc := NewSessionCost()
	m := provider.Model{
		CostInput:      3.0,
		CostOutput:     15.0,
		CostReasoning:  15.0,
		CostCacheRead:  0.30,
		CostCacheWrite: 3.75,
	}
	u := &provider.Usage{
		InputTokens:      1000,
		OutputTokens:     500,
		ReasoningTokens:  200,
		CacheReadTokens:  5000,
		CacheWriteTokens: 800,
	}
	sc.Add(u, m)

	snap := sc.Snapshot()
	wantCost := 1000.0/1e6*3.0 +
		500.0/1e6*15.0 +
		200.0/1e6*15.0 +
		5000.0/1e6*0.30 +
		800.0/1e6*3.75
	if snap.TotalCost != wantCost {
		t.Errorf("TotalCost = %f, want %f", snap.TotalCost, wantCost)
	}
	if snap.ReasoningTokens != 200 {
		t.Errorf("ReasoningTokens = %d, want 200", snap.ReasoningTokens)
	}
	if snap.CacheReadTokens != 5000 {
		t.Errorf("CacheReadTokens = %d, want 5000", snap.CacheReadTokens)
	}
	if snap.CacheWriteTokens != 800 {
		t.Errorf("CacheWriteTokens = %d, want 800", snap.CacheWriteTokens)
	}
}

func TestSessionCost_AddSubagentCost(t *testing.T) {
	sc := NewSessionCost()
	m := provider.Model{CostInput: 3.0, CostOutput: 15.0}
	sc.Add(&provider.Usage{InputTokens: 2000, OutputTokens: 1000}, m)

	agentSnap := CostSnapshot{
		InputTokens:  500,
		OutputTokens: 200,
		TotalCost:    0.005,
	}
	sc.AddSubagentCost(agentSnap)

	snap := sc.Snapshot()
	if snap.InputTokens != 2500 {
		t.Errorf("InputTokens = %d, want 2500", snap.InputTokens)
	}
	if snap.OutputTokens != 1200 {
		t.Errorf("OutputTokens = %d, want 1200", snap.OutputTokens)
	}
	if snap.SubagentCost != 0.005 {
		t.Errorf("SubagentCost = %f, want 0.005", snap.SubagentCost)
	}
	if snap.SubagentInputs != 500 {
		t.Errorf("SubagentInputs = %d, want 500", snap.SubagentInputs)
	}
	if snap.SubagentOutputs != 200 {
		t.Errorf("SubagentOutputs = %d, want 200", snap.SubagentOutputs)
	}
}

func TestSessionCost_AddSubagentCost_Multiple(t *testing.T) {
	sc := NewSessionCost()

	sc.AddSubagentCost(CostSnapshot{InputTokens: 100, OutputTokens: 50, TotalCost: 0.001})
	sc.AddSubagentCost(CostSnapshot{InputTokens: 200, OutputTokens: 80, TotalCost: 0.002})
	sc.AddSubagentCost(CostSnapshot{InputTokens: 300, OutputTokens: 120, TotalCost: 0.003})

	snap := sc.Snapshot()
	if snap.SubagentInputs != 600 {
		t.Errorf("SubagentInputs = %d, want 600", snap.SubagentInputs)
	}
	if snap.SubagentOutputs != 250 {
		t.Errorf("SubagentOutputs = %d, want 250", snap.SubagentOutputs)
	}
	if snap.SubagentCost != 0.006 {
		t.Errorf("SubagentCost = %f, want 0.006", snap.SubagentCost)
	}
	if snap.TotalCost != 0.006 {
		t.Errorf("TotalCost = %f, want 0.006 (all from subagents)", snap.TotalCost)
	}
}

func TestSessionCost_AddSubagentCost_Concurrent(t *testing.T) {
	sc := NewSessionCost()
	var wg sync.WaitGroup
	for i := range 100 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sc.AddSubagentCost(CostSnapshot{
				InputTokens:  100,
				OutputTokens: 50,
				TotalCost:    0.001,
			})
		}(i)
	}
	wg.Wait()

	snap := sc.Snapshot()
	if snap.SubagentInputs != 10000 {
		t.Errorf("SubagentInputs = %d, want 10000", snap.SubagentInputs)
	}
	if diff := snap.SubagentCost - 0.1; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("SubagentCost = %f, want ~0.1", snap.SubagentCost)
	}
}

func TestSessionCost_AddNilUsage(t *testing.T) {
	sc := NewSessionCost()
	sc.Add(nil, provider.Model{CostInput: 3.0, CostOutput: 15.0})
	snap := sc.Snapshot()
	if snap.TotalCost != 0 {
		t.Errorf("TotalCost = %f, want 0 after nil usage", snap.TotalCost)
	}
}

func TestSessionCost_Total_IncludesSubagents(t *testing.T) {
	sc := NewSessionCost()
	m := provider.Model{CostInput: 3.0, CostOutput: 15.0}
	sc.Add(&provider.Usage{InputTokens: 1000, OutputTokens: 500}, m)
	parentCost := sc.Total()

	sc.AddSubagentCost(CostSnapshot{InputTokens: 100, OutputTokens: 50, TotalCost: 0.01})

	if sc.Total() != parentCost+0.01 {
		t.Errorf("Total() = %f, want %f", sc.Total(), parentCost+0.01)
	}
}
