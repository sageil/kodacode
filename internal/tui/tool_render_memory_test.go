package tui

import (
	"strings"
	"testing"

	"github.com/sageil/kodacode/internal/events"
)

func TestMemoryToolDisplayNameAndSummary(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "memory",
		Input:    `{"action":"save","content":"Remember the runtime owns orchestration.","id":null}`,
	}

	if got := memoryToolDisplayName(call); got != "save project memory" {
		t.Fatalf("memoryToolDisplayName() = %q", got)
	}
	summary := memoryToolListSummary(call)
	if summary == "" || !containsAll(summary, "action: save", "content: Remember the runtime owns orchestration.") {
		t.Fatalf("memoryToolListSummary() = %q", summary)
	}
}

func TestMemoryInspectorParamsIncludeActionAndID(t *testing.T) {
	call := &events.ToolCallState{
		ToolName: "memory",
		Input:    `{"action":"delete","content":null,"id":"memory-1"}`,
	}

	params := memoryInspectorParams(call)
	if len(params) < 2 {
		t.Fatalf("memoryInspectorParams() = %#v", params)
	}
	if params[0].Label != "Action" || params[0].Value != "delete" {
		t.Fatalf("first param = %#v", params[0])
	}
	if params[1].Label != "ID" || params[1].Value != "memory-1" {
		t.Fatalf("second param = %#v", params[1])
	}
}

func containsAll(value string, parts ...string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
