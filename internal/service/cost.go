package service

import (
	"math"
	"sync"

	"github.com/sageil/kodacode/v1/internal/provider"
)

type CostSnapshot struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  int     `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens int     `json:"cache_write_tokens,omitempty"`
	TotalCost        float64 `json:"total_cost"`
	SubagentCost     float64 `json:"subagent_cost,omitempty"`
	SubagentInputs   int     `json:"subagent_inputs,omitempty"`
	SubagentOutputs  int     `json:"subagent_outputs,omitempty"`
}

type SessionCost struct {
	mu               sync.RWMutex
	inputTokens      int
	outputTokens     int
	reasoningTokens  int
	cacheReadTokens  int
	cacheWriteTokens int
	totalMicroUSD    int64 // cost in micro-dollars (1e-6 USD) for precision
	subagentMicroUSD int64
	subagentInputs   int
	subagentOutputs  int
	lastInputTokens  int
}

func NewSessionCost() *SessionCost {
	return &SessionCost{}
}

// toMicroUSD converts token counts and per-million pricing to micro-dollars.
func toMicroUSD(tokens int, costPerMillion float64) int64 {
	return int64(math.Round(float64(tokens) / 1_000_000 * costPerMillion * 1e6))
}

func usageMicroUSD(u *provider.Usage, m provider.Model) int64 {
	return toMicroUSD(u.InputTokens, m.CostInput) +
		toMicroUSD(u.OutputTokens, m.CostOutput) +
		toMicroUSD(u.ReasoningTokens, m.CostReasoning) +
		toMicroUSD(u.CacheReadTokens, m.CostCacheRead) +
		toMicroUSD(u.CacheWriteTokens, m.CostCacheWrite)
}

func (sc *SessionCost) Add(u *provider.Usage, m provider.Model) {
	if u == nil {
		return
	}
	micro := usageMicroUSD(u, m)
	sc.mu.Lock()
	sc.inputTokens += u.InputTokens
	sc.outputTokens += u.OutputTokens
	sc.reasoningTokens += u.ReasoningTokens
	sc.cacheReadTokens += u.CacheReadTokens
	sc.cacheWriteTokens += u.CacheWriteTokens
	sc.totalMicroUSD += micro
	sc.lastInputTokens = u.InputTokens + u.CacheWriteTokens
	sc.mu.Unlock()
}

func (sc *SessionCost) LastInputTokens() int {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return sc.lastInputTokens
}

func (sc *SessionCost) Total() float64 {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return float64(sc.totalMicroUSD) / 1e6
}

func (sc *SessionCost) AddSubagentCost(snap CostSnapshot) {
	micro := int64(math.Round(snap.TotalCost * 1e6))
	sc.mu.Lock()
	sc.inputTokens += snap.InputTokens
	sc.outputTokens += snap.OutputTokens
	sc.reasoningTokens += snap.ReasoningTokens
	sc.cacheReadTokens += snap.CacheReadTokens
	sc.cacheWriteTokens += snap.CacheWriteTokens
	sc.totalMicroUSD += micro
	sc.subagentMicroUSD += micro
	sc.subagentInputs += snap.InputTokens
	sc.subagentOutputs += snap.OutputTokens
	sc.mu.Unlock()
}

func (sc *SessionCost) Snapshot() CostSnapshot {
	sc.mu.RLock()
	defer sc.mu.RUnlock()
	return CostSnapshot{
		InputTokens:      sc.inputTokens,
		OutputTokens:     sc.outputTokens,
		ReasoningTokens:  sc.reasoningTokens,
		CacheReadTokens:  sc.cacheReadTokens,
		CacheWriteTokens: sc.cacheWriteTokens,
		TotalCost:        float64(sc.totalMicroUSD) / 1e6,
		SubagentCost:     float64(sc.subagentMicroUSD) / 1e6,
		SubagentInputs:   sc.subagentInputs,
		SubagentOutputs:  sc.subagentOutputs,
	}
}

func (sc *SessionCost) Seed(inputTokens, outputTokens, lastInputTokens int, totalCost float64) {
	sc.mu.Lock()
	sc.inputTokens = inputTokens
	sc.outputTokens = outputTokens
	sc.lastInputTokens = lastInputTokens
	sc.totalMicroUSD = int64(math.Round(totalCost * 1e6))
	sc.mu.Unlock()
}
