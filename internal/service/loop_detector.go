package service

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
)

type loopVerdict int

const (
	loopNone  loopVerdict = iota
	loopWarn
	loopNudge
	loopStop
)

type loopDetector struct {
	callCounts  map[string]int
	history     []string
	windowSize  int
	exemptTools map[string]bool
}

const (
	defaultWindowSize   = 12
	exactRepeatWarn     = 3
	exactRepeatNudge    = 4
	exactRepeatStop     = 5
	cycleDetectMinCalls = 4
)

func newLoopDetector() *loopDetector {
	return &loopDetector{
		callCounts: make(map[string]int),
		windowSize: defaultWindowSize,
		exemptTools: map[string]bool{
			"test":     true,
			"question": true,
			"task":     true,
			"skill":    true,
		},
	}
}

func (ld *loopDetector) record(toolName, args, output string) loopVerdict {
	if ld.exemptTools[toolName] {
		return loopNone
	}

	callHash := hashPair(toolName+"|"+args, output)
	ld.callCounts[callHash]++
	ld.history = append(ld.history, callHash)

	if len(ld.history) > ld.windowSize {
		ld.history = ld.history[len(ld.history)-ld.windowSize:]
		// Rebuild counts from the current window to prevent unbounded growth.
		ld.callCounts = make(map[string]int, len(ld.history))
		for _, h := range ld.history {
			ld.callCounts[h]++
		}
	}

	count := ld.callCounts[callHash]

	if count >= exactRepeatStop {
		log.Printf("loop: exact repetition — tool=%s count=%d", toolName, count)
		return loopStop
	}
	if count >= exactRepeatNudge {
		log.Printf("loop: repeated tool call — tool=%s count=%d", toolName, count)
		return loopNudge
	}
	if count >= exactRepeatWarn {
		log.Printf("loop: potential loop — tool=%s count=%d", toolName, count)
		return loopWarn
	}

	if v := ld.detectCycle(); v != loopNone {
		return v
	}

	return loopNone
}

func (ld *loopDetector) recordBatch(calls []toolExecution) loopVerdict {
	worst := loopNone
	for _, ex := range calls {
		output := ex.output
		if ex.errStr != nil {
			output = *ex.errStr
		}
		v := ld.record(ex.call.Name, ex.call.Arguments, output)
		if v > worst {
			worst = v
		}
	}
	return worst
}

func (ld *loopDetector) detectCycle() loopVerdict {
	n := len(ld.history)
	if n < cycleDetectMinCalls {
		return loopNone
	}

	maxCycleLen := n / 3
	if maxCycleLen < 2 {
		return loopNone
	}

	for cycleLen := 2; cycleLen <= maxCycleLen; cycleLen++ {
		needed := cycleLen * 3
		if n < needed {
			continue
		}
		tail := ld.history[n-needed:]
		if isRepeating(tail, cycleLen) {
			log.Printf("loop: cycle detected — %d-step pattern repeated 3x in last %d calls", cycleLen, needed)
			return loopNudge
		}
	}

	for cycleLen := 2; cycleLen <= min(maxCycleLen, n/2); cycleLen++ {
		needed := cycleLen * 2
		if n < needed {
			continue
		}
		tail := ld.history[n-needed:]
		if isRepeating(tail, cycleLen) {
			log.Printf("loop: potential cycle — %d-step pattern repeated 2x", cycleLen)
			return loopWarn
		}
	}

	return loopNone
}

func isRepeating(seq []string, cycleLen int) bool {
	for i := cycleLen; i < len(seq); i++ {
		if seq[i] != seq[i%cycleLen] {
			return false
		}
	}
	return true
}

func hashPair(call, output string) string {
	h := sha256.New()
	h.Write([]byte(call))
	h.Write([]byte{0})
	if len(output) > 4096 {
		output = output[:4096]
	}
	h.Write([]byte(output))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
