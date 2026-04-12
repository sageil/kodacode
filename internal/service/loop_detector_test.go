package service

import (
	"testing"

	"github.com/sageil/kodacode/v1/internal/provider"
)

func TestLoopDetector_noLoopOnDifferentOutputs(t *testing.T) {
	ld := newLoopDetector()

	for i := range 10 {
		v := ld.record("bash", `{"cmd":"go test"}`, result(i))
		if v >= loopNudge {
			t.Fatalf("step %d: should not detect loop when outputs differ, got verdict=%d", i, v)
		}
	}
}

func TestLoopDetector_detectsExactRepetition(t *testing.T) {
	ld := newLoopDetector()

	var lastVerdict loopVerdict
	for range exactRepeatNudge + 1 {
		lastVerdict = ld.record("bash", `{"cmd":"go build"}`, "exit 1: compile error")
	}
	if lastVerdict < loopNudge {
		t.Fatalf("expected nudge after %d identical calls, got verdict=%d", exactRepeatNudge, lastVerdict)
	}
}

func TestLoopDetector_stopsOnPersistentRepetition(t *testing.T) {
	ld := newLoopDetector()

	var lastVerdict loopVerdict
	for range exactRepeatStop + 1 {
		lastVerdict = ld.record("bash", `{"cmd":"go build"}`, "exit 1: same error")
	}
	if lastVerdict != loopStop {
		t.Fatalf("expected stop after %d identical calls, got verdict=%d", exactRepeatStop, lastVerdict)
	}
}

func TestLoopDetector_exemptTools(t *testing.T) {
	ld := newLoopDetector()

	for range 20 {
		v := ld.record("test", `{"cmd":"go test ./..."}`, "FAIL")
		if v != loopNone {
			t.Fatalf("test tool should be exempt from loop detection")
		}
	}
}

func TestLoopDetector_detectsSlidingWindowCycle(t *testing.T) {
	ld := newLoopDetector()

	// Simulate A→B→A→B→A→B pattern (3 repetitions of 2-step cycle).
	var lastVerdict loopVerdict
	for i := range 6 {
		if i%2 == 0 {
			lastVerdict = ld.record("read", `{"file":"config.go"}`, "package config")
		} else {
			lastVerdict = ld.record("edit", `{"file":"config.go"}`, "ok")
		}
	}
	if lastVerdict < loopWarn {
		t.Fatalf("expected at least warn for 2-step cycle repeated 3 times, got verdict=%d", lastVerdict)
	}
}

func TestLoopDetector_noFalsePositiveOnProgress(t *testing.T) {
	ld := newLoopDetector()

	// Simulate test→fix→test→fix with DIFFERENT test outputs each time.
	steps := []struct {
		tool, args, output string
	}{
		{"bash", `{"cmd":"go test"}`, "FAIL: 3 errors"},
		{"edit", `{"file":"handler.go"}`, "ok"},
		{"bash", `{"cmd":"go test"}`, "FAIL: 2 errors"},
		{"edit", `{"file":"handler.go"}`, "ok"},
		{"bash", `{"cmd":"go test"}`, "FAIL: 1 error"},
		{"edit", `{"file":"handler.go"}`, "ok"},
		{"bash", `{"cmd":"go test"}`, "PASS"},
	}
	for i, s := range steps {
		v := ld.record(s.tool, s.args, s.output)
		if v >= loopNudge {
			t.Fatalf("step %d: false positive — outputs are different, should not nudge", i)
		}
	}
}

func TestLoopDetector_recordBatch(t *testing.T) {
	ld := newLoopDetector()

	batch := []toolExecution{
		{call: provider.ToolCall{Name: "read", Arguments: `{"file":"a.go"}`}, output: "content a"},
		{call: provider.ToolCall{Name: "read", Arguments: `{"file":"b.go"}`}, output: "content b"},
	}

	v := ld.recordBatch(batch)
	if v != loopNone {
		t.Fatalf("first batch should not trigger loop, got verdict=%d", v)
	}
}

func TestLoopDetector_threeStepCycle(t *testing.T) {
	ld := newLoopDetector()

	// A→B→C repeated 3 times.
	var lastVerdict loopVerdict
	for i := range 9 {
		switch i % 3 {
		case 0:
			lastVerdict = ld.record("read", `{"file":"main.go"}`, "package main")
		case 1:
			lastVerdict = ld.record("edit", `{"file":"main.go","change":"fix"}`, "ok")
		case 2:
			lastVerdict = ld.record("bash", `{"cmd":"go build"}`, "exit 1: error")
		}
	}
	if lastVerdict < loopWarn {
		t.Fatalf("expected at least warn for 3-step cycle repeated 3 times, got verdict=%d", lastVerdict)
	}
}

func TestHashPair_deterministic(t *testing.T) {
	h1 := hashPair("bash|go test", "PASS")
	h2 := hashPair("bash|go test", "PASS")
	if h1 != h2 {
		t.Fatalf("hash should be deterministic: %s != %s", h1, h2)
	}
}

func TestHashPair_differentForDifferentOutputs(t *testing.T) {
	h1 := hashPair("bash|go test", "PASS")
	h2 := hashPair("bash|go test", "FAIL")
	if h1 == h2 {
		t.Fatal("different outputs should produce different hashes")
	}
}

func TestIsRepeating(t *testing.T) {
	if !isRepeating([]string{"a", "b", "a", "b"}, 2) {
		t.Error("should detect 2-element repeat")
	}
	if !isRepeating([]string{"a", "b", "c", "a", "b", "c"}, 3) {
		t.Error("should detect 3-element repeat")
	}
	if isRepeating([]string{"a", "b", "c", "d"}, 2) {
		t.Error("should not detect repeat in non-repeating sequence")
	}
}

func result(i int) string {
	return string(rune('A' + i))
}
