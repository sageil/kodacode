package service

import (
	"sync"
	"time"

	"github.com/sageil/kodacode/v1/internal/provider"
)

// StepToolTrace captures timing for one tool call within a step.
type StepToolTrace struct {
	Name    string        `json:"name"`
	Elapsed time.Duration `json:"-"`
	Error   string        `json:"error,omitempty"`
	// ElapsedMS is the JSON-friendly representation of Elapsed.
	ElapsedMS int64 `json:"elapsed_ms"`
}

// StepTrace captures everything about a single LLM call (step) in the turn loop.
type StepTrace struct {
	Step         int             `json:"step"`
	ModelID      string          `json:"model_id"`
	Usage        *provider.Usage `json:"usage,omitempty"`
	CostMicroUSD int64           `json:"cost_micro_usd"`
	Tools        []StepToolTrace `json:"tools,omitempty"`
	RetryCount   int             `json:"retry_count"`
	FallbackUsed bool            `json:"fallback_used,omitempty"`
	LoopVerdict  int             `json:"loop_verdict,omitempty"`
	WallClock    time.Duration   `json:"-"`
	WallClockMS  int64           `json:"wall_clock_ms"`
	Segments     *SegmentBytes   `json:"segments,omitempty"`
}

// SegmentBytes holds the exact byte size of each prompt segment.
// Byte proportions show which segment dominates the prompt without
// fabricating token counts — the actual token total comes from the API.
type SegmentBytes struct {
	StablePrompt int `json:"stable_prompt"`
	SemiStable   int `json:"semi_stable"`
	Volatile     int `json:"volatile"`
	Messages     int `json:"messages"`
	ToolSchemas  int `json:"tool_schemas"`
	Total        int `json:"total"`
}

// SessionTraces accumulates per-turn traces for the session lifetime.
type SessionTraces struct {
	mu       sync.Mutex
	turns    [][]StepTrace
	onCommit func(turnIndex int, steps []StepTrace)
}

func NewSessionTraces() *SessionTraces {
	return &SessionTraces{}
}

func (st *SessionTraces) CommitTurn(steps []StepTrace) {
	cp := make([]StepTrace, len(steps))
	copy(cp, steps)
	st.mu.Lock()
	st.turns = append(st.turns, cp)
	idx := len(st.turns) - 1
	cb := st.onCommit
	st.mu.Unlock()
	if cb != nil {
		cb(idx, cp)
	}
}

func (st *SessionTraces) AllTurns() [][]StepTrace {
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([][]StepTrace, len(st.turns))
	copy(out, st.turns)
	return out
}

func (st *SessionTraces) TurnCount() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.turns)
}

func finalizeStepTrace(s *StepTrace) {
	s.WallClockMS = int64(s.WallClock / time.Millisecond)
	for i := range s.Tools {
		s.Tools[i].ElapsedMS = int64(s.Tools[i].Elapsed / time.Millisecond)
	}
}

func captureSegmentBytes(systemParts []string, msgs []provider.Message, tools []provider.Tool) SegmentBytes {
	var sb SegmentBytes
	if len(systemParts) > 0 {
		sb.StablePrompt = len(systemParts[0])
	}
	if len(systemParts) > 1 {
		sb.SemiStable = len(systemParts[1])
	}
	if len(systemParts) > 2 {
		sb.Volatile = len(systemParts[2])
	}
	for _, m := range msgs {
		for _, p := range m.Parts {
			switch v := p.(type) {
			case provider.TextPart:
				sb.Messages += len(v.Text)
			case provider.ToolCallPart:
				sb.Messages += len(v.Name) + len(v.Arguments)
			case provider.ToolResultPart:
				sb.Messages += len(v.Output)
			}
		}
	}
	for _, t := range tools {
		sb.ToolSchemas += len(t.Name) + len(t.Description) + len(t.Parameters)
	}
	sb.Total = sb.StablePrompt + sb.SemiStable + sb.Volatile + sb.Messages + sb.ToolSchemas
	return sb
}
